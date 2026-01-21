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

func setupTestWAL(t *testing.T) (*BadgerWAL, func()) {
	t.Helper()

	dir, err := os.MkdirTemp("", "wal-test-*")
	require.NoError(t, err)

	opts := &BadgerWALOptions{
		DataDir:    dir,
		Region:     "test-region",
		Logger:     zap.NewNop(),
		SyncWrites: false,
	}

	wal, err := NewBadgerWAL(opts)
	require.NoError(t, err)

	cleanup := func() {
		wal.Close()
		os.RemoveAll(dir)
	}

	return wal, cleanup
}

func TestBadgerWAL_Append(t *testing.T) {
	wal, cleanup := setupTestWAL(t)
	defer cleanup()

	ctx := context.Background()

	entry := &WALEntry{
		TenantID:     "tenant-1",
		Operation:    OperationCreate,
		ResourceType: ResourceTypeMemory,
		ResourceID:   "mem-123",
		Namespace:    "test-ns",
		Data:         []byte(`{"content": "test memory"}`),
	}

	err := wal.Append(ctx, entry)
	require.NoError(t, err)

	// Verify entry was assigned ID and sequence
	assert.NotEmpty(t, entry.ID)
	assert.Equal(t, uint64(1), entry.Sequence)
	assert.NotZero(t, entry.Checksum)
	assert.Equal(t, "test-region", entry.Region)
}

func TestBadgerWAL_Read(t *testing.T) {
	wal, cleanup := setupTestWAL(t)
	defer cleanup()

	ctx := context.Background()

	// Append multiple entries
	for i := 0; i < 5; i++ {
		entry := &WALEntry{
			TenantID:     "tenant-1",
			Operation:    OperationCreate,
			ResourceType: ResourceTypeMemory,
			ResourceID:   "mem-" + string(rune('a'+i)),
			Data:         []byte(`{"content": "test"}`),
		}
		err := wal.Append(ctx, entry)
		require.NoError(t, err)
	}

	// Read all entries
	entries, err := wal.Read(ctx, 0, 10)
	require.NoError(t, err)
	assert.Len(t, entries, 5)

	// Verify ordering
	for i, e := range entries {
		assert.Equal(t, uint64(i+1), e.Sequence)
	}

	// Read with offset
	entries, err = wal.Read(ctx, 3, 10)
	require.NoError(t, err)
	assert.Len(t, entries, 2)
	assert.Equal(t, uint64(4), entries[0].Sequence)
	assert.Equal(t, uint64(5), entries[1].Sequence)
}

func TestBadgerWAL_GetEntry(t *testing.T) {
	wal, cleanup := setupTestWAL(t)
	defer cleanup()

	ctx := context.Background()

	entry := &WALEntry{
		TenantID:     "tenant-1",
		Operation:    OperationUpdate,
		ResourceType: ResourceTypeMemory,
		ResourceID:   "mem-123",
		Data:         []byte(`{"content": "updated"}`),
	}

	err := wal.Append(ctx, entry)
	require.NoError(t, err)

	// Get by ID
	retrieved, err := wal.GetEntry(ctx, entry.ID)
	require.NoError(t, err)
	assert.Equal(t, entry.ID, retrieved.ID)
	assert.Equal(t, entry.Operation, retrieved.Operation)
	assert.Equal(t, entry.ResourceID, retrieved.ResourceID)

	// Get non-existent
	_, err = wal.GetEntry(ctx, "non-existent")
	assert.Error(t, err)
}

func TestBadgerWAL_Position(t *testing.T) {
	wal, cleanup := setupTestWAL(t)
	defer cleanup()

	ctx := context.Background()

	// Initial position
	pos, err := wal.Position(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), pos.Sequence)

	// After appending entries
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

	pos, err = wal.Position(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(3), pos.Sequence)
	assert.NotEmpty(t, pos.EntryID)
}

func TestBadgerWAL_Truncate(t *testing.T) {
	wal, cleanup := setupTestWAL(t)
	defer cleanup()

	ctx := context.Background()

	// Append entries
	for i := 0; i < 10; i++ {
		entry := &WALEntry{
			TenantID:     "tenant-1",
			Operation:    OperationCreate,
			ResourceType: ResourceTypeMemory,
			ResourceID:   "mem-" + string(rune('a'+i)),
		}
		err := wal.Append(ctx, entry)
		require.NoError(t, err)
	}

	// Truncate entries before sequence 6
	err := wal.Truncate(ctx, 6)
	require.NoError(t, err)

	// Read remaining entries
	entries, err := wal.Read(ctx, 0, 20)
	require.NoError(t, err)
	assert.Len(t, entries, 5) // Entries 6-10 remain
	assert.Equal(t, uint64(6), entries[0].Sequence)
}

func TestBadgerWAL_Stats(t *testing.T) {
	wal, cleanup := setupTestWAL(t)
	defer cleanup()

	ctx := context.Background()

	// Append entries
	for i := 0; i < 5; i++ {
		entry := &WALEntry{
			TenantID:     "tenant-1",
			Operation:    OperationCreate,
			ResourceType: ResourceTypeMemory,
			ResourceID:   "mem-" + string(rune('a'+i)),
			Data:         []byte(`{"content": "test data"}`),
		}
		err := wal.Append(ctx, entry)
		require.NoError(t, err)
	}

	stats, err := wal.Stats(ctx)
	require.NoError(t, err)
	// Entry count includes all keys with wal: prefix
	assert.GreaterOrEqual(t, stats.EntryCount, int64(5))
	assert.Equal(t, uint64(5), stats.CurrentSeq)
	assert.True(t, stats.TotalBytes > 0)
}

func TestBadgerWAL_Sync(t *testing.T) {
	wal, cleanup := setupTestWAL(t)
	defer cleanup()

	ctx := context.Background()

	entry := &WALEntry{
		TenantID:     "tenant-1",
		Operation:    OperationCreate,
		ResourceType: ResourceTypeMemory,
		ResourceID:   "mem-123",
	}

	err := wal.Append(ctx, entry)
	require.NoError(t, err)

	err = wal.Sync(ctx)
	require.NoError(t, err)
}

func TestBadgerWAL_ClosedOperations(t *testing.T) {
	wal, cleanup := setupTestWAL(t)

	// Close the WAL
	err := wal.Close()
	require.NoError(t, err)
	defer cleanup()

	ctx := context.Background()

	// All operations should fail
	entry := &WALEntry{
		TenantID:     "tenant-1",
		Operation:    OperationCreate,
		ResourceType: ResourceTypeMemory,
		ResourceID:   "mem-123",
	}

	err = wal.Append(ctx, entry)
	assert.ErrorIs(t, err, ErrWALClosed)

	_, err = wal.Read(ctx, 0, 10)
	assert.ErrorIs(t, err, ErrWALClosed)

	_, err = wal.GetEntry(ctx, "test")
	assert.ErrorIs(t, err, ErrWALClosed)

	_, err = wal.Position(ctx)
	assert.ErrorIs(t, err, ErrWALClosed)
}

func TestWALEntry_Checksum(t *testing.T) {
	entry := &WALEntry{
		ID:           "test-id",
		Sequence:     1,
		Timestamp:    time.Now().UTC(),
		TenantID:     "tenant-1",
		Operation:    OperationCreate,
		ResourceType: ResourceTypeMemory,
		ResourceID:   "mem-123",
		Namespace:    "test-ns",
		Data:         []byte(`{"content": "test"}`),
	}

	// Compute checksum
	checksum := entry.ComputeChecksum()
	assert.NotZero(t, checksum)

	// Set and verify
	entry.Checksum = checksum
	assert.True(t, entry.VerifyChecksum())

	// Modify and verify fails
	entry.Data = []byte(`{"content": "modified"}`)
	assert.False(t, entry.VerifyChecksum())
}

func TestWALEntry_Validate(t *testing.T) {
	tests := []struct {
		name    string
		entry   *WALEntry
		wantErr bool
	}{
		{
			name: "valid entry",
			entry: &WALEntry{
				ID:           "test-id",
				Timestamp:    time.Now(),
				Operation:    OperationCreate,
				ResourceType: ResourceTypeMemory,
				ResourceID:   "mem-123",
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			entry: &WALEntry{
				Timestamp:    time.Now(),
				Operation:    OperationCreate,
				ResourceType: ResourceTypeMemory,
				ResourceID:   "mem-123",
			},
			wantErr: true,
		},
		{
			name: "missing timestamp",
			entry: &WALEntry{
				ID:           "test-id",
				Operation:    OperationCreate,
				ResourceType: ResourceTypeMemory,
				ResourceID:   "mem-123",
			},
			wantErr: true,
		},
		{
			name: "missing operation",
			entry: &WALEntry{
				ID:           "test-id",
				Timestamp:    time.Now(),
				ResourceType: ResourceTypeMemory,
				ResourceID:   "mem-123",
			},
			wantErr: true,
		},
		{
			name: "missing resource type",
			entry: &WALEntry{
				ID:         "test-id",
				Timestamp:  time.Now(),
				Operation:  OperationCreate,
				ResourceID: "mem-123",
			},
			wantErr: true,
		},
		{
			name: "missing resource ID",
			entry: &WALEntry{
				ID:           "test-id",
				Timestamp:    time.Now(),
				Operation:    OperationCreate,
				ResourceType: ResourceTypeMemory,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.entry.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
