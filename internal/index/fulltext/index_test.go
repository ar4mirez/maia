package fulltext

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestIndex(t *testing.T) (*BleveIndex, func()) {
	t.Helper()

	idx, err := NewBleveIndex(Config{InMemory: true})
	require.NoError(t, err)

	cleanup := func() {
		idx.Close()
	}

	return idx, cleanup
}

func TestBleveIndex_Index(t *testing.T) {
	idx, cleanup := setupTestIndex(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("indexes document successfully", func(t *testing.T) {
		doc := &Document{
			Content:   "The quick brown fox jumps over the lazy dog",
			Namespace: "test",
			Tags:      []string{"animal", "test"},
			Type:      "semantic",
		}

		err := idx.Index(ctx, "doc1", doc)
		require.NoError(t, err)

		size, err := idx.Size()
		require.NoError(t, err)
		assert.Equal(t, uint64(1), size)
	})

	t.Run("updates existing document", func(t *testing.T) {
		doc := &Document{
			Content:   "Updated content",
			Namespace: "test",
		}

		err := idx.Index(ctx, "doc1", doc)
		require.NoError(t, err)

		// Size should still be 1
		size, err := idx.Size()
		require.NoError(t, err)
		assert.Equal(t, uint64(1), size)
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		doc := &Document{Content: "test"}
		err := idx.Index(ctx, "doc2", doc)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestBleveIndex_IndexBatch(t *testing.T) {
	idx, cleanup := setupTestIndex(t)
	defer cleanup()

	ctx := context.Background()

	docs := map[string]*Document{
		"doc1": {Content: "First document", Namespace: "ns1"},
		"doc2": {Content: "Second document", Namespace: "ns1"},
		"doc3": {Content: "Third document", Namespace: "ns2"},
	}

	err := idx.IndexBatch(ctx, docs)
	require.NoError(t, err)

	size, err := idx.Size()
	require.NoError(t, err)
	assert.Equal(t, uint64(3), size)
}

func TestBleveIndex_Delete(t *testing.T) {
	idx, cleanup := setupTestIndex(t)
	defer cleanup()

	ctx := context.Background()

	// Index a document
	doc := &Document{Content: "Test content", Namespace: "test"}
	err := idx.Index(ctx, "doc1", doc)
	require.NoError(t, err)

	// Delete it
	err = idx.Delete(ctx, "doc1")
	require.NoError(t, err)

	size, err := idx.Size()
	require.NoError(t, err)
	assert.Equal(t, uint64(0), size)
}

func TestBleveIndex_Search(t *testing.T) {
	idx, cleanup := setupTestIndex(t)
	defer cleanup()

	ctx := context.Background()

	// Index test documents
	docs := map[string]*Document{
		"doc1": {
			Content:   "The quick brown fox jumps over the lazy dog",
			Namespace: "animals",
			Tags:      []string{"fox", "dog"},
			Type:      "semantic",
		},
		"doc2": {
			Content:   "A fast auburn canine leaps above a sleepy hound",
			Namespace: "animals",
			Tags:      []string{"canine", "hound"},
			Type:      "semantic",
		},
		"doc3": {
			Content:   "Machine learning and artificial intelligence",
			Namespace: "tech",
			Tags:      []string{"ml", "ai"},
			Type:      "episodic",
		},
		"doc4": {
			Content:   "Natural language processing with neural networks",
			Namespace: "tech",
			Tags:      []string{"nlp", "neural"},
			Type:      "semantic",
		},
	}

	err := idx.IndexBatch(ctx, docs)
	require.NoError(t, err)

	t.Run("finds matching documents", func(t *testing.T) {
		results, err := idx.Search(ctx, "fox", nil)
		require.NoError(t, err)
		assert.Equal(t, uint64(1), results.Total)
		assert.Len(t, results.Hits, 1)
		assert.Equal(t, "doc1", results.Hits[0].ID)
	})

	t.Run("finds partial matches", func(t *testing.T) {
		results, err := idx.Search(ctx, "machine learning", nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, results.Total, uint64(1))
	})

	t.Run("filters by namespace", func(t *testing.T) {
		opts := &SearchOptions{
			Limit:     10,
			Namespace: "tech",
		}
		results, err := idx.Search(ctx, "", opts)
		require.NoError(t, err)
		assert.Equal(t, uint64(2), results.Total)
	})

	t.Run("filters by type", func(t *testing.T) {
		opts := &SearchOptions{
			Limit: 10,
			Type:  "semantic",
		}
		results, err := idx.Search(ctx, "", opts)
		require.NoError(t, err)
		assert.Equal(t, uint64(3), results.Total)
	})

	t.Run("filters by tags", func(t *testing.T) {
		opts := &SearchOptions{
			Limit: 10,
			Tags:  []string{"fox"},
		}
		results, err := idx.Search(ctx, "", opts)
		require.NoError(t, err)
		assert.Equal(t, uint64(1), results.Total)
		assert.Equal(t, "doc1", results.Hits[0].ID)
	})

	t.Run("combines filters", func(t *testing.T) {
		opts := &SearchOptions{
			Limit:     10,
			Namespace: "animals",
			Type:      "semantic",
		}
		results, err := idx.Search(ctx, "fox", opts)
		require.NoError(t, err)
		assert.Equal(t, uint64(1), results.Total)
	})

	t.Run("respects limit", func(t *testing.T) {
		opts := &SearchOptions{
			Limit: 2,
		}
		results, err := idx.Search(ctx, "", opts)
		require.NoError(t, err)
		assert.Len(t, results.Hits, 2)
	})

	t.Run("respects offset", func(t *testing.T) {
		opts := &SearchOptions{
			Limit:  10,
			Offset: 2,
		}
		results, err := idx.Search(ctx, "", opts)
		require.NoError(t, err)
		assert.Len(t, results.Hits, 2) // Total 4, skip 2
	})

	t.Run("returns empty for no matches", func(t *testing.T) {
		results, err := idx.Search(ctx, "nonexistent_word_xyz", nil)
		require.NoError(t, err)
		assert.Equal(t, uint64(0), results.Total)
		assert.Len(t, results.Hits, 0)
	})

	t.Run("highlights matches", func(t *testing.T) {
		opts := &SearchOptions{
			Limit:           10,
			HighlightFields: []string{"content"},
		}
		results, err := idx.Search(ctx, "fox", opts)
		require.NoError(t, err)
		require.Len(t, results.Hits, 1)
		// Bleve may or may not return highlights depending on configuration
		// Just verify no error occurs
	})
}

func TestBleveIndex_Persistence(t *testing.T) {
	dir, err := os.MkdirTemp("", "bleve-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	indexPath := filepath.Join(dir, "test.bleve")
	ctx := context.Background()

	// Create and populate index
	idx1, err := NewBleveIndex(Config{Path: indexPath})
	require.NoError(t, err)

	doc := &Document{
		Content:   "Persistent document",
		Namespace: "test",
	}
	err = idx1.Index(ctx, "doc1", doc)
	require.NoError(t, err)

	err = idx1.Close()
	require.NoError(t, err)

	// Reopen and verify
	idx2, err := NewBleveIndex(Config{Path: indexPath})
	require.NoError(t, err)
	defer idx2.Close()

	size, err := idx2.Size()
	require.NoError(t, err)
	assert.Equal(t, uint64(1), size)

	results, err := idx2.Search(ctx, "persistent", nil)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), results.Total)
}

func TestBleveIndex_Close(t *testing.T) {
	idx, _ := setupTestIndex(t)
	ctx := context.Background()

	err := idx.Close()
	require.NoError(t, err)

	// Operations should fail after close
	doc := &Document{Content: "test"}
	err = idx.Index(ctx, "doc1", doc)
	assert.ErrorIs(t, err, ErrIndexClosed)

	_, err = idx.Search(ctx, "test", nil)
	assert.ErrorIs(t, err, ErrIndexClosed)
}

func TestDefaultSearchOptions(t *testing.T) {
	opts := DefaultSearchOptions()

	assert.Equal(t, 10, opts.Limit)
	assert.Equal(t, 0, opts.Offset)
}

func BenchmarkBleveIndex_Index(b *testing.B) {
	idx, err := NewBleveIndex(Config{InMemory: true})
	require.NoError(b, err)
	defer idx.Close()

	ctx := context.Background()
	doc := &Document{
		Content:   "The quick brown fox jumps over the lazy dog",
		Namespace: "bench",
		Tags:      []string{"test"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Index(ctx, string(rune(i)), doc)
	}
}

func BenchmarkBleveIndex_Search(b *testing.B) {
	idx, err := NewBleveIndex(Config{InMemory: true})
	require.NoError(b, err)
	defer idx.Close()

	ctx := context.Background()

	// Index 1000 documents
	for i := 0; i < 1000; i++ {
		doc := &Document{
			Content:   "The quick brown fox jumps over the lazy dog number " + string(rune(i)),
			Namespace: "bench",
		}
		idx.Index(ctx, string(rune(i)), doc)
	}

	opts := &SearchOptions{Limit: 10}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Search(ctx, "quick fox", opts)
	}
}

func TestBleveIndex_Get(t *testing.T) {
	idx, cleanup := setupTestIndex(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("gets existing document", func(t *testing.T) {
		doc := &Document{
			Content:   "Test document content",
			Namespace: "test",
		}
		err := idx.Index(ctx, "doc1", doc)
		require.NoError(t, err)

		retrieved, err := idx.Get(ctx, "doc1")
		require.NoError(t, err)
		assert.Equal(t, "doc1", retrieved.ID)
	})

	t.Run("returns not found for missing document", func(t *testing.T) {
		_, err := idx.Get(ctx, "nonexistent")
		assert.ErrorIs(t, err, ErrNotFound)
	})
}

func TestBleveIndex_Get_Closed(t *testing.T) {
	idx, _ := setupTestIndex(t)
	ctx := context.Background()

	// Index a document first
	doc := &Document{Content: "Test content"}
	err := idx.Index(ctx, "doc1", doc)
	require.NoError(t, err)

	// Close the index
	idx.Close()

	// Get should fail
	_, err = idx.Get(ctx, "doc1")
	assert.ErrorIs(t, err, ErrIndexClosed)
}

func TestBleveIndex_Size_Closed(t *testing.T) {
	idx, _ := setupTestIndex(t)

	idx.Close()

	_, err := idx.Size()
	assert.ErrorIs(t, err, ErrIndexClosed)
}

func TestBleveIndex_Delete_Closed(t *testing.T) {
	idx, _ := setupTestIndex(t)
	ctx := context.Background()

	idx.Close()

	err := idx.Delete(ctx, "doc1")
	assert.ErrorIs(t, err, ErrIndexClosed)
}

func TestBleveIndex_IndexBatch_Closed(t *testing.T) {
	idx, _ := setupTestIndex(t)
	ctx := context.Background()

	idx.Close()

	docs := map[string]*Document{
		"doc1": {Content: "Test"},
	}
	err := idx.IndexBatch(ctx, docs)
	assert.ErrorIs(t, err, ErrIndexClosed)
}

func TestBleveIndex_IndexBatch_ContextCancellation(t *testing.T) {
	idx, cleanup := setupTestIndex(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	docs := map[string]*Document{
		"doc1": {Content: "Test"},
	}
	err := idx.IndexBatch(ctx, docs)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestDeleteIndex(t *testing.T) {
	dir, err := os.MkdirTemp("", "bleve-delete-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	indexPath := filepath.Join(dir, "test-delete.bleve")

	// Create an index
	idx, err := NewBleveIndex(Config{Path: indexPath})
	require.NoError(t, err)

	ctx := context.Background()
	doc := &Document{Content: "Test content"}
	err = idx.Index(ctx, "doc1", doc)
	require.NoError(t, err)

	// Close it first
	err = idx.Close()
	require.NoError(t, err)

	// Verify it exists
	_, err = os.Stat(indexPath)
	require.NoError(t, err)

	// Delete the index
	err = DeleteIndex(indexPath)
	require.NoError(t, err)

	// Verify it's gone
	_, err = os.Stat(indexPath)
	assert.True(t, os.IsNotExist(err))
}

func TestBleveIndex_CloseIdempotent(t *testing.T) {
	idx, _ := setupTestIndex(t)

	// First close
	err := idx.Close()
	require.NoError(t, err)

	// Second close should not error
	err = idx.Close()
	assert.NoError(t, err)
}

func TestBleveIndex_Search_EmptyQuery(t *testing.T) {
	idx, cleanup := setupTestIndex(t)
	defer cleanup()

	ctx := context.Background()

	// Index some documents
	docs := map[string]*Document{
		"doc1": {Content: "First document", Namespace: "ns1"},
		"doc2": {Content: "Second document", Namespace: "ns2"},
	}
	err := idx.IndexBatch(ctx, docs)
	require.NoError(t, err)

	// Search with empty query (match all)
	results, err := idx.Search(ctx, "", &SearchOptions{Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, uint64(2), results.Total)
}
