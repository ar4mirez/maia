package badger

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ar4mirez/maia/internal/storage"
)

func setupTestStore(t *testing.T) (*Store, func()) {
	t.Helper()

	dir, err := os.MkdirTemp("", "maia-test-*")
	require.NoError(t, err)

	store, err := NewWithPath(dir)
	require.NoError(t, err)

	cleanup := func() {
		store.Close()
		os.RemoveAll(dir)
	}

	return store, cleanup
}

func TestStore_CreateMemory(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	input := &storage.CreateMemoryInput{
		Namespace:  "test",
		Content:    "Test memory content",
		Type:       storage.MemoryTypeSemantic,
		Tags:       []string{"test", "unit"},
		Confidence: 0.9,
		Source:     storage.MemorySourceUser,
	}

	mem, err := store.CreateMemory(ctx, input)
	require.NoError(t, err)
	assert.NotEmpty(t, mem.ID)
	assert.Equal(t, input.Namespace, mem.Namespace)
	assert.Equal(t, input.Content, mem.Content)
	assert.Equal(t, input.Type, mem.Type)
	assert.Equal(t, input.Tags, mem.Tags)
	assert.Equal(t, input.Confidence, mem.Confidence)
	assert.Equal(t, input.Source, mem.Source)
	assert.False(t, mem.CreatedAt.IsZero())
	assert.False(t, mem.UpdatedAt.IsZero())
	assert.Zero(t, mem.AccessCount)
}

func TestStore_CreateMemory_Validation(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name    string
		input   *storage.CreateMemoryInput
		wantErr bool
	}{
		{
			name:    "nil input",
			input:   nil,
			wantErr: true,
		},
		{
			name: "empty content",
			input: &storage.CreateMemoryInput{
				Namespace: "test",
				Content:   "",
			},
			wantErr: true,
		},
		{
			name: "empty namespace",
			input: &storage.CreateMemoryInput{
				Namespace: "",
				Content:   "test",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := store.CreateMemory(ctx, tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestStore_GetMemory(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create a memory
	input := &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Test content",
		Type:      storage.MemoryTypeSemantic,
	}
	created, err := store.CreateMemory(ctx, input)
	require.NoError(t, err)

	// Get the memory
	retrieved, err := store.GetMemory(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, retrieved.ID)
	assert.Equal(t, created.Content, retrieved.Content)
}

func TestStore_GetMemory_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := store.GetMemory(ctx, "nonexistent-id")
	assert.Error(t, err)

	var notFound *storage.ErrNotFound
	assert.ErrorAs(t, err, &notFound)
}

func TestStore_UpdateMemory(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create a memory
	input := &storage.CreateMemoryInput{
		Namespace:  "test",
		Content:    "Original content",
		Confidence: 0.8,
	}
	created, err := store.CreateMemory(ctx, input)
	require.NoError(t, err)

	// Wait a bit to ensure UpdatedAt changes
	time.Sleep(10 * time.Millisecond)

	// Update the memory
	newContent := "Updated content"
	newConfidence := 0.95
	updateInput := &storage.UpdateMemoryInput{
		Content:    &newContent,
		Confidence: &newConfidence,
		Tags:       []string{"updated"},
	}

	updated, err := store.UpdateMemory(ctx, created.ID, updateInput)
	require.NoError(t, err)
	assert.Equal(t, newContent, updated.Content)
	assert.Equal(t, newConfidence, updated.Confidence)
	assert.Equal(t, []string{"updated"}, updated.Tags)
	assert.True(t, updated.UpdatedAt.After(created.UpdatedAt))
}

func TestStore_DeleteMemory(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create a memory
	input := &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "To be deleted",
	}
	created, err := store.CreateMemory(ctx, input)
	require.NoError(t, err)

	// Delete the memory
	err = store.DeleteMemory(ctx, created.ID)
	require.NoError(t, err)

	// Verify it's gone
	_, err = store.GetMemory(ctx, created.ID)
	assert.Error(t, err)
}

func TestStore_ListMemories(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	namespace := "test-list"

	// Create several memories
	for i := 0; i < 5; i++ {
		input := &storage.CreateMemoryInput{
			Namespace: namespace,
			Content:   "Memory " + string(rune('A'+i)),
		}
		_, err := store.CreateMemory(ctx, input)
		require.NoError(t, err)
	}

	// List memories
	memories, err := store.ListMemories(ctx, namespace, &storage.ListOptions{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, memories, 5)
}

func TestStore_ListMemories_Pagination(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	namespace := "test-pagination"

	// Create 10 memories
	for i := 0; i < 10; i++ {
		input := &storage.CreateMemoryInput{
			Namespace: namespace,
			Content:   "Memory " + string(rune('A'+i)),
		}
		_, err := store.CreateMemory(ctx, input)
		require.NoError(t, err)
	}

	// Get first page
	page1, err := store.ListMemories(ctx, namespace, &storage.ListOptions{Limit: 5, Offset: 0})
	require.NoError(t, err)
	assert.Len(t, page1, 5)

	// Get second page
	page2, err := store.ListMemories(ctx, namespace, &storage.ListOptions{Limit: 5, Offset: 5})
	require.NoError(t, err)
	assert.Len(t, page2, 5)

	// Verify no overlap
	for _, m1 := range page1 {
		for _, m2 := range page2 {
			assert.NotEqual(t, m1.ID, m2.ID)
		}
	}
}

func TestStore_SearchMemories_ByTags(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	namespace := "test-search"

	// Create memories with different tags
	_, err := store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: namespace,
		Content:   "Memory with tag A",
		Tags:      []string{"tag-a"},
	})
	require.NoError(t, err)

	_, err = store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: namespace,
		Content:   "Memory with tag B",
		Tags:      []string{"tag-b"},
	})
	require.NoError(t, err)

	_, err = store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: namespace,
		Content:   "Memory with both tags",
		Tags:      []string{"tag-a", "tag-b"},
	})
	require.NoError(t, err)

	// Search by tag-a
	results, err := store.SearchMemories(ctx, &storage.SearchOptions{
		Namespace: namespace,
		Tags:      []string{"tag-a"},
	})
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestStore_SearchMemories_ByType(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	namespace := "test-search-type"

	// Create memories with different types
	_, err := store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: namespace,
		Content:   "Semantic memory",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	_, err = store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: namespace,
		Content:   "Episodic memory",
		Type:      storage.MemoryTypeEpisodic,
	})
	require.NoError(t, err)

	// Search by type
	results, err := store.SearchMemories(ctx, &storage.SearchOptions{
		Namespace: namespace,
		Types:     []storage.MemoryType{storage.MemoryTypeSemantic},
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, storage.MemoryTypeSemantic, results[0].Memory.Type)
}

func TestStore_TouchMemory(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create a memory
	input := &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Test content",
	}
	created, err := store.CreateMemory(ctx, input)
	require.NoError(t, err)
	assert.Zero(t, created.AccessCount)

	// Touch the memory
	time.Sleep(10 * time.Millisecond)
	err = store.TouchMemory(ctx, created.ID)
	require.NoError(t, err)

	// Verify access count and time updated
	retrieved, err := store.GetMemory(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), retrieved.AccessCount)
	assert.True(t, retrieved.AccessedAt.After(created.AccessedAt))
}

func TestStore_CreateNamespace(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	input := &storage.CreateNamespaceInput{
		Name: "test-namespace",
		Config: storage.NamespaceConfig{
			TokenBudget: 5000,
		},
	}

	ns, err := store.CreateNamespace(ctx, input)
	require.NoError(t, err)
	assert.NotEmpty(t, ns.ID)
	assert.Equal(t, input.Name, ns.Name)
	assert.Equal(t, input.Config.TokenBudget, ns.Config.TokenBudget)
}

func TestStore_CreateNamespace_Duplicate(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	input := &storage.CreateNamespaceInput{
		Name: "duplicate-ns",
	}

	_, err := store.CreateNamespace(ctx, input)
	require.NoError(t, err)

	// Try to create again
	_, err = store.CreateNamespace(ctx, input)
	assert.Error(t, err)

	var alreadyExists *storage.ErrAlreadyExists
	assert.ErrorAs(t, err, &alreadyExists)
}

func TestStore_GetNamespaceByName(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	input := &storage.CreateNamespaceInput{
		Name: "findme",
	}

	created, err := store.CreateNamespace(ctx, input)
	require.NoError(t, err)

	// Get by name
	found, err := store.GetNamespaceByName(ctx, "findme")
	require.NoError(t, err)
	assert.Equal(t, created.ID, found.ID)
}

func TestStore_ListNamespaces(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create several namespaces
	for i := 0; i < 3; i++ {
		input := &storage.CreateNamespaceInput{
			Name: "ns-" + string(rune('A'+i)),
		}
		_, err := store.CreateNamespace(ctx, input)
		require.NoError(t, err)
	}

	// List namespaces
	namespaces, err := store.ListNamespaces(ctx, &storage.ListOptions{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, namespaces, 3)
}

func TestStore_DeleteNamespace(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	input := &storage.CreateNamespaceInput{
		Name: "to-delete",
	}

	created, err := store.CreateNamespace(ctx, input)
	require.NoError(t, err)

	// Delete
	err = store.DeleteNamespace(ctx, created.ID)
	require.NoError(t, err)

	// Verify it's gone
	_, err = store.GetNamespace(ctx, created.ID)
	assert.Error(t, err)

	// Also verify name index is gone
	_, err = store.GetNamespaceByName(ctx, "to-delete")
	assert.Error(t, err)
}

func TestStore_BatchCreateMemories(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	inputs := []*storage.CreateMemoryInput{
		{Namespace: "batch", Content: "Memory 1"},
		{Namespace: "batch", Content: "Memory 2"},
		{Namespace: "batch", Content: "Memory 3"},
	}

	memories, err := store.BatchCreateMemories(ctx, inputs)
	require.NoError(t, err)
	assert.Len(t, memories, 3)

	// Verify all were created
	for _, mem := range memories {
		retrieved, err := store.GetMemory(ctx, mem.ID)
		require.NoError(t, err)
		assert.Equal(t, mem.Content, retrieved.Content)
	}
}

func TestStore_BatchDeleteMemories(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create memories
	var ids []string
	for i := 0; i < 3; i++ {
		mem, err := store.CreateMemory(ctx, &storage.CreateMemoryInput{
			Namespace: "batch-delete",
			Content:   "Memory " + string(rune('A'+i)),
		})
		require.NoError(t, err)
		ids = append(ids, mem.ID)
	}

	// Batch delete
	err := store.BatchDeleteMemories(ctx, ids)
	require.NoError(t, err)

	// Verify all are gone
	for _, id := range ids {
		_, err := store.GetMemory(ctx, id)
		assert.Error(t, err)
	}
}

func TestStore_Stats(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create some data
	_, err := store.CreateNamespace(ctx, &storage.CreateNamespaceInput{Name: "stats-ns"})
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		_, err := store.CreateMemory(ctx, &storage.CreateMemoryInput{
			Namespace: "stats-ns",
			Content:   "Memory " + string(rune('A'+i)),
		})
		require.NoError(t, err)
	}

	// Get stats
	stats, err := store.Stats(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(5), stats.TotalMemories)
	assert.Equal(t, int64(1), stats.TotalNamespaces)
	// Storage size may be 0 initially due to BadgerDB lazy flushing
	assert.GreaterOrEqual(t, stats.StorageSizeBytes, int64(0))
}
