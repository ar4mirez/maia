package replication

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupMigrationTest(t *testing.T) (*MigrationExecutor, *mockManagerWithPlacement, func()) {
	t.Helper()

	// Create temp directory for BadgerDB
	tmpDir, err := os.MkdirTemp("", "migration-test-*")
	require.NoError(t, err)

	// Open BadgerDB
	db, err := badger.Open(badger.DefaultOptions(filepath.Join(tmpDir, "migration")).
		WithLogger(nil))
	require.NoError(t, err)

	// Create mock manager with placement support
	manager := newMockManagerWithPlacement("us-west-1")

	// Create mock WAL
	mockWAL := &mockWAL{
		position: &WALPosition{
			Sequence: 100,
			EntryID:  "entry-100",
		},
	}

	executor := NewMigrationExecutor(&MigrationExecutorConfig{
		DB:      db,
		Manager: manager,
		WAL:     mockWAL,
		Logger:  zap.NewNop(),
	})

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return executor, manager, cleanup
}

// mockManagerWithPlacement extends mockManager with placement support
type mockManagerWithPlacement struct {
	*mockManager
	region     string
	placements map[string]*TenantPlacement
}

func newMockManagerWithPlacement(region string) *mockManagerWithPlacement {
	return &mockManagerWithPlacement{
		mockManager: newMockManager(),
		region:      region,
		placements:  make(map[string]*TenantPlacement),
	}
}

func (m *mockManagerWithPlacement) Region() string {
	return m.region
}

func (m *mockManagerWithPlacement) GetTenantPlacement(_ context.Context, tenantID string) (*TenantPlacement, error) {
	if p, ok := m.placements[tenantID]; ok {
		return p, nil
	}
	return nil, ErrInvalidPlacement
}

func (m *mockManagerWithPlacement) SetTenantPlacement(_ context.Context, placement *TenantPlacement) error {
	m.placements[placement.TenantID] = placement
	return nil
}

// mockWAL implements WAL for testing
type mockWAL struct {
	position *WALPosition
	entries  []*WALEntry
}

func (w *mockWAL) Append(_ context.Context, entry *WALEntry) error {
	w.entries = append(w.entries, entry)
	return nil
}

func (w *mockWAL) Read(_ context.Context, _ uint64, _ int) ([]*WALEntry, error) {
	return w.entries, nil
}

func (w *mockWAL) ReadByID(_ context.Context, _ string, _ int) ([]*WALEntry, error) {
	return w.entries, nil
}

func (w *mockWAL) GetEntry(_ context.Context, _ string) (*WALEntry, error) {
	return nil, nil
}

func (w *mockWAL) Position(_ context.Context) (*WALPosition, error) {
	return w.position, nil
}

func (w *mockWAL) Truncate(_ context.Context, _ uint64) error {
	return nil
}

func (w *mockWAL) Sync(_ context.Context) error {
	return nil
}

func (w *mockWAL) Close() error {
	return nil
}

func TestMigrationExecutor_StartMigration_DryRun(t *testing.T) {
	executor, manager, cleanup := setupMigrationTest(t)
	defer cleanup()

	// Set up initial placement
	manager.placements["tenant-1"] = &TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-west-1",
		Mode:          PlacementSingle,
	}

	ctx := context.Background()
	migration, err := executor.StartMigration(ctx, "tenant-1", "eu-central-1", true)

	require.NoError(t, err)
	assert.NotEmpty(t, migration.ID)
	assert.Equal(t, "tenant-1", migration.TenantID)
	assert.Equal(t, "us-west-1", migration.FromRegion)
	assert.Equal(t, "eu-central-1", migration.ToRegion)
	assert.Equal(t, MigrationStateCompleted, migration.State)
	assert.True(t, migration.DryRun)
	assert.Equal(t, 100, migration.Progress)
}

func TestMigrationExecutor_StartMigration_Execute(t *testing.T) {
	executor, manager, cleanup := setupMigrationTest(t)
	defer cleanup()

	// Set up initial placement
	manager.placements["tenant-1"] = &TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-west-1",
		Mode:          PlacementSingle,
	}

	ctx := context.Background()
	migration, err := executor.StartMigration(ctx, "tenant-1", "eu-central-1", false)

	require.NoError(t, err)
	assert.NotEmpty(t, migration.ID)
	assert.Equal(t, MigrationStatePending, migration.State)

	// Wait for migration to complete
	time.Sleep(500 * time.Millisecond)

	// Check final state
	result, err := executor.GetMigration(ctx, migration.ID)
	require.NoError(t, err)
	assert.Equal(t, MigrationStateCompleted, result.State)
	assert.Equal(t, 100, result.Progress)
}

func TestMigrationExecutor_StartMigration_SameRegion(t *testing.T) {
	executor, manager, cleanup := setupMigrationTest(t)
	defer cleanup()

	manager.placements["tenant-1"] = &TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-west-1",
		Mode:          PlacementSingle,
	}

	ctx := context.Background()
	_, err := executor.StartMigration(ctx, "tenant-1", "us-west-1", false)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTargetRegionSameAsCurrent)
}

func TestMigrationExecutor_StartMigration_MissingTenantID(t *testing.T) {
	executor, _, cleanup := setupMigrationTest(t)
	defer cleanup()

	ctx := context.Background()
	_, err := executor.StartMigration(ctx, "", "eu-central-1", false)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidMigration)
}

func TestMigrationExecutor_StartMigration_MissingRegion(t *testing.T) {
	executor, _, cleanup := setupMigrationTest(t)
	defer cleanup()

	ctx := context.Background()
	_, err := executor.StartMigration(ctx, "tenant-1", "", false)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidMigration)
}

func TestMigrationExecutor_StartMigration_AlreadyInProgress(t *testing.T) {
	executor, manager, cleanup := setupMigrationTest(t)
	defer cleanup()

	manager.placements["tenant-1"] = &TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-west-1",
		Mode:          PlacementSingle,
	}

	ctx := context.Background()

	// Start first migration
	_, err := executor.StartMigration(ctx, "tenant-1", "eu-central-1", false)
	require.NoError(t, err)

	// Try to start second migration for same tenant
	_, err = executor.StartMigration(ctx, "tenant-1", "ap-tokyo-1", false)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMigrationInProgress)
}

func TestMigrationExecutor_GetMigration(t *testing.T) {
	executor, manager, cleanup := setupMigrationTest(t)
	defer cleanup()

	manager.placements["tenant-1"] = &TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-west-1",
		Mode:          PlacementSingle,
	}

	ctx := context.Background()
	migration, err := executor.StartMigration(ctx, "tenant-1", "eu-central-1", true)
	require.NoError(t, err)

	// Retrieve migration
	result, err := executor.GetMigration(ctx, migration.ID)
	require.NoError(t, err)
	assert.Equal(t, migration.ID, result.ID)
	assert.Equal(t, migration.TenantID, result.TenantID)
}

func TestMigrationExecutor_GetMigration_NotFound(t *testing.T) {
	executor, _, cleanup := setupMigrationTest(t)
	defer cleanup()

	ctx := context.Background()
	_, err := executor.GetMigration(ctx, "nonexistent")

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMigrationNotFound)
}

func TestMigrationExecutor_CancelMigration(t *testing.T) {
	executor, manager, cleanup := setupMigrationTest(t)
	defer cleanup()

	manager.placements["tenant-1"] = &TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-west-1",
		Mode:          PlacementSingle,
	}

	ctx := context.Background()
	migration, err := executor.StartMigration(ctx, "tenant-1", "eu-central-1", false)
	require.NoError(t, err)

	// Cancel immediately
	err = executor.CancelMigration(ctx, migration.ID)
	require.NoError(t, err)

	// Check state
	result, err := executor.GetMigration(ctx, migration.ID)
	require.NoError(t, err)
	assert.Equal(t, MigrationStateCancelled, result.State)
	assert.NotNil(t, result.CompletedAt)
}

func TestMigrationExecutor_CancelMigration_NotFound(t *testing.T) {
	executor, _, cleanup := setupMigrationTest(t)
	defer cleanup()

	ctx := context.Background()
	err := executor.CancelMigration(ctx, "nonexistent")

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMigrationNotFound)
}

func TestMigrationExecutor_ListMigrations(t *testing.T) {
	executor, manager, cleanup := setupMigrationTest(t)
	defer cleanup()

	manager.placements["tenant-1"] = &TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-west-1",
		Mode:          PlacementSingle,
	}
	manager.placements["tenant-2"] = &TenantPlacement{
		TenantID:      "tenant-2",
		PrimaryRegion: "eu-central-1",
		Mode:          PlacementSingle,
	}

	ctx := context.Background()

	// Create migrations for tenant-1
	_, err := executor.StartMigration(ctx, "tenant-1", "eu-central-1", true)
	require.NoError(t, err)

	manager.placements["tenant-1"].PrimaryRegion = "eu-central-1"
	_, err = executor.StartMigration(ctx, "tenant-1", "ap-tokyo-1", true)
	require.NoError(t, err)

	// Create migration for tenant-2
	_, err = executor.StartMigration(ctx, "tenant-2", "us-west-1", true)
	require.NoError(t, err)

	// List tenant-1 migrations
	migrations, err := executor.ListMigrations(ctx, "tenant-1")
	require.NoError(t, err)
	assert.Len(t, migrations, 2)

	// List tenant-2 migrations
	migrations, err = executor.ListMigrations(ctx, "tenant-2")
	require.NoError(t, err)
	assert.Len(t, migrations, 1)
}

func TestMigrationExecutor_ListAllMigrations(t *testing.T) {
	executor, manager, cleanup := setupMigrationTest(t)
	defer cleanup()

	manager.placements["tenant-1"] = &TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-west-1",
		Mode:          PlacementSingle,
	}
	manager.placements["tenant-2"] = &TenantPlacement{
		TenantID:      "tenant-2",
		PrimaryRegion: "eu-central-1",
		Mode:          PlacementSingle,
	}

	ctx := context.Background()

	// Create migrations
	_, _ = executor.StartMigration(ctx, "tenant-1", "eu-central-1", true)
	_, _ = executor.StartMigration(ctx, "tenant-2", "us-west-1", true)

	// List all
	migrations, err := executor.ListAllMigrations(ctx)
	require.NoError(t, err)
	assert.Len(t, migrations, 2)
}

func TestMigrationExecutor_IsActiveMigration(t *testing.T) {
	executor, manager, cleanup := setupMigrationTest(t)
	defer cleanup()

	manager.placements["tenant-1"] = &TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-west-1",
		Mode:          PlacementSingle,
	}

	ctx := context.Background()

	// No active migration initially
	assert.False(t, executor.IsActiveMigration("tenant-1"))

	// Start migration (non-dry-run)
	_, err := executor.StartMigration(ctx, "tenant-1", "eu-central-1", false)
	require.NoError(t, err)

	// Should be active now
	assert.True(t, executor.IsActiveMigration("tenant-1"))

	// Wait for completion
	time.Sleep(500 * time.Millisecond)

	// Should no longer be active
	assert.False(t, executor.IsActiveMigration("tenant-1"))
}

func TestMigrationExecutor_PlacementUpdatedAfterMigration(t *testing.T) {
	executor, manager, cleanup := setupMigrationTest(t)
	defer cleanup()

	manager.placements["tenant-1"] = &TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-west-1",
		Mode:          PlacementSingle,
	}

	ctx := context.Background()

	// Start migration
	migration, err := executor.StartMigration(ctx, "tenant-1", "eu-central-1", false)
	require.NoError(t, err)

	// Wait for completion
	time.Sleep(500 * time.Millisecond)

	// Check migration completed
	result, err := executor.GetMigration(ctx, migration.ID)
	require.NoError(t, err)
	assert.Equal(t, MigrationStateCompleted, result.State)

	// Check placement was updated
	placement := manager.placements["tenant-1"]
	assert.Equal(t, "eu-central-1", placement.PrimaryRegion)
	assert.Contains(t, placement.Replicas, "us-west-1")
}
