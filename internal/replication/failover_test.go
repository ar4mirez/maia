package replication

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestFailoverManager_NewFailoverManager(t *testing.T) {
	manager := newMockManager()
	election := NewElection(&ElectionConfig{NodeID: "node-1"}, nil)

	fm := NewFailoverManager(nil, manager, election, nil)

	assert.NotNil(t, fm)
	assert.True(t, fm.cfg.Enabled) // Default is enabled
	assert.Equal(t, 30*time.Second, fm.cfg.LeaderTimeout)
	assert.Equal(t, 60*time.Second, fm.cfg.InhibitWindow)
}

func TestFailoverManager_WithConfig(t *testing.T) {
	manager := newMockManager()

	cfg := &FailoverConfig{
		Enabled:             true,
		LeaderTimeout:       10 * time.Second,
		InhibitWindow:       30 * time.Second,
		HealthCheckInterval: 1 * time.Second,
	}

	fm := NewFailoverManager(cfg, manager, nil, zap.NewNop())

	assert.Equal(t, 10*time.Second, fm.cfg.LeaderTimeout)
	assert.Equal(t, 30*time.Second, fm.cfg.InhibitWindow)
}

func TestFailoverManager_IsInhibited(t *testing.T) {
	manager := newMockManager()
	fm := NewFailoverManager(&FailoverConfig{
		Enabled:       true,
		InhibitWindow: 100 * time.Millisecond,
	}, manager, nil, nil)

	// Initially not inhibited
	assert.False(t, fm.IsInhibited())

	// Simulate a failover
	fm.mu.Lock()
	fm.lastFailover = time.Now()
	fm.mu.Unlock()

	// Should be inhibited
	assert.True(t, fm.IsInhibited())

	// Wait for inhibit window to pass
	time.Sleep(150 * time.Millisecond)
	assert.False(t, fm.IsInhibited())
}

func TestFailoverManager_IsFailoverInProgress(t *testing.T) {
	manager := newMockManager()
	fm := NewFailoverManager(nil, manager, nil, nil)

	assert.False(t, fm.IsFailoverInProgress())

	fm.mu.Lock()
	fm.failoverInProgress = true
	fm.mu.Unlock()

	assert.True(t, fm.IsFailoverInProgress())
}

func TestFailoverManager_RecordLeaderHealthy(t *testing.T) {
	manager := newMockManager()
	fm := NewFailoverManager(nil, manager, nil, nil)

	// Set some failures
	fm.mu.Lock()
	fm.consecutiveFails = 5
	fm.leaderLastSeen = time.Now().Add(-1 * time.Hour)
	fm.mu.Unlock()

	fm.RecordLeaderHealthy()

	fm.mu.RLock()
	assert.Equal(t, 0, fm.consecutiveFails)
	assert.True(t, time.Since(fm.leaderLastSeen) < time.Second)
	fm.mu.RUnlock()
}

func TestFailoverManager_RecordLeaderUnhealthy(t *testing.T) {
	manager := newMockManager()
	fm := NewFailoverManager(nil, manager, nil, nil)

	fm.RecordLeaderUnhealthy()
	fm.RecordLeaderUnhealthy()
	fm.RecordLeaderUnhealthy()

	fm.mu.RLock()
	assert.Equal(t, 3, fm.consecutiveFails)
	fm.mu.RUnlock()
}

func TestFailoverManager_TriggerFailover_WithElection(t *testing.T) {
	manager := newMockManager()
	election := NewElection(&ElectionConfig{NodeID: "node-1", Nodes: []string{"node-1"}}, nil)

	fm := NewFailoverManager(&FailoverConfig{
		Enabled:       true,
		InhibitWindow: 50 * time.Millisecond,
	}, manager, election, zap.NewNop())

	ctx := context.Background()
	event, err := fm.TriggerFailover(ctx, "")

	require.NoError(t, err)
	assert.True(t, event.Success)
	assert.Equal(t, "manual trigger", event.Reason)
	assert.Equal(t, "node-1", event.NewLeaderID)
	assert.True(t, election.IsLeader())
}

func TestFailoverManager_TriggerFailover_AlreadyInProgress(t *testing.T) {
	manager := newMockManager()
	fm := NewFailoverManager(nil, manager, nil, nil)

	fm.mu.Lock()
	fm.failoverInProgress = true
	fm.mu.Unlock()

	ctx := context.Background()
	_, err := fm.TriggerFailover(ctx, "")

	assert.ErrorIs(t, err, ErrFailoverInProgress)
}

func TestFailoverManager_TriggerFailover_Inhibited(t *testing.T) {
	manager := newMockManager()
	fm := NewFailoverManager(&FailoverConfig{
		Enabled:       true,
		InhibitWindow: 1 * time.Hour,
	}, manager, nil, nil)

	fm.mu.Lock()
	fm.lastFailover = time.Now()
	fm.mu.Unlock()

	ctx := context.Background()
	_, err := fm.TriggerFailover(ctx, "")

	assert.ErrorIs(t, err, ErrFailoverInhibited)
}

func TestFailoverManager_TriggerFailover_NoElection(t *testing.T) {
	manager := newMockManager()
	fm := NewFailoverManager(&FailoverConfig{
		Enabled:       true,
		InhibitWindow: 10 * time.Millisecond,
	}, manager, nil, zap.NewNop())

	ctx := context.Background()
	event, err := fm.TriggerFailover(ctx, "")

	require.Error(t, err)
	assert.False(t, event.Success)
	assert.Contains(t, event.Error, "no election system")
}

func TestFailoverManager_Events(t *testing.T) {
	manager := newMockManager()
	election := NewElection(&ElectionConfig{NodeID: "node-1", Nodes: []string{"node-1"}}, nil)

	fm := NewFailoverManager(&FailoverConfig{
		Enabled:       true,
		InhibitWindow: 10 * time.Millisecond,
	}, manager, election, nil)

	ctx := context.Background()

	// Trigger multiple failovers
	_, _ = fm.TriggerFailover(ctx, "")
	time.Sleep(20 * time.Millisecond)
	_, _ = fm.TriggerFailover(ctx, "")
	time.Sleep(20 * time.Millisecond)
	_, _ = fm.TriggerFailover(ctx, "")

	events := fm.Events()
	assert.Len(t, events, 3)
}

func TestFailoverManager_Status(t *testing.T) {
	manager := newMockManager()
	fm := NewFailoverManager(&FailoverConfig{
		Enabled:             true,
		LeaderTimeout:       30 * time.Second,
		InhibitWindow:       60 * time.Second,
		HealthCheckInterval: 5 * time.Second,
	}, manager, nil, nil)

	fm.RecordLeaderUnhealthy()
	fm.RecordLeaderUnhealthy()

	status := fm.Status()

	assert.True(t, status.Enabled)
	assert.Equal(t, 60*time.Second, status.InhibitWindow)
	assert.Equal(t, 30*time.Second, status.LeaderTimeout)
	assert.False(t, status.IsInhibited)
	assert.False(t, status.InProgress)
	assert.Equal(t, 2, status.ConsecutiveFails)
}

func TestFailoverManager_Start_Disabled(t *testing.T) {
	manager := newMockManager()
	fm := NewFailoverManager(&FailoverConfig{
		Enabled: false,
	}, manager, nil, zap.NewNop())

	ctx := context.Background()
	err := fm.Start(ctx)

	require.NoError(t, err)
}

func TestFailoverManager_Start_Stop(t *testing.T) {
	manager := newMockManager()
	fm := NewFailoverManager(&FailoverConfig{
		Enabled:             true,
		HealthCheckInterval: 10 * time.Millisecond,
	}, manager, nil, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := fm.Start(ctx)
	require.NoError(t, err)

	// Let it run briefly
	time.Sleep(50 * time.Millisecond)

	// Stop
	fm.Stop()
}

func TestFailoverManager_EventHistoryLimit(t *testing.T) {
	manager := newMockManager()
	election := NewElection(&ElectionConfig{NodeID: "node-1", Nodes: []string{"node-1"}}, nil)

	fm := NewFailoverManager(&FailoverConfig{
		Enabled:       true,
		InhibitWindow: 1 * time.Millisecond,
	}, manager, election, nil)

	ctx := context.Background()

	// Trigger many failovers
	for i := 0; i < 150; i++ {
		time.Sleep(2 * time.Millisecond)
		_, _ = fm.TriggerFailover(ctx, "")
	}

	events := fm.Events()
	// Should be limited to 100
	assert.LessOrEqual(t, len(events), 100)
}
