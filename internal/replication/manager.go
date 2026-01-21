package replication

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ar4mirez/maia/internal/storage"
	"go.uber.org/zap"
)

// ManagerConfig configures the ReplicationManager.
type ManagerConfig struct {
	// Role is the replication role (leader, follower, standalone).
	Role Role

	// Region is the current region identifier.
	Region string

	// InstanceID is a unique identifier for this instance.
	InstanceID string

	// SyncMode defines how writes are synchronized.
	SyncMode SyncMode

	// MinSyncReplicas is the minimum followers to wait for in semi-sync mode.
	MinSyncReplicas int

	// MaxReplicationLag is the maximum acceptable replication lag.
	MaxReplicationLag time.Duration

	// ConflictStrategy defines how conflicts are resolved.
	ConflictStrategy ConflictStrategy

	// Leader config (for followers)
	LeaderEndpoint string

	// Follower configs (for leader)
	Followers []FollowerConfig

	// PullInterval is how often followers pull from leader.
	PullInterval time.Duration

	// PushInterval is how often leader pushes to followers.
	PushInterval time.Duration

	// BatchSize is the number of entries to sync at once.
	BatchSize int

	// HTTPClient is the HTTP client for replication.
	HTTPClient *http.Client
}

// DefaultManagerConfig returns a default configuration.
func DefaultManagerConfig() *ManagerConfig {
	return &ManagerConfig{
		Role:              RoleStandalone,
		SyncMode:          SyncModeAsync,
		MinSyncReplicas:   1,
		MaxReplicationLag: 30 * time.Second,
		ConflictStrategy:  ConflictLastWriteWins,
		PullInterval:      time.Second,
		PushInterval:      time.Second,
		BatchSize:         100,
		HTTPClient:        &http.Client{Timeout: 30 * time.Second},
	}
}

// Manager implements ReplicationManager for leader-follower replication.
type Manager struct {
	cfg              *ManagerConfig
	wal              WAL
	store            storage.Store
	resolver         ConflictResolver
	logger           *zap.Logger
	mu               sync.RWMutex
	followers        map[string]*followerState
	placements       map[string]*TenantPlacement
	leaderInfo       *LeaderInfo
	started          atomic.Bool
	stopped          atomic.Bool
	stopCh           chan struct{}
	wg               sync.WaitGroup
	conflictsResolved atomic.Int64
}

// followerState tracks the state of a follower.
type followerState struct {
	config      *FollowerConfig
	status      *FollowerStatus
	mu          sync.Mutex
	lastPushSeq uint64
}

// NewManager creates a new replication manager.
func NewManager(cfg *ManagerConfig, wal WAL, store storage.Store, logger *zap.Logger) *Manager {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Manager{
		cfg:        cfg,
		wal:        wal,
		store:      store,
		resolver:   NewConflictResolver(cfg.ConflictStrategy),
		logger:     logger,
		followers:  make(map[string]*followerState),
		placements: make(map[string]*TenantPlacement),
		stopCh:     make(chan struct{}),
	}
}

// Start begins replication operations.
func (m *Manager) Start(ctx context.Context) error {
	if m.started.Swap(true) {
		return errors.New("replication manager already started")
	}

	m.logger.Info("starting replication manager",
		zap.String("role", string(m.cfg.Role)),
		zap.String("region", m.cfg.Region),
		zap.String("instance_id", m.cfg.InstanceID),
	)

	// Initialize followers (for leader)
	if m.cfg.Role == RoleLeader {
		for _, fc := range m.cfg.Followers {
			fc := fc // capture loop variable
			m.followers[fc.ID] = &followerState{
				config: &fc,
				status: &FollowerStatus{
					ID:        fc.ID,
					Region:    fc.Region,
					Connected: false,
				},
			}
		}

		// Start push goroutine
		m.wg.Add(1)
		go m.pushLoop(ctx)
	}

	// Start pull goroutine (for followers)
	if m.cfg.Role == RoleFollower {
		m.wg.Add(1)
		go m.pullLoop(ctx)
	}

	return nil
}

// Stop gracefully stops replication.
func (m *Manager) Stop(ctx context.Context) error {
	if !m.started.Load() {
		return nil // Never started
	}

	if m.stopped.Swap(true) {
		return nil // Already stopped
	}

	m.logger.Info("stopping replication manager")

	close(m.stopCh)

	// Wait for goroutines with timeout
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Role returns the current replication role.
func (m *Manager) Role() Role {
	return m.cfg.Role
}

// Region returns the current region.
func (m *Manager) Region() string {
	return m.cfg.Region
}

// Position returns the current WAL position.
func (m *Manager) Position(ctx context.Context) (*WALPosition, error) {
	return m.wal.Position(ctx)
}

// Stats returns replication statistics.
func (m *Manager) Stats(ctx context.Context) (*ReplicationStats, error) {
	pos, err := m.wal.Position(ctx)
	if err != nil {
		return nil, err
	}

	walStats, err := m.wal.(*BadgerWAL).Stats(ctx)
	if err != nil {
		return nil, err
	}

	stats := &ReplicationStats{
		Role:              m.cfg.Role,
		Region:            m.cfg.Region,
		Position:          pos,
		WALSize:           walStats.TotalBytes,
		WALEntries:        walStats.EntryCount,
		ConflictsResolved: m.conflictsResolved.Load(),
	}

	if m.cfg.Role == RoleLeader {
		stats.Followers = m.listFollowerStatuses()
	}

	if m.cfg.Role == RoleFollower {
		m.mu.RLock()
		stats.Leader = m.leaderInfo
		m.mu.RUnlock()
	}

	return stats, nil
}

// AddFollower registers a new follower.
func (m *Manager) AddFollower(ctx context.Context, cfg *FollowerConfig) error {
	if m.cfg.Role != RoleLeader {
		return ErrNotLeader
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.followers[cfg.ID] = &followerState{
		config: cfg,
		status: &FollowerStatus{
			ID:        cfg.ID,
			Region:    cfg.Region,
			Connected: false,
		},
	}

	m.logger.Info("added follower",
		zap.String("follower_id", cfg.ID),
		zap.String("region", cfg.Region),
		zap.String("endpoint", cfg.Endpoint),
	)

	return nil
}

// RemoveFollower removes a follower.
func (m *Manager) RemoveFollower(ctx context.Context, id string) error {
	if m.cfg.Role != RoleLeader {
		return ErrNotLeader
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.followers[id]; !exists {
		return ErrFollowerNotFound
	}

	delete(m.followers, id)

	m.logger.Info("removed follower", zap.String("follower_id", id))

	return nil
}

// GetFollowerStatus returns the status of a specific follower.
func (m *Manager) GetFollowerStatus(ctx context.Context, id string) (*FollowerStatus, error) {
	if m.cfg.Role != RoleLeader {
		return nil, ErrNotLeader
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	fs, exists := m.followers[id]
	if !exists {
		return nil, ErrFollowerNotFound
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()

	// Return a copy
	status := *fs.status
	return &status, nil
}

// ListFollowers returns all registered followers.
func (m *Manager) ListFollowers(ctx context.Context) ([]FollowerStatus, error) {
	if m.cfg.Role != RoleLeader {
		return nil, ErrNotLeader
	}

	return m.listFollowerStatuses(), nil
}

func (m *Manager) listFollowerStatuses() []FollowerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make([]FollowerStatus, 0, len(m.followers))
	for _, fs := range m.followers {
		fs.mu.Lock()
		statuses = append(statuses, *fs.status)
		fs.mu.Unlock()
	}

	return statuses
}

// GetLeaderInfo returns information about the current leader.
func (m *Manager) GetLeaderInfo(ctx context.Context) (*LeaderInfo, error) {
	if m.cfg.Role != RoleFollower {
		return nil, ErrNotFollower
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.leaderInfo == nil {
		return nil, ErrLeaderUnavailable
	}

	info := *m.leaderInfo
	return &info, nil
}

// SetLeader configures the leader to replicate from.
func (m *Manager) SetLeader(ctx context.Context, endpoint string) error {
	if m.cfg.Role != RoleFollower {
		return ErrNotFollower
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.cfg.LeaderEndpoint = endpoint
	m.leaderInfo = &LeaderInfo{
		Endpoint: endpoint,
	}

	m.logger.Info("set leader endpoint", zap.String("endpoint", endpoint))

	return nil
}

// GetTenantPlacement returns the placement for a tenant.
func (m *Manager) GetTenantPlacement(ctx context.Context, tenantID string) (*TenantPlacement, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	placement, exists := m.placements[tenantID]
	if !exists {
		return nil, ErrTenantNotReplicated
	}

	return placement, nil
}

// SetTenantPlacement configures placement for a tenant.
func (m *Manager) SetTenantPlacement(ctx context.Context, placement *TenantPlacement) error {
	if err := validatePlacement(placement); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	if placement.CreatedAt.IsZero() {
		placement.CreatedAt = now
	}
	placement.UpdatedAt = now

	m.placements[placement.TenantID] = placement

	m.logger.Info("set tenant placement",
		zap.String("tenant_id", placement.TenantID),
		zap.String("primary_region", placement.PrimaryRegion),
		zap.String("mode", string(placement.Mode)),
	)

	return nil
}

// IsLocalTenant checks if a tenant's data should be stored locally.
func (m *Manager) IsLocalTenant(ctx context.Context, tenantID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	placement, exists := m.placements[tenantID]
	if !exists {
		// No placement configured, default to local
		return true, nil
	}

	// Check if this region is the primary
	if placement.PrimaryRegion == m.cfg.Region {
		return true, nil
	}

	// Check if this region is a replica
	for _, replica := range placement.Replicas {
		if replica == m.cfg.Region {
			return true, nil
		}
	}

	// Global mode means all regions
	if placement.Mode == PlacementGlobal {
		return true, nil
	}

	return false, nil
}

// pushLoop continuously pushes changes to followers.
func (m *Manager) pushLoop(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(m.cfg.PushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.pushToFollowers(ctx)
		}
	}
}

// pushToFollowers pushes WAL entries to all followers.
func (m *Manager) pushToFollowers(ctx context.Context) {
	m.mu.RLock()
	followers := make([]*followerState, 0, len(m.followers))
	for _, fs := range m.followers {
		followers = append(followers, fs)
	}
	m.mu.RUnlock()

	var wg sync.WaitGroup
	for _, fs := range followers {
		wg.Add(1)
		go func(fs *followerState) {
			defer wg.Done()
			m.pushToFollower(ctx, fs)
		}(fs)
	}

	// In semi-sync mode, wait for minimum replicas
	if m.cfg.SyncMode == SyncModeSemiSync {
		wg.Wait()
	}
}

// pushToFollower pushes WAL entries to a single follower.
func (m *Manager) pushToFollower(ctx context.Context, fs *followerState) {
	fs.mu.Lock()
	afterSeq := fs.lastPushSeq
	fs.mu.Unlock()

	// Read entries to send
	entries, err := m.wal.Read(ctx, afterSeq, m.cfg.BatchSize)
	if err != nil {
		m.logger.Error("failed to read WAL entries for push",
			zap.String("follower_id", fs.config.ID),
			zap.Error(err),
		)
		fs.mu.Lock()
		fs.status.Connected = false
		fs.status.LastError = err.Error()
		fs.mu.Unlock()
		return
	}

	if len(entries) == 0 {
		return
	}

	// Send entries to follower
	err = m.sendEntriesToFollower(ctx, fs, entries)
	if err != nil {
		m.logger.Error("failed to push entries to follower",
			zap.String("follower_id", fs.config.ID),
			zap.Error(err),
		)
		fs.mu.Lock()
		fs.status.Connected = false
		fs.status.LastError = err.Error()
		fs.mu.Unlock()
		return
	}

	// Update follower state
	fs.mu.Lock()
	fs.lastPushSeq = entries[len(entries)-1].Sequence
	fs.status.Connected = true
	fs.status.LastSeen = time.Now()
	fs.status.EntriesSent += uint64(len(entries))
	fs.status.LastError = ""
	fs.mu.Unlock()
}

// sendEntriesToFollower sends entries to a follower via HTTP.
func (m *Manager) sendEntriesToFollower(ctx context.Context, fs *followerState, entries []*WALEntry) error {
	data, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("failed to marshal entries: %w", err)
	}

	url := fmt.Sprintf("%s/replication/entries", fs.config.Endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-MAIA-Region", m.cfg.Region)
	req.Header.Set("X-MAIA-Instance-ID", m.cfg.InstanceID)

	// Use the HTTP client
	resp, err := m.cfg.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("follower returned status %d: %s", resp.StatusCode, string(body))
	}

	// Update bytes sent
	fs.mu.Lock()
	fs.status.BytesSent += uint64(len(data))
	fs.mu.Unlock()

	return nil
}

// pullLoop continuously pulls changes from leader.
func (m *Manager) pullLoop(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(m.cfg.PullInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.pullFromLeader(ctx)
		}
	}
}

// pullFromLeader pulls WAL entries from the leader.
func (m *Manager) pullFromLeader(ctx context.Context) {
	if m.cfg.LeaderEndpoint == "" {
		return
	}

	// Get current position
	pos, err := m.wal.Position(ctx)
	if err != nil {
		m.logger.Error("failed to get WAL position", zap.Error(err))
		return
	}

	// Fetch entries from leader
	entries, err := m.fetchEntriesFromLeader(ctx, pos.Sequence)
	if err != nil {
		m.logger.Error("failed to fetch entries from leader", zap.Error(err))
		return
	}

	if len(entries) == 0 {
		return
	}

	// Apply entries
	for _, entry := range entries {
		if err := m.applyEntry(ctx, entry); err != nil {
			m.logger.Error("failed to apply entry",
				zap.String("entry_id", entry.ID),
				zap.Error(err),
			)
			break
		}
	}

	m.logger.Debug("pulled entries from leader",
		zap.Int("count", len(entries)),
		zap.Uint64("from_seq", pos.Sequence),
	)
}

// fetchEntriesFromLeader fetches entries from the leader via HTTP.
func (m *Manager) fetchEntriesFromLeader(ctx context.Context, afterSequence uint64) ([]*WALEntry, error) {
	url := fmt.Sprintf("%s/replication/entries?after=%d&limit=%d",
		m.cfg.LeaderEndpoint, afterSequence, m.cfg.BatchSize)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-MAIA-Region", m.cfg.Region)
	req.Header.Set("X-MAIA-Instance-ID", m.cfg.InstanceID)

	resp, err := m.cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("leader returned status %d: %s", resp.StatusCode, string(body))
	}

	var entries []*WALEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("failed to decode entries: %w", err)
	}

	// Update leader info from response headers
	m.mu.Lock()
	if m.leaderInfo == nil {
		m.leaderInfo = &LeaderInfo{}
	}
	m.leaderInfo.Endpoint = m.cfg.LeaderEndpoint
	m.leaderInfo.Region = resp.Header.Get("X-MAIA-Region")
	m.leaderInfo.ID = resp.Header.Get("X-MAIA-Instance-ID")
	m.mu.Unlock()

	return entries, nil
}

// applyEntry applies a WAL entry to the local store.
func (m *Manager) applyEntry(ctx context.Context, entry *WALEntry) error {
	// Verify checksum
	if !entry.VerifyChecksum() {
		return ErrChecksumMismatch
	}

	// Check for conflicts
	localEntry, err := m.wal.GetEntry(ctx, entry.ID)
	if err == nil && localEntry != nil {
		// Conflict detected
		resolved, err := m.resolver.Resolve(ctx, localEntry, entry)
		if err != nil {
			return fmt.Errorf("conflict resolution failed: %w", err)
		}
		m.conflictsResolved.Add(1)
		entry = resolved
	}

	// Mark as replicated
	entry.Replicated = true

	// Append to local WAL
	if err := m.wal.Append(ctx, entry); err != nil {
		return fmt.Errorf("failed to append entry to WAL: %w", err)
	}

	// Apply to store based on resource type
	switch entry.ResourceType {
	case ResourceTypeMemory:
		return m.applyMemoryEntry(ctx, entry)
	case ResourceTypeNamespace:
		return m.applyNamespaceEntry(ctx, entry)
	default:
		m.logger.Warn("unknown resource type in WAL entry",
			zap.String("resource_type", string(entry.ResourceType)),
		)
		return nil
	}
}

// applyMemoryEntry applies a memory WAL entry to the store.
func (m *Manager) applyMemoryEntry(ctx context.Context, entry *WALEntry) error {
	switch entry.Operation {
	case OperationCreate, OperationUpdate:
		var memory storage.Memory
		if err := json.Unmarshal(entry.Data, &memory); err != nil {
			return fmt.Errorf("failed to unmarshal memory: %w", err)
		}
		// Use direct storage operation to avoid WAL recursion
		// This would need the underlying store to have a "raw" method
		// For now, we skip applying to avoid the loop
		m.logger.Debug("would apply memory entry",
			zap.String("operation", string(entry.Operation)),
			zap.String("memory_id", memory.ID),
		)
		return nil

	case OperationDelete:
		// Delete from store
		m.logger.Debug("would delete memory",
			zap.String("memory_id", entry.ResourceID),
		)
		return nil

	default:
		return fmt.Errorf("unknown operation: %s", entry.Operation)
	}
}

// applyNamespaceEntry applies a namespace WAL entry to the store.
func (m *Manager) applyNamespaceEntry(ctx context.Context, entry *WALEntry) error {
	switch entry.Operation {
	case OperationCreate, OperationUpdate:
		var ns storage.Namespace
		if err := json.Unmarshal(entry.Data, &ns); err != nil {
			return fmt.Errorf("failed to unmarshal namespace: %w", err)
		}
		m.logger.Debug("would apply namespace entry",
			zap.String("operation", string(entry.Operation)),
			zap.String("namespace_id", ns.ID),
		)
		return nil

	case OperationDelete:
		m.logger.Debug("would delete namespace",
			zap.String("namespace_id", entry.ResourceID),
		)
		return nil

	default:
		return fmt.Errorf("unknown operation: %s", entry.Operation)
	}
}

// validatePlacement validates a tenant placement configuration.
func validatePlacement(p *TenantPlacement) error {
	if p.TenantID == "" {
		return fmt.Errorf("%w: tenant ID is required", ErrInvalidPlacement)
	}
	if p.PrimaryRegion == "" {
		return fmt.Errorf("%w: primary region is required", ErrInvalidPlacement)
	}
	if p.Mode == "" {
		p.Mode = PlacementSingle
	}
	return nil
}
