package replication

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLastWriteWinsResolver(t *testing.T) {
	resolver := NewLastWriteWinsResolver()
	ctx := context.Background()

	now := time.Now()

	local := &WALEntry{
		ID:           "local-1",
		Timestamp:    now.Add(-time.Hour),
		Operation:    OperationUpdate,
		ResourceType: ResourceTypeMemory,
		ResourceID:   "mem-123",
		Data:         []byte(`{"content": "local version"}`),
	}

	remote := &WALEntry{
		ID:           "remote-1",
		Timestamp:    now,
		Operation:    OperationUpdate,
		ResourceType: ResourceTypeMemory,
		ResourceID:   "mem-123",
		Data:         []byte(`{"content": "remote version"}`),
	}

	// Remote is newer, should win
	winner, err := resolver.Resolve(ctx, local, remote)
	require.NoError(t, err)
	assert.Equal(t, remote.ID, winner.ID)

	// Swap timestamps - local should win
	local.Timestamp = now
	remote.Timestamp = now.Add(-time.Hour)

	winner, err = resolver.Resolve(ctx, local, remote)
	require.NoError(t, err)
	assert.Equal(t, local.ID, winner.ID)
}

func TestMergeResolver_MemoryMerge(t *testing.T) {
	resolver := NewMergeResolver()
	ctx := context.Background()

	now := time.Now()

	local := &WALEntry{
		ID:           "local-1",
		Timestamp:    now.Add(-time.Hour),
		Operation:    OperationUpdate,
		ResourceType: ResourceTypeMemory,
		ResourceID:   "mem-123",
		Data: []byte(`{
			"id": "mem-123",
			"content": "local content",
			"metadata": {"key1": "value1"},
			"tags": ["tag1", "tag2"],
			"access_count": 5
		}`),
	}

	remote := &WALEntry{
		ID:           "remote-1",
		Timestamp:    now,
		Operation:    OperationUpdate,
		ResourceType: ResourceTypeMemory,
		ResourceID:   "mem-123",
		Data: []byte(`{
			"id": "mem-123",
			"content": "remote content",
			"metadata": {"key2": "value2"},
			"tags": ["tag2", "tag3"],
			"access_count": 10
		}`),
	}

	winner, err := resolver.Resolve(ctx, local, remote)
	require.NoError(t, err)
	assert.NotNil(t, winner)

	// Winner should use later timestamp
	assert.Equal(t, now, winner.Timestamp)
}

func TestMergeResolver_FallbackForNonMemory(t *testing.T) {
	resolver := NewMergeResolver()
	ctx := context.Background()

	now := time.Now()

	local := &WALEntry{
		ID:           "local-1",
		Timestamp:    now.Add(-time.Hour),
		Operation:    OperationUpdate,
		ResourceType: ResourceTypeNamespace,
		ResourceID:   "ns-123",
		Data:         []byte(`{"name": "local"}`),
	}

	remote := &WALEntry{
		ID:           "remote-1",
		Timestamp:    now,
		Operation:    OperationUpdate,
		ResourceType: ResourceTypeNamespace,
		ResourceID:   "ns-123",
		Data:         []byte(`{"name": "remote"}`),
	}

	// Should fall back to last-write-wins
	winner, err := resolver.Resolve(ctx, local, remote)
	require.NoError(t, err)
	assert.Equal(t, remote.ID, winner.ID)
}

func TestMergeResolver_FallbackForCreate(t *testing.T) {
	resolver := NewMergeResolver()
	ctx := context.Background()

	now := time.Now()

	local := &WALEntry{
		ID:           "local-1",
		Timestamp:    now.Add(-time.Hour),
		Operation:    OperationCreate,
		ResourceType: ResourceTypeMemory,
		ResourceID:   "mem-123",
		Data:         []byte(`{"content": "local"}`),
	}

	remote := &WALEntry{
		ID:           "remote-1",
		Timestamp:    now,
		Operation:    OperationCreate,
		ResourceType: ResourceTypeMemory,
		ResourceID:   "mem-123",
		Data:         []byte(`{"content": "remote"}`),
	}

	// Should fall back to last-write-wins for creates
	winner, err := resolver.Resolve(ctx, local, remote)
	require.NoError(t, err)
	assert.Equal(t, remote.ID, winner.ID)
}

func TestRejectResolver(t *testing.T) {
	resolver := NewRejectResolver()
	ctx := context.Background()

	local := &WALEntry{
		ID:           "local-1",
		Operation:    OperationUpdate,
		ResourceType: ResourceTypeMemory,
		ResourceID:   "mem-123",
	}

	remote := &WALEntry{
		ID:           "remote-1",
		Operation:    OperationUpdate,
		ResourceType: ResourceTypeMemory,
		ResourceID:   "mem-123",
	}

	_, err := resolver.Resolve(ctx, local, remote)
	assert.ErrorIs(t, err, ErrConflict)
}

func TestNewConflictResolver(t *testing.T) {
	tests := []struct {
		strategy ConflictStrategy
		expected string
	}{
		{ConflictLastWriteWins, "*replication.LastWriteWinsResolver"},
		{ConflictMerge, "*replication.MergeResolver"},
		{ConflictReject, "*replication.RejectResolver"},
		{"unknown", "*replication.LastWriteWinsResolver"},
	}

	for _, tt := range tests {
		t.Run(string(tt.strategy), func(t *testing.T) {
			resolver := NewConflictResolver(tt.strategy)
			assert.NotNil(t, resolver)
		})
	}
}
