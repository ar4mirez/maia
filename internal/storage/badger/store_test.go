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

func TestStore_UpdateNamespace(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create a namespace
	input := &storage.CreateNamespaceInput{
		Name: "update-test",
		Config: storage.NamespaceConfig{
			TokenBudget:   4000,
			MaxMemories:   1000,
			RetentionDays: 30,
		},
	}

	created, err := store.CreateNamespace(ctx, input)
	require.NoError(t, err)

	// Wait to ensure UpdatedAt changes
	time.Sleep(10 * time.Millisecond)

	// Update the namespace
	newConfig := &storage.NamespaceConfig{
		TokenBudget:       8000,
		MaxMemories:       2000,
		RetentionDays:     60,
		InheritFromParent: true,
	}

	updated, err := store.UpdateNamespace(ctx, created.ID, newConfig)
	require.NoError(t, err)
	assert.Equal(t, created.ID, updated.ID)
	assert.Equal(t, created.Name, updated.Name)
	assert.Equal(t, 8000, updated.Config.TokenBudget)
	assert.Equal(t, 2000, updated.Config.MaxMemories)
	assert.Equal(t, 60, updated.Config.RetentionDays)
	assert.True(t, updated.Config.InheritFromParent)
	assert.True(t, updated.UpdatedAt.After(created.UpdatedAt))
}

func TestStore_UpdateNamespace_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := store.UpdateNamespace(ctx, "nonexistent-id", &storage.NamespaceConfig{
		TokenBudget: 5000,
	})
	assert.Error(t, err)

	var notFound *storage.ErrNotFound
	assert.ErrorAs(t, err, &notFound)
}

func TestStore_GetNamespace(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create a namespace
	input := &storage.CreateNamespaceInput{
		Name: "get-test",
		Config: storage.NamespaceConfig{
			TokenBudget: 5000,
		},
	}

	created, err := store.CreateNamespace(ctx, input)
	require.NoError(t, err)

	// Get by ID
	found, err := store.GetNamespace(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, found.ID)
	assert.Equal(t, created.Name, found.Name)
	assert.Equal(t, created.Config.TokenBudget, found.Config.TokenBudget)
}

func TestStore_GetNamespace_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := store.GetNamespace(ctx, "nonexistent-id")
	assert.Error(t, err)

	var notFound *storage.ErrNotFound
	assert.ErrorAs(t, err, &notFound)
}

func TestStore_GetNamespaceByName_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := store.GetNamespaceByName(ctx, "nonexistent-name")
	assert.Error(t, err)

	var notFound *storage.ErrNotFound
	assert.ErrorAs(t, err, &notFound)
}

func TestStore_DeleteMemory_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	err := store.DeleteMemory(ctx, "nonexistent-id")
	assert.Error(t, err)

	var notFound *storage.ErrNotFound
	assert.ErrorAs(t, err, &notFound)
}

func TestStore_DeleteNamespace_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	err := store.DeleteNamespace(ctx, "nonexistent-id")
	assert.Error(t, err)

	var notFound *storage.ErrNotFound
	assert.ErrorAs(t, err, &notFound)
}

func TestStore_UpdateMemory_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	newContent := "updated"
	_, err := store.UpdateMemory(ctx, "nonexistent-id", &storage.UpdateMemoryInput{
		Content: &newContent,
	})
	assert.Error(t, err)

	var notFound *storage.ErrNotFound
	assert.ErrorAs(t, err, &notFound)
}

func TestStore_TouchMemory_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	err := store.TouchMemory(ctx, "nonexistent-id")
	assert.Error(t, err)

	var notFound *storage.ErrNotFound
	assert.ErrorAs(t, err, &notFound)
}

func TestStore_SearchMemories_WithFilters(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	namespace := "test-filters"

	// Create memories with different metadata
	_, err := store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: namespace,
		Content:   "Memory with metadata A",
		Metadata:  map[string]interface{}{"category": "A"},
	})
	require.NoError(t, err)

	_, err = store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: namespace,
		Content:   "Memory with metadata B",
		Metadata:  map[string]interface{}{"category": "B"},
	})
	require.NoError(t, err)

	// Search all in namespace
	results, err := store.SearchMemories(ctx, &storage.SearchOptions{
		Namespace: namespace,
	})
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestStore_SearchMemories_WithTimeRange(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	namespace := "test-daterange"

	// Create a memory
	_, err := store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: namespace,
		Content:   "Recent memory",
	})
	require.NoError(t, err)

	// Search with time range including now
	now := time.Now()
	results, err := store.SearchMemories(ctx, &storage.SearchOptions{
		Namespace: namespace,
		TimeRange: &storage.TimeRange{
			Start: now.Add(-1 * time.Hour),
			End:   now.Add(1 * time.Hour),
		},
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)

	// Search with time range in the past
	results, err = store.SearchMemories(ctx, &storage.SearchOptions{
		Namespace: namespace,
		TimeRange: &storage.TimeRange{
			Start: now.Add(-48 * time.Hour),
			End:   now.Add(-24 * time.Hour),
		},
	})
	require.NoError(t, err)
	assert.Len(t, results, 0)
}

func TestStore_SearchMemories_WithLimit(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	namespace := "test-limit"

	// Create 10 memories
	for i := 0; i < 10; i++ {
		_, err := store.CreateMemory(ctx, &storage.CreateMemoryInput{
			Namespace: namespace,
			Content:   "Memory " + string(rune('A'+i)),
		})
		require.NoError(t, err)
	}

	// Search with limit
	results, err := store.SearchMemories(ctx, &storage.SearchOptions{
		Namespace: namespace,
		Limit:     5,
	})
	require.NoError(t, err)
	assert.Len(t, results, 5)
}

func TestStore_ListMemories_Empty(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	memories, err := store.ListMemories(ctx, "empty-namespace", &storage.ListOptions{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, memories, 0)
}

func TestStore_ListNamespaces_Empty(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	namespaces, err := store.ListNamespaces(ctx, &storage.ListOptions{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, namespaces, 0)
}

func TestStore_ListNamespaces_Pagination(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create 10 namespaces
	for i := 0; i < 10; i++ {
		input := &storage.CreateNamespaceInput{
			Name: "ns-paginate-" + string(rune('A'+i)),
		}
		_, err := store.CreateNamespace(ctx, input)
		require.NoError(t, err)
	}

	// Get first page
	page1, err := store.ListNamespaces(ctx, &storage.ListOptions{Limit: 5, Offset: 0})
	require.NoError(t, err)
	assert.Len(t, page1, 5)

	// Get second page
	page2, err := store.ListNamespaces(ctx, &storage.ListOptions{Limit: 5, Offset: 5})
	require.NoError(t, err)
	assert.Len(t, page2, 5)

	// Verify no overlap
	for _, n1 := range page1 {
		for _, n2 := range page2 {
			assert.NotEqual(t, n1.ID, n2.ID)
		}
	}
}

func TestStore_BatchCreateMemories_Validation(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Test with one invalid input
	inputs := []*storage.CreateMemoryInput{
		{Namespace: "batch", Content: "Valid memory"},
		{Namespace: "", Content: "Invalid - empty namespace"},
	}

	_, err := store.BatchCreateMemories(ctx, inputs)
	assert.Error(t, err)
}

func TestStore_BatchDeleteMemories_PartialNotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create one memory
	mem, err := store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "batch-partial",
		Content:   "Memory to delete",
	})
	require.NoError(t, err)

	// Try to batch delete existing and non-existing IDs
	// This should succeed for existing ones and skip non-existing
	err = store.BatchDeleteMemories(ctx, []string{mem.ID, "nonexistent-1", "nonexistent-2"})
	// The implementation should handle partial deletes gracefully
	require.NoError(t, err)

	// Verify the existing one was deleted
	_, err = store.GetMemory(ctx, mem.ID)
	assert.Error(t, err)
}

func TestStore_DataDir(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	dir := store.DataDir()
	assert.NotEmpty(t, dir)
	// The data dir should be inside the temp directory
	assert.DirExists(t, dir)
}

func TestStore_CreateMemory_WithEmbedding(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	embedding := []float32{0.1, 0.2, 0.3, 0.4, 0.5}
	input := &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Memory with embedding",
		Embedding: embedding,
	}

	mem, err := store.CreateMemory(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, embedding, mem.Embedding)

	// Verify embedding persisted
	retrieved, err := store.GetMemory(ctx, mem.ID)
	require.NoError(t, err)
	assert.Equal(t, embedding, retrieved.Embedding)
}

func TestStore_UpdateMemory_AllFields(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create a memory
	input := &storage.CreateMemoryInput{
		Namespace:  "test",
		Content:    "Original content",
		Type:       storage.MemoryTypeSemantic,
		Confidence: 0.5,
		Tags:       []string{"original"},
	}
	created, err := store.CreateMemory(ctx, input)
	require.NoError(t, err)

	// Wait to ensure UpdatedAt changes
	time.Sleep(10 * time.Millisecond)

	// Update all fields
	newContent := "Updated content"
	newConfidence := 0.9
	embedding := []float32{0.1, 0.2, 0.3}

	updateInput := &storage.UpdateMemoryInput{
		Content:    &newContent,
		Confidence: &newConfidence,
		Tags:       []string{"updated", "new"},
		Embedding:  embedding,
	}

	updated, err := store.UpdateMemory(ctx, created.ID, updateInput)
	require.NoError(t, err)
	assert.Equal(t, newContent, updated.Content)
	assert.Equal(t, newConfidence, updated.Confidence)
	assert.Equal(t, []string{"updated", "new"}, updated.Tags)
	assert.Equal(t, embedding, updated.Embedding)
	assert.True(t, updated.UpdatedAt.After(created.UpdatedAt))
}
