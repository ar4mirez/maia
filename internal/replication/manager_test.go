package replication

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestManager(t *testing.T, role Role) (*Manager, *BadgerWAL, func()) {
	t.Helper()

	dir, err := os.MkdirTemp("", "manager-test-*")
	require.NoError(t, err)

	walOpts := &BadgerWALOptions{
		DataDir:    dir,
		Region:     "test-region",
		Logger:     zap.NewNop(),
		SyncWrites: false,
	}

	wal, err := NewBadgerWAL(walOpts)
	require.NoError(t, err)

	cfg := &ManagerConfig{
		Role:              role,
		Region:            "test-region",
		InstanceID:        "test-instance",
		SyncMode:          SyncModeAsync,
		MinSyncReplicas:   1,
		MaxReplicationLag: 30 * time.Second,
		ConflictStrategy:  ConflictLastWriteWins,
		PullInterval:      time.Second,
		PushInterval:      time.Second,
		BatchSize:         100,
	}

	manager := NewManager(cfg, wal, nil, zap.NewNop())

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = manager.Stop(ctx)
		_ = wal.Close()
		os.RemoveAll(dir)
	}

	return manager, wal, cleanup
}

func TestManager_Role(t *testing.T) {
	tests := []struct {
		name string
		role Role
	}{
		{"leader", RoleLeader},
		{"follower", RoleFollower},
		{"standalone", RoleStandalone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, _, cleanup := setupTestManager(t, tt.role)
			defer cleanup()

			assert.Equal(t, tt.role, manager.Role())
		})
	}
}

func TestManager_Region(t *testing.T) {
	manager, _, cleanup := setupTestManager(t, RoleLeader)
	defer cleanup()

	assert.Equal(t, "test-region", manager.Region())
}

func TestManager_StartStop(t *testing.T) {
	manager, _, cleanup := setupTestManager(t, RoleLeader)
	defer cleanup()

	ctx := context.Background()

	err := manager.Start(ctx)
	require.NoError(t, err)

	// Starting again should fail
	err = manager.Start(ctx)
	assert.Error(t, err)

	err = manager.Stop(ctx)
	require.NoError(t, err)
}

func TestManager_Position(t *testing.T) {
	manager, wal, cleanup := setupTestManager(t, RoleLeader)
	defer cleanup()

	ctx := context.Background()

	// Initial position
	pos, err := manager.Position(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), pos.Sequence)

	// Add entries
	entry := &WALEntry{
		TenantID:     "tenant-1",
		Operation:    OperationCreate,
		ResourceType: ResourceTypeMemory,
		ResourceID:   "mem-123",
	}
	err = wal.Append(ctx, entry)
	require.NoError(t, err)

	pos, err = manager.Position(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), pos.Sequence)
}

func TestManager_Stats(t *testing.T) {
	manager, wal, cleanup := setupTestManager(t, RoleLeader)
	defer cleanup()

	ctx := context.Background()

	// Add some entries
	for i := 0; i < 3; i++ {
		entry := &WALEntry{
			TenantID:     "tenant-1",
			Operation:    OperationCreate,
			ResourceType: ResourceTypeMemory,
			ResourceID:   "mem-" + string(rune('a'+i)),
		}
		err := wal.Append(ctx, entry)
		require.NoError(t, err)
	}

	stats, err := manager.Stats(ctx)
	require.NoError(t, err)
	assert.Equal(t, RoleLeader, stats.Role)
	assert.Equal(t, "test-region", stats.Region)
	// WAL entries includes metadata entries as well
	assert.GreaterOrEqual(t, stats.WALEntries, int64(3))
}

func TestManager_LeaderFollowerManagement(t *testing.T) {
	manager, _, cleanup := setupTestManager(t, RoleLeader)
	defer cleanup()

	ctx := context.Background()

	// Add follower
	cfg := &FollowerConfig{
		ID:       "follower-1",
		Endpoint: "http://localhost:8081",
		Region:   "us-west-2",
		Priority: 1,
	}
	err := manager.AddFollower(ctx, cfg)
	require.NoError(t, err)

	// List followers
	followers, err := manager.ListFollowers(ctx)
	require.NoError(t, err)
	assert.Len(t, followers, 1)
	assert.Equal(t, "follower-1", followers[0].ID)

	// Get follower status
	status, err := manager.GetFollowerStatus(ctx, "follower-1")
	require.NoError(t, err)
	assert.Equal(t, "follower-1", status.ID)
	assert.Equal(t, "us-west-2", status.Region)
	assert.False(t, status.Connected)

	// Remove follower
	err = manager.RemoveFollower(ctx, "follower-1")
	require.NoError(t, err)

	// List should be empty
	followers, err = manager.ListFollowers(ctx)
	require.NoError(t, err)
	assert.Len(t, followers, 0)

	// Remove non-existent should fail
	err = manager.RemoveFollower(ctx, "non-existent")
	assert.ErrorIs(t, err, ErrFollowerNotFound)
}

func TestManager_LeaderOperationsOnFollower(t *testing.T) {
	manager, _, cleanup := setupTestManager(t, RoleFollower)
	defer cleanup()

	ctx := context.Background()

	// Leader operations should fail on follower
	cfg := &FollowerConfig{ID: "test"}
	err := manager.AddFollower(ctx, cfg)
	assert.ErrorIs(t, err, ErrNotLeader)

	err = manager.RemoveFollower(ctx, "test")
	assert.ErrorIs(t, err, ErrNotLeader)

	_, err = manager.GetFollowerStatus(ctx, "test")
	assert.ErrorIs(t, err, ErrNotLeader)

	_, err = manager.ListFollowers(ctx)
	assert.ErrorIs(t, err, ErrNotLeader)
}

func TestManager_FollowerOperationsOnLeader(t *testing.T) {
	manager, _, cleanup := setupTestManager(t, RoleLeader)
	defer cleanup()

	ctx := context.Background()

	// Follower operations should fail on leader
	_, err := manager.GetLeaderInfo(ctx)
	assert.ErrorIs(t, err, ErrNotFollower)

	err = manager.SetLeader(ctx, "http://localhost:8080")
	assert.ErrorIs(t, err, ErrNotFollower)
}

func TestManager_SetLeader(t *testing.T) {
	manager, _, cleanup := setupTestManager(t, RoleFollower)
	defer cleanup()

	ctx := context.Background()

	// Initially no leader
	_, err := manager.GetLeaderInfo(ctx)
	assert.ErrorIs(t, err, ErrLeaderUnavailable)

	// Set leader
	err = manager.SetLeader(ctx, "http://localhost:8080")
	require.NoError(t, err)

	info, err := manager.GetLeaderInfo(ctx)
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:8080", info.Endpoint)
}

func TestManager_TenantPlacement(t *testing.T) {
	manager, _, cleanup := setupTestManager(t, RoleLeader)
	defer cleanup()

	ctx := context.Background()

	// Get non-existent placement
	_, err := manager.GetTenantPlacement(ctx, "tenant-1")
	assert.ErrorIs(t, err, ErrTenantNotReplicated)

	// Set placement
	placement := &TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-east-1",
		Replicas:      []string{"us-west-2", "eu-central-1"},
		Mode:          PlacementReplicated,
	}
	err = manager.SetTenantPlacement(ctx, placement)
	require.NoError(t, err)

	// Get placement
	retrieved, err := manager.GetTenantPlacement(ctx, "tenant-1")
	require.NoError(t, err)
	assert.Equal(t, "tenant-1", retrieved.TenantID)
	assert.Equal(t, "us-east-1", retrieved.PrimaryRegion)
	assert.Equal(t, PlacementReplicated, retrieved.Mode)
	assert.NotZero(t, retrieved.CreatedAt)
	assert.NotZero(t, retrieved.UpdatedAt)
}

func TestManager_IsLocalTenant(t *testing.T) {
	manager, _, cleanup := setupTestManager(t, RoleLeader)
	defer cleanup()

	ctx := context.Background()

	// No placement = local
	isLocal, err := manager.IsLocalTenant(ctx, "tenant-1")
	require.NoError(t, err)
	assert.True(t, isLocal)

	// Set placement with primary in this region
	placement := &TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "test-region",
		Mode:          PlacementSingle,
	}
	err = manager.SetTenantPlacement(ctx, placement)
	require.NoError(t, err)

	isLocal, err = manager.IsLocalTenant(ctx, "tenant-1")
	require.NoError(t, err)
	assert.True(t, isLocal)

	// Set placement with primary in different region
	placement.PrimaryRegion = "other-region"
	err = manager.SetTenantPlacement(ctx, placement)
	require.NoError(t, err)

	isLocal, err = manager.IsLocalTenant(ctx, "tenant-1")
	require.NoError(t, err)
	assert.False(t, isLocal)

	// Add this region as replica
	placement.Replicas = []string{"test-region"}
	err = manager.SetTenantPlacement(ctx, placement)
	require.NoError(t, err)

	isLocal, err = manager.IsLocalTenant(ctx, "tenant-1")
	require.NoError(t, err)
	assert.True(t, isLocal)

	// Global mode = always local
	placement.Mode = PlacementGlobal
	placement.Replicas = nil
	err = manager.SetTenantPlacement(ctx, placement)
	require.NoError(t, err)

	isLocal, err = manager.IsLocalTenant(ctx, "tenant-1")
	require.NoError(t, err)
	assert.True(t, isLocal)
}

func TestManager_InvalidPlacement(t *testing.T) {
	manager, _, cleanup := setupTestManager(t, RoleLeader)
	defer cleanup()

	ctx := context.Background()

	// Missing tenant ID
	placement := &TenantPlacement{
		PrimaryRegion: "us-east-1",
	}
	err := manager.SetTenantPlacement(ctx, placement)
	assert.Error(t, err)

	// Missing primary region
	placement = &TenantPlacement{
		TenantID: "tenant-1",
	}
	err = manager.SetTenantPlacement(ctx, placement)
	assert.Error(t, err)
}
