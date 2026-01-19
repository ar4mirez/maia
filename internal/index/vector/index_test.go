package vector

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ar4mirez/maia/internal/embedding"
)

func TestBruteForceIndex_Add(t *testing.T) {
	idx := NewBruteForceIndex(3)
	defer idx.Close()

	ctx := context.Background()

	t.Run("adds vector successfully", func(t *testing.T) {
		err := idx.Add(ctx, "vec1", []float32{1, 0, 0})
		require.NoError(t, err)
		assert.Equal(t, 1, idx.Size())
	})

	t.Run("rejects mismatched dimension", func(t *testing.T) {
		err := idx.Add(ctx, "vec2", []float32{1, 0})
		assert.ErrorIs(t, err, ErrDimensionMismatch)
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := idx.Add(ctx, "vec3", []float32{1, 0, 0})
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestBruteForceIndex_Search(t *testing.T) {
	idx := NewBruteForceIndex(3)
	defer idx.Close()

	ctx := context.Background()

	// Add some vectors
	vectors := map[string][]float32{
		"north": {0, 1, 0},
		"south": {0, -1, 0},
		"east":  {1, 0, 0},
		"west":  {-1, 0, 0},
		"up":    {0, 0, 1},
	}

	for id, vec := range vectors {
		err := idx.Add(ctx, id, vec)
		require.NoError(t, err)
	}

	t.Run("finds nearest neighbor", func(t *testing.T) {
		// Search for something close to "north"
		query := []float32{0.1, 0.9, 0}
		results, err := idx.Search(ctx, query, 1)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "north", results[0].ID)
	})

	t.Run("returns k results sorted by similarity", func(t *testing.T) {
		query := []float32{0.5, 0.5, 0}
		results, err := idx.Search(ctx, query, 3)
		require.NoError(t, err)
		require.Len(t, results, 3)

		// Results should be sorted by score descending
		for i := 1; i < len(results); i++ {
			assert.GreaterOrEqual(t, results[i-1].Score, results[i].Score)
		}
	})

	t.Run("returns fewer than k if index is small", func(t *testing.T) {
		query := []float32{1, 0, 0}
		results, err := idx.Search(ctx, query, 100)
		require.NoError(t, err)
		assert.Len(t, results, 5)
	})

	t.Run("rejects invalid k", func(t *testing.T) {
		query := []float32{1, 0, 0}
		_, err := idx.Search(ctx, query, 0)
		assert.ErrorIs(t, err, ErrInvalidK)
	})
}

func TestBruteForceIndex_Remove(t *testing.T) {
	idx := NewBruteForceIndex(3)
	defer idx.Close()

	ctx := context.Background()

	err := idx.Add(ctx, "vec1", []float32{1, 0, 0})
	require.NoError(t, err)

	t.Run("removes existing vector", func(t *testing.T) {
		err := idx.Remove(ctx, "vec1")
		require.NoError(t, err)
		assert.Equal(t, 0, idx.Size())
	})

	t.Run("returns error for non-existent vector", func(t *testing.T) {
		err := idx.Remove(ctx, "nonexistent")
		assert.ErrorIs(t, err, ErrNotFound)
	})
}

func TestBruteForceIndex_Get(t *testing.T) {
	idx := NewBruteForceIndex(3)
	defer idx.Close()

	ctx := context.Background()
	original := []float32{1, 2, 3}
	err := idx.Add(ctx, "vec1", original)
	require.NoError(t, err)

	t.Run("retrieves stored vector", func(t *testing.T) {
		retrieved, err := idx.Get(ctx, "vec1")
		require.NoError(t, err)
		assert.Equal(t, original, retrieved)
	})

	t.Run("returns error for non-existent vector", func(t *testing.T) {
		_, err := idx.Get(ctx, "nonexistent")
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("returns a copy, not the original", func(t *testing.T) {
		retrieved, err := idx.Get(ctx, "vec1")
		require.NoError(t, err)
		retrieved[0] = 999

		retrieved2, err := idx.Get(ctx, "vec1")
		require.NoError(t, err)
		assert.Equal(t, float32(1), retrieved2[0])
	})
}

func TestBruteForceIndex_Close(t *testing.T) {
	idx := NewBruteForceIndex(3)
	ctx := context.Background()

	err := idx.Add(ctx, "vec1", []float32{1, 0, 0})
	require.NoError(t, err)

	err = idx.Close()
	require.NoError(t, err)

	_, err = idx.Search(ctx, []float32{1, 0, 0}, 1)
	assert.ErrorIs(t, err, ErrIndexClosed)
}

func TestHNSWIndex_BasicOperations(t *testing.T) {
	cfg := DefaultConfig(3)
	idx := NewHNSWIndex(cfg)
	defer idx.Close()

	ctx := context.Background()

	t.Run("adds vectors successfully", func(t *testing.T) {
		err := idx.Add(ctx, "vec1", []float32{1, 0, 0})
		require.NoError(t, err)

		err = idx.Add(ctx, "vec2", []float32{0, 1, 0})
		require.NoError(t, err)

		assert.Equal(t, 2, idx.Size())
	})

	t.Run("retrieves vectors", func(t *testing.T) {
		vec, err := idx.Get(ctx, "vec1")
		require.NoError(t, err)
		assert.Equal(t, []float32{1, 0, 0}, vec)
	})

	t.Run("removes vectors", func(t *testing.T) {
		err := idx.Remove(ctx, "vec1")
		require.NoError(t, err)
		assert.Equal(t, 1, idx.Size())
	})
}

func TestHNSWIndex_Search(t *testing.T) {
	cfg := DefaultConfig(384)
	cfg.EfSearch = 100
	idx := NewHNSWIndex(cfg)
	defer idx.Close()

	ctx := context.Background()
	provider := embedding.NewMockProvider(384)

	// Add several vectors
	texts := []string{
		"The quick brown fox jumps over the lazy dog",
		"A fast auburn canine leaps above a sleepy hound",
		"Hello world this is a test",
		"Machine learning and artificial intelligence",
		"Natural language processing techniques",
	}

	for i, text := range texts {
		emb, err := provider.Embed(ctx, text)
		require.NoError(t, err)
		err = idx.Add(ctx, texts[i], emb)
		require.NoError(t, err)
	}

	t.Run("finds similar texts", func(t *testing.T) {
		// Search for something similar to the first text
		query, err := provider.Embed(ctx, texts[0])
		require.NoError(t, err)

		results, err := idx.Search(ctx, query, 2)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(results), 1)

		// The exact same text should be the top result
		assert.Equal(t, texts[0], results[0].ID)
		assert.InDelta(t, 1.0, results[0].Score, 0.001)
	})

	t.Run("returns empty for empty index", func(t *testing.T) {
		emptyIdx := NewHNSWIndex(cfg)
		defer emptyIdx.Close()

		query, _ := provider.Embed(ctx, "test")
		results, err := emptyIdx.Search(ctx, query, 5)
		require.NoError(t, err)
		assert.Len(t, results, 0)
	})
}

func TestHNSWIndex_BatchAdd(t *testing.T) {
	cfg := DefaultConfig(3)
	idx := NewHNSWIndex(cfg)
	defer idx.Close()

	ctx := context.Background()

	ids := []string{"v1", "v2", "v3"}
	vectors := [][]float32{
		{1, 0, 0},
		{0, 1, 0},
		{0, 0, 1},
	}

	err := idx.AddBatch(ctx, ids, vectors)
	require.NoError(t, err)
	assert.Equal(t, 3, idx.Size())
}

func BenchmarkBruteForceIndex_Search(b *testing.B) {
	idx := NewBruteForceIndex(384)
	ctx := context.Background()
	provider := embedding.NewMockProvider(384)

	// Add 1000 vectors
	for i := 0; i < 1000; i++ {
		emb, _ := provider.Embed(ctx, string(rune(i)))
		idx.Add(ctx, string(rune(i)), emb)
	}

	query, _ := provider.Embed(ctx, "test query")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Search(ctx, query, 10)
	}
}

func BenchmarkHNSWIndex_Search(b *testing.B) {
	cfg := DefaultConfig(384)
	idx := NewHNSWIndex(cfg)
	ctx := context.Background()
	provider := embedding.NewMockProvider(384)

	// Add 1000 vectors
	for i := 0; i < 1000; i++ {
		emb, _ := provider.Embed(ctx, string(rune(i)))
		idx.Add(ctx, string(rune(i)), emb)
	}

	query, _ := provider.Embed(ctx, "test query")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Search(ctx, query, 10)
	}
}

func TestBruteForceIndex_Dimension(t *testing.T) {
	idx := NewBruteForceIndex(128)
	defer idx.Close()

	assert.Equal(t, 128, idx.Dimension())
}

func TestBruteForceIndex_AddBatch(t *testing.T) {
	idx := NewBruteForceIndex(3)
	defer idx.Close()

	ctx := context.Background()

	t.Run("adds batch of vectors", func(t *testing.T) {
		ids := []string{"v1", "v2", "v3"}
		vectors := [][]float32{
			{1, 0, 0},
			{0, 1, 0},
			{0, 0, 1},
		}

		err := idx.AddBatch(ctx, ids, vectors)
		require.NoError(t, err)
		assert.Equal(t, 3, idx.Size())
	})

	t.Run("rejects mismatched lengths", func(t *testing.T) {
		ids := []string{"v4", "v5"}
		vectors := [][]float32{{1, 0, 0}}

		err := idx.AddBatch(ctx, ids, vectors)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "same length")
	})
}

func TestHNSWIndex_Dimension(t *testing.T) {
	cfg := DefaultConfig(256)
	idx := NewHNSWIndex(cfg)
	defer idx.Close()

	assert.Equal(t, 256, idx.Dimension())
}

func TestHNSWIndex_AddBatch_Errors(t *testing.T) {
	cfg := DefaultConfig(3)
	idx := NewHNSWIndex(cfg)
	defer idx.Close()

	ctx := context.Background()

	t.Run("rejects mismatched lengths", func(t *testing.T) {
		ids := []string{"v1", "v2"}
		vectors := [][]float32{{1, 0, 0}}

		err := idx.AddBatch(ctx, ids, vectors)
		assert.ErrorIs(t, err, ErrDimensionMismatch)
	})
}

func TestHNSWIndex_ErrorCases(t *testing.T) {
	cfg := DefaultConfig(3)
	idx := NewHNSWIndex(cfg)

	ctx := context.Background()

	t.Run("add rejects dimension mismatch", func(t *testing.T) {
		err := idx.Add(ctx, "vec1", []float32{1, 0}) // Only 2 dimensions
		assert.ErrorIs(t, err, ErrDimensionMismatch)
	})

	t.Run("search rejects dimension mismatch", func(t *testing.T) {
		_, err := idx.Search(ctx, []float32{1, 0}, 5)
		assert.ErrorIs(t, err, ErrDimensionMismatch)
	})

	t.Run("search rejects invalid k", func(t *testing.T) {
		err := idx.Add(ctx, "vec1", []float32{1, 0, 0})
		require.NoError(t, err)

		_, err = idx.Search(ctx, []float32{1, 0, 0}, 0)
		assert.ErrorIs(t, err, ErrInvalidK)

		_, err = idx.Search(ctx, []float32{1, 0, 0}, -1)
		assert.ErrorIs(t, err, ErrInvalidK)
	})

	t.Run("get returns not found for missing id", func(t *testing.T) {
		_, err := idx.Get(ctx, "nonexistent")
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("remove returns not found for missing id", func(t *testing.T) {
		err := idx.Remove(ctx, "nonexistent")
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("operations fail after close", func(t *testing.T) {
		idx.Close()

		err := idx.Add(ctx, "vec2", []float32{1, 0, 0})
		assert.ErrorIs(t, err, ErrIndexClosed)

		_, err = idx.Search(ctx, []float32{1, 0, 0}, 1)
		assert.ErrorIs(t, err, ErrIndexClosed)

		_, err = idx.Get(ctx, "vec1")
		assert.ErrorIs(t, err, ErrIndexClosed)

		err = idx.Remove(ctx, "vec1")
		assert.ErrorIs(t, err, ErrIndexClosed)
	})
}

func TestHNSWIndex_ContextCancellation(t *testing.T) {
	cfg := DefaultConfig(3)
	idx := NewHNSWIndex(cfg)
	defer idx.Close()

	t.Run("add respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := idx.Add(ctx, "vec1", []float32{1, 0, 0})
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("search respects context cancellation", func(t *testing.T) {
		// First add a vector with valid context
		ctx := context.Background()
		err := idx.Add(ctx, "vec1", []float32{1, 0, 0})
		require.NoError(t, err)

		// Now search with cancelled context
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err = idx.Search(cancelledCtx, []float32{1, 0, 0}, 1)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestHNSWIndex_LargeIndex(t *testing.T) {
	// Test with enough vectors to exercise shrinkConnections
	cfg := Config{
		Dimension:      3,
		M:              4,  // Small M to force more shrinking
		EfConstruction: 20,
		EfSearch:       10,
	}
	idx := NewHNSWIndex(cfg)
	defer idx.Close()

	ctx := context.Background()

	// Add many vectors to trigger neighbor shrinking
	for i := 0; i < 100; i++ {
		x := float32(i % 10)
		y := float32(i / 10)
		z := float32(i % 3)
		err := idx.Add(ctx, string(rune('a'+i)), []float32{x, y, z})
		require.NoError(t, err)
	}

	assert.Equal(t, 100, idx.Size())

	// Search should still work
	results, err := idx.Search(ctx, []float32{5, 5, 1}, 10)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(results), 10)
}

func TestHNSWIndex_RemoveEntryPoint(t *testing.T) {
	cfg := DefaultConfig(3)
	idx := NewHNSWIndex(cfg)
	defer idx.Close()

	ctx := context.Background()

	// Add vectors
	err := idx.Add(ctx, "vec1", []float32{1, 0, 0})
	require.NoError(t, err)
	err = idx.Add(ctx, "vec2", []float32{0, 1, 0})
	require.NoError(t, err)

	// Remove first vector (likely entry point)
	err = idx.Remove(ctx, "vec1")
	require.NoError(t, err)

	// Index should still work
	assert.Equal(t, 1, idx.Size())

	results, err := idx.Search(ctx, []float32{0, 1, 0}, 1)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "vec2", results[0].ID)
}

func TestHNSWIndex_RemoveAllVectors(t *testing.T) {
	cfg := DefaultConfig(3)
	idx := NewHNSWIndex(cfg)
	defer idx.Close()

	ctx := context.Background()

	// Add and remove all vectors
	err := idx.Add(ctx, "vec1", []float32{1, 0, 0})
	require.NoError(t, err)

	err = idx.Remove(ctx, "vec1")
	require.NoError(t, err)

	assert.Equal(t, 0, idx.Size())

	// Search on empty index
	results, err := idx.Search(ctx, []float32{1, 0, 0}, 5)
	require.NoError(t, err)
	assert.Len(t, results, 0)
}

func TestConfig_Defaults(t *testing.T) {
	t.Run("NewHNSWIndex applies defaults for zero values", func(t *testing.T) {
		cfg := Config{
			Dimension:      3,
			M:              0,  // Should default to 16
			EfConstruction: 0,  // Should default to 200
			EfSearch:       0,  // Should default to 50
		}
		idx := NewHNSWIndex(cfg)
		defer idx.Close()

		// Verify it works (defaults were applied)
		ctx := context.Background()
		err := idx.Add(ctx, "vec1", []float32{1, 0, 0})
		require.NoError(t, err)
	})
}

func TestBruteForceIndex_SearchDimensionMismatch(t *testing.T) {
	idx := NewBruteForceIndex(3)
	defer idx.Close()

	ctx := context.Background()
	err := idx.Add(ctx, "vec1", []float32{1, 0, 0})
	require.NoError(t, err)

	_, err = idx.Search(ctx, []float32{1, 0}, 1) // Wrong dimension
	assert.ErrorIs(t, err, ErrDimensionMismatch)
}

func TestBruteForceIndex_ClosedOperations(t *testing.T) {
	idx := NewBruteForceIndex(3)
	ctx := context.Background()

	err := idx.Add(ctx, "vec1", []float32{1, 0, 0})
	require.NoError(t, err)

	idx.Close()

	t.Run("add fails after close", func(t *testing.T) {
		err := idx.Add(ctx, "vec2", []float32{0, 1, 0})
		assert.ErrorIs(t, err, ErrIndexClosed)
	})

	t.Run("remove fails after close", func(t *testing.T) {
		err := idx.Remove(ctx, "vec1")
		assert.ErrorIs(t, err, ErrIndexClosed)
	})

	t.Run("get fails after close", func(t *testing.T) {
		_, err := idx.Get(ctx, "vec1")
		assert.ErrorIs(t, err, ErrIndexClosed)
	})

	t.Run("search fails after close", func(t *testing.T) {
		_, err := idx.Search(ctx, []float32{1, 0, 0}, 1)
		assert.ErrorIs(t, err, ErrIndexClosed)
	})
}
