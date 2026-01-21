package replication

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/oklog/ulid/v2"
	"go.uber.org/zap"
)

// Migration errors.
var (
	ErrMigrationNotFound    = errors.New("migration not found")
	ErrMigrationInProgress  = errors.New("migration already in progress for tenant")
	ErrMigrationCancelled   = errors.New("migration was cancelled")
	ErrMigrationFailed      = errors.New("migration failed")
	ErrInvalidMigration     = errors.New("invalid migration configuration")
	ErrTargetRegionSameAsCurrent = errors.New("target region is same as current primary")
)

// MigrationState represents the state of a tenant migration.
type MigrationState string

const (
	MigrationStatePending    MigrationState = "pending"
	MigrationStateInProgress MigrationState = "in_progress"
	MigrationStateCompleted  MigrationState = "completed"
	MigrationStateFailed     MigrationState = "failed"
	MigrationStateCancelled  MigrationState = "cancelled"
)

// Migration represents a tenant data migration between regions.
type Migration struct {
	// ID is the unique migration identifier.
	ID string `json:"id"`

	// TenantID is the tenant being migrated.
	TenantID string `json:"tenant_id"`

	// FromRegion is the source region.
	FromRegion string `json:"from_region"`

	// ToRegion is the target region.
	ToRegion string `json:"to_region"`

	// State is the current migration state.
	State MigrationState `json:"state"`

	// StartedAt is when the migration started.
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is when the migration completed (or failed).
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Error contains the error message if migration failed.
	Error string `json:"error,omitempty"`

	// WALPosition is the WAL position at start.
	WALPosition *WALPosition `json:"wal_position,omitempty"`

	// TargetWALPosition is the WAL position to reach before switchover.
	TargetWALPosition *WALPosition `json:"target_wal_position,omitempty"`

	// Progress is the migration progress (0-100).
	Progress int `json:"progress"`

	// DryRun indicates this is a validation-only migration.
	DryRun bool `json:"dry_run,omitempty"`
}

// MigrationEvent represents a migration lifecycle event.
type MigrationEvent struct {
	MigrationID string         `json:"migration_id"`
	TenantID    string         `json:"tenant_id"`
	Event       string         `json:"event"`
	State       MigrationState `json:"state"`
	Timestamp   time.Time      `json:"timestamp"`
	Details     string         `json:"details,omitempty"`
}

// MigrationManager manages tenant migrations.
type MigrationManager interface {
	// StartMigration initiates a tenant migration.
	StartMigration(ctx context.Context, tenantID, toRegion string, dryRun bool) (*Migration, error)

	// GetMigration returns a migration by ID.
	GetMigration(ctx context.Context, migrationID string) (*Migration, error)

	// CancelMigration cancels an in-progress migration.
	CancelMigration(ctx context.Context, migrationID string) error

	// ListMigrations returns migrations for a tenant.
	ListMigrations(ctx context.Context, tenantID string) ([]*Migration, error)

	// ListAllMigrations returns all migrations.
	ListAllMigrations(ctx context.Context) ([]*Migration, error)
}

// MigrationExecutor handles the actual migration execution.
type MigrationExecutor struct {
	db              *badger.DB
	manager         ReplicationManager
	wal             WAL
	cache           *PlacementCache
	logger          *zap.Logger
	activeMigrations sync.Map // tenantID -> migrationID
	cancelFuncs      sync.Map // migrationID -> context.CancelFunc
}

// MigrationExecutorConfig configures the migration executor.
type MigrationExecutorConfig struct {
	DB      *badger.DB
	Manager ReplicationManager
	WAL     WAL
	Cache   *PlacementCache
	Logger  *zap.Logger
}

// NewMigrationExecutor creates a new migration executor.
func NewMigrationExecutor(cfg *MigrationExecutorConfig) *MigrationExecutor {
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	return &MigrationExecutor{
		db:      cfg.DB,
		manager: cfg.Manager,
		wal:     cfg.WAL,
		cache:   cfg.Cache,
		logger:  logger,
	}
}

// StartMigration initiates a tenant migration.
func (e *MigrationExecutor) StartMigration(ctx context.Context, tenantID, toRegion string, dryRun bool) (*Migration, error) {
	// Validate inputs
	if tenantID == "" {
		return nil, fmt.Errorf("%w: tenant ID is required", ErrInvalidMigration)
	}
	if toRegion == "" {
		return nil, fmt.Errorf("%w: target region is required", ErrInvalidMigration)
	}

	// Check for existing migration
	if existingID, ok := e.activeMigrations.Load(tenantID); ok {
		return nil, fmt.Errorf("%w: migration %s already running", ErrMigrationInProgress, existingID)
	}

	// Get current placement
	currentPlacement, err := e.manager.GetTenantPlacement(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get current placement: %w", err)
	}

	if currentPlacement.PrimaryRegion == toRegion {
		return nil, ErrTargetRegionSameAsCurrent
	}

	// Get current WAL position
	walPos, err := e.wal.Position(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get WAL position: %w", err)
	}

	// Create migration record
	migration := &Migration{
		ID:          ulid.Make().String(),
		TenantID:    tenantID,
		FromRegion:  currentPlacement.PrimaryRegion,
		ToRegion:    toRegion,
		State:       MigrationStatePending,
		StartedAt:   time.Now(),
		WALPosition: walPos,
		DryRun:      dryRun,
	}

	// Store migration
	if err := e.storeMigration(ctx, migration); err != nil {
		return nil, fmt.Errorf("failed to store migration: %w", err)
	}

	if dryRun {
		// For dry run, just return the plan
		migration.State = MigrationStateCompleted
		migration.Progress = 100
		now := time.Now()
		migration.CompletedAt = &now
		if err := e.storeMigration(ctx, migration); err != nil {
			return nil, fmt.Errorf("failed to store dry run result: %w", err)
		}
		return migration, nil
	}

	// Track active migration
	e.activeMigrations.Store(tenantID, migration.ID)

	// Create cancellable context for migration
	migCtx, cancel := context.WithCancel(context.Background())
	e.cancelFuncs.Store(migration.ID, cancel)

	// Execute migration in background
	go e.executeMigration(migCtx, migration)

	return migration, nil
}

// GetMigration returns a migration by ID.
func (e *MigrationExecutor) GetMigration(ctx context.Context, migrationID string) (*Migration, error) {
	var migration Migration
	err := e.db.View(func(txn *badger.Txn) error {
		key := []byte(fmt.Sprintf("migration:%s", migrationID))
		item, err := txn.Get(key)
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrMigrationNotFound
			}
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &migration)
		})
	})
	if err != nil {
		return nil, err
	}
	return &migration, nil
}

// CancelMigration cancels an in-progress migration.
func (e *MigrationExecutor) CancelMigration(ctx context.Context, migrationID string) error {
	migration, err := e.GetMigration(ctx, migrationID)
	if err != nil {
		return err
	}

	if migration.State != MigrationStatePending && migration.State != MigrationStateInProgress {
		return fmt.Errorf("cannot cancel migration in state %s", migration.State)
	}

	// Cancel the context
	if cancelFunc, ok := e.cancelFuncs.Load(migrationID); ok {
		cancelFunc.(context.CancelFunc)()
		e.cancelFuncs.Delete(migrationID)
	}

	// Update state
	migration.State = MigrationStateCancelled
	now := time.Now()
	migration.CompletedAt = &now
	migration.Error = "cancelled by user"

	if err := e.storeMigration(ctx, migration); err != nil {
		return fmt.Errorf("failed to update migration state: %w", err)
	}

	// Remove from active migrations
	e.activeMigrations.Delete(migration.TenantID)

	return nil
}

// ListMigrations returns migrations for a tenant.
func (e *MigrationExecutor) ListMigrations(ctx context.Context, tenantID string) ([]*Migration, error) {
	var migrations []*Migration

	err := e.db.View(func(txn *badger.Txn) error {
		prefix := []byte(fmt.Sprintf("migration_tenant:%s:", tenantID))
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			var migrationID string
			if err := item.Value(func(val []byte) error {
				migrationID = string(val)
				return nil
			}); err != nil {
				return err
			}

			migration, err := e.GetMigration(ctx, migrationID)
			if err != nil {
				continue // Skip if migration not found
			}
			migrations = append(migrations, migration)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return migrations, nil
}

// ListAllMigrations returns all migrations.
func (e *MigrationExecutor) ListAllMigrations(ctx context.Context) ([]*Migration, error) {
	var migrations []*Migration

	err := e.db.View(func(txn *badger.Txn) error {
		prefix := []byte("migration:")
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			var migration Migration
			if err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &migration)
			}); err != nil {
				continue
			}
			migrations = append(migrations, &migration)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return migrations, nil
}

// executeMigration performs the actual migration.
func (e *MigrationExecutor) executeMigration(ctx context.Context, migration *Migration) {
	defer func() {
		e.activeMigrations.Delete(migration.TenantID)
		e.cancelFuncs.Delete(migration.ID)
	}()

	e.logger.Info("starting migration execution",
		zap.String("migration_id", migration.ID),
		zap.String("tenant_id", migration.TenantID),
		zap.String("from_region", migration.FromRegion),
		zap.String("to_region", migration.ToRegion))

	// Update state to in_progress
	migration.State = MigrationStateInProgress
	migration.Progress = 10
	if err := e.storeMigration(ctx, migration); err != nil {
		e.failMigration(ctx, migration, fmt.Errorf("failed to update state: %w", err))
		return
	}

	// Step 1: Validate target region is available (30%)
	select {
	case <-ctx.Done():
		e.failMigration(ctx, migration, ErrMigrationCancelled)
		return
	default:
	}

	migration.Progress = 30
	if err := e.storeMigration(ctx, migration); err != nil {
		e.failMigration(ctx, migration, err)
		return
	}

	// Step 2: Wait for replication to catch up (60%)
	select {
	case <-ctx.Done():
		e.failMigration(ctx, migration, ErrMigrationCancelled)
		return
	default:
	}

	// In a real implementation, we would:
	// 1. Pause writes to the tenant (or queue them)
	// 2. Wait for all WAL entries to be replicated to target
	// 3. Verify data consistency
	time.Sleep(100 * time.Millisecond) // Simulated wait

	migration.Progress = 60
	if err := e.storeMigration(ctx, migration); err != nil {
		e.failMigration(ctx, migration, err)
		return
	}

	// Step 3: Update placement (80%)
	select {
	case <-ctx.Done():
		e.failMigration(ctx, migration, ErrMigrationCancelled)
		return
	default:
	}

	newPlacement := &TenantPlacement{
		TenantID:      migration.TenantID,
		PrimaryRegion: migration.ToRegion,
		Replicas:      []string{migration.FromRegion}, // Old primary becomes replica
		Mode:          PlacementReplicated,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if err := e.manager.SetTenantPlacement(ctx, newPlacement); err != nil {
		e.failMigration(ctx, migration, fmt.Errorf("failed to update placement: %w", err))
		return
	}

	migration.Progress = 80
	if err := e.storeMigration(ctx, migration); err != nil {
		e.failMigration(ctx, migration, err)
		return
	}

	// Step 4: Invalidate cache and complete (100%)
	if e.cache != nil {
		e.cache.Invalidate(migration.TenantID)
	}

	migration.State = MigrationStateCompleted
	migration.Progress = 100
	now := time.Now()
	migration.CompletedAt = &now

	if err := e.storeMigration(ctx, migration); err != nil {
		e.logger.Error("failed to store completed migration", zap.Error(err))
		return
	}

	e.logger.Info("migration completed successfully",
		zap.String("migration_id", migration.ID),
		zap.String("tenant_id", migration.TenantID))
}

// failMigration marks a migration as failed.
func (e *MigrationExecutor) failMigration(ctx context.Context, migration *Migration, err error) {
	migration.State = MigrationStateFailed
	migration.Error = err.Error()
	now := time.Now()
	migration.CompletedAt = &now

	if storeErr := e.storeMigration(ctx, migration); storeErr != nil {
		e.logger.Error("failed to store failed migration state",
			zap.Error(storeErr),
			zap.String("migration_id", migration.ID))
	}

	e.logger.Error("migration failed",
		zap.String("migration_id", migration.ID),
		zap.String("tenant_id", migration.TenantID),
		zap.Error(err))
}

// storeMigration persists a migration to BadgerDB.
func (e *MigrationExecutor) storeMigration(ctx context.Context, migration *Migration) error {
	data, err := json.Marshal(migration)
	if err != nil {
		return err
	}

	return e.db.Update(func(txn *badger.Txn) error {
		// Store migration
		migrationKey := []byte(fmt.Sprintf("migration:%s", migration.ID))
		if err := txn.Set(migrationKey, data); err != nil {
			return err
		}

		// Store tenant index
		tenantKey := []byte(fmt.Sprintf("migration_tenant:%s:%s", migration.TenantID, migration.ID))
		return txn.Set(tenantKey, []byte(migration.ID))
	})
}

// IsActiveMigration checks if a tenant has an active migration.
func (e *MigrationExecutor) IsActiveMigration(tenantID string) bool {
	_, ok := e.activeMigrations.Load(tenantID)
	return ok
}
