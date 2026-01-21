package replication

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Failover errors.
var (
	ErrFailoverInProgress  = errors.New("failover already in progress")
	ErrFailoverInhibited   = errors.New("failover is inhibited")
	ErrNoHealthyFollowers  = errors.New("no healthy followers available")
	ErrFailoverDisabled    = errors.New("automatic failover is disabled")
)

// FailoverConfig configures the failover manager.
type FailoverConfig struct {
	// Enabled controls whether automatic failover is enabled.
	Enabled bool

	// LeaderTimeout is how long the leader can be unhealthy before failover.
	LeaderTimeout time.Duration

	// InhibitWindow is the minimum time between failovers.
	InhibitWindow time.Duration

	// HealthCheckInterval is how often to check leader health.
	HealthCheckInterval time.Duration
}

// DefaultFailoverConfig returns default failover configuration.
func DefaultFailoverConfig() *FailoverConfig {
	return &FailoverConfig{
		Enabled:             true,
		LeaderTimeout:       30 * time.Second,
		InhibitWindow:       60 * time.Second,
		HealthCheckInterval: 5 * time.Second,
	}
}

// FailoverEvent represents a failover event.
type FailoverEvent struct {
	Timestamp    time.Time `json:"timestamp"`
	OldLeaderID  string    `json:"old_leader_id"`
	NewLeaderID  string    `json:"new_leader_id"`
	Reason       string    `json:"reason"`
	Term         uint64    `json:"term"`
	Success      bool      `json:"success"`
	Error        string    `json:"error,omitempty"`
	DurationMS   int64     `json:"duration_ms"`
}

// FailoverManager manages automatic failover.
type FailoverManager struct {
	cfg            *FailoverConfig
	manager        ReplicationManager
	election       *Election
	logger         *zap.Logger

	// State
	mu               sync.RWMutex
	lastFailover     time.Time
	failoverInProgress bool
	leaderLastSeen   time.Time
	consecutiveFails int

	// History
	events []FailoverEvent

	// Control
	stopCh chan struct{}
}

// NewFailoverManager creates a new failover manager.
func NewFailoverManager(
	cfg *FailoverConfig,
	manager ReplicationManager,
	election *Election,
	logger *zap.Logger,
) *FailoverManager {
	if cfg == nil {
		cfg = DefaultFailoverConfig()
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	return &FailoverManager{
		cfg:            cfg,
		manager:        manager,
		election:       election,
		logger:         logger,
		leaderLastSeen: time.Now(),
		events:         make([]FailoverEvent, 0),
		stopCh:         make(chan struct{}),
	}
}

// Start begins the failover monitoring.
func (f *FailoverManager) Start(ctx context.Context) error {
	if !f.cfg.Enabled {
		f.logger.Info("automatic failover is disabled")
		return nil
	}

	go f.monitorLoop(ctx)
	return nil
}

// Stop stops the failover monitoring.
func (f *FailoverManager) Stop() {
	close(f.stopCh)
}

// TriggerFailover manually triggers a failover.
func (f *FailoverManager) TriggerFailover(ctx context.Context, targetFollowerID string) (*FailoverEvent, error) {
	f.mu.Lock()
	if f.failoverInProgress {
		f.mu.Unlock()
		return nil, ErrFailoverInProgress
	}
	if f.isInhibitedLocked() {
		f.mu.Unlock()
		return nil, ErrFailoverInhibited
	}
	f.failoverInProgress = true
	f.mu.Unlock()

	defer func() {
		f.mu.Lock()
		f.failoverInProgress = false
		f.mu.Unlock()
	}()

	return f.executeFailover(ctx, targetFollowerID, "manual trigger")
}

// IsInhibited returns true if failover is currently inhibited.
func (f *FailoverManager) IsInhibited() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.isInhibitedLocked()
}

// isInhibitedLocked checks inhibition without acquiring lock (must be called with lock held).
func (f *FailoverManager) isInhibitedLocked() bool {
	return time.Since(f.lastFailover) < f.cfg.InhibitWindow
}

// IsFailoverInProgress returns true if a failover is in progress.
func (f *FailoverManager) IsFailoverInProgress() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.failoverInProgress
}

// Events returns the failover event history.
func (f *FailoverManager) Events() []FailoverEvent {
	f.mu.RLock()
	defer f.mu.RUnlock()
	events := make([]FailoverEvent, len(f.events))
	copy(events, f.events)
	return events
}

// RecordLeaderHealthy records that the leader was seen healthy.
func (f *FailoverManager) RecordLeaderHealthy() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.leaderLastSeen = time.Now()
	f.consecutiveFails = 0
}

// RecordLeaderUnhealthy records a leader health check failure.
func (f *FailoverManager) RecordLeaderUnhealthy() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.consecutiveFails++
}

// monitorLoop runs the failover monitoring loop.
func (f *FailoverManager) monitorLoop(ctx context.Context) {
	ticker := time.NewTicker(f.cfg.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-f.stopCh:
			return
		case <-ticker.C:
			f.checkAndFailover(ctx)
		}
	}
}

// checkAndFailover checks leader health and triggers failover if needed.
func (f *FailoverManager) checkAndFailover(ctx context.Context) {
	// Only followers should monitor for failover
	if f.manager.Role() != RoleFollower {
		return
	}

	f.mu.RLock()
	leaderUnhealthyDuration := time.Since(f.leaderLastSeen)
	consecutiveFails := f.consecutiveFails
	inhibited := f.isInhibitedLocked()
	inProgress := f.failoverInProgress
	f.mu.RUnlock()

	// Check if leader has been unhealthy long enough
	if leaderUnhealthyDuration < f.cfg.LeaderTimeout {
		return
	}

	if inhibited {
		f.logger.Debug("failover inhibited, waiting",
			zap.Duration("inhibit_remaining", f.cfg.InhibitWindow-time.Since(f.lastFailover)))
		return
	}

	if inProgress {
		return
	}

	f.logger.Warn("leader unhealthy, initiating failover",
		zap.Duration("unhealthy_duration", leaderUnhealthyDuration),
		zap.Int("consecutive_fails", consecutiveFails))

	// Start failover
	f.mu.Lock()
	f.failoverInProgress = true
	f.mu.Unlock()

	event, err := f.executeFailover(ctx, "", "leader timeout")
	if err != nil {
		f.logger.Error("failover failed", zap.Error(err))
	} else {
		f.logger.Info("failover completed",
			zap.String("new_leader", event.NewLeaderID),
			zap.Duration("duration", time.Duration(event.DurationMS)*time.Millisecond))
	}

	f.mu.Lock()
	f.failoverInProgress = false
	f.mu.Unlock()
}

// executeFailover executes the failover process.
func (f *FailoverManager) executeFailover(ctx context.Context, targetFollowerID, reason string) (*FailoverEvent, error) {
	startTime := time.Now()
	oldLeaderID := ""

	// Get current leader info
	if leaderInfo, err := f.manager.GetLeaderInfo(ctx); err == nil && leaderInfo != nil {
		oldLeaderID = leaderInfo.ID
	}

	event := &FailoverEvent{
		Timestamp:   startTime,
		OldLeaderID: oldLeaderID,
		Reason:      reason,
	}

	defer func() {
		event.DurationMS = time.Since(startTime).Milliseconds()
		f.mu.Lock()
		f.events = append(f.events, *event)
		if len(f.events) > 100 {
			f.events = f.events[1:] // Keep last 100 events
		}
		f.lastFailover = time.Now()
		f.mu.Unlock()
	}()

	// If we have an election system, use it
	if f.election != nil {
		f.logger.Info("initiating leader election",
			zap.String("reason", reason))

		// Force this node to become leader (in real implementation,
		// this would start an election process)
		f.election.ForceLeadership()

		event.NewLeaderID = f.election.Info().LeaderID
		event.Term = f.election.Term()
		event.Success = true

		return event, nil
	}

	// Fallback: Just record the event without actually changing leadership
	event.Success = false
	event.Error = "no election system configured"

	return event, errors.New("no election system configured")
}

// FailoverStatus returns the current failover status.
type FailoverStatus struct {
	Enabled            bool          `json:"enabled"`
	InhibitWindow      time.Duration `json:"inhibit_window"`
	LeaderTimeout      time.Duration `json:"leader_timeout"`
	IsInhibited        bool          `json:"is_inhibited"`
	InProgress         bool          `json:"in_progress"`
	LastFailover       *time.Time    `json:"last_failover,omitempty"`
	LeaderLastSeen     time.Time     `json:"leader_last_seen"`
	ConsecutiveFails   int           `json:"consecutive_fails"`
	RecentEvents       int           `json:"recent_events"`
}

// Status returns the current failover status.
func (f *FailoverManager) Status() *FailoverStatus {
	f.mu.RLock()
	defer f.mu.RUnlock()

	status := &FailoverStatus{
		Enabled:          f.cfg.Enabled,
		InhibitWindow:    f.cfg.InhibitWindow,
		LeaderTimeout:    f.cfg.LeaderTimeout,
		IsInhibited:      time.Since(f.lastFailover) < f.cfg.InhibitWindow,
		InProgress:       f.failoverInProgress,
		LeaderLastSeen:   f.leaderLastSeen,
		ConsecutiveFails: f.consecutiveFails,
		RecentEvents:     len(f.events),
	}

	if !f.lastFailover.IsZero() {
		status.LastFailover = &f.lastFailover
	}

	return status
}
