package embedding

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockProvider_Embed(t *testing.T) {
	provider := NewMockProvider(384)
	defer provider.Close()

	ctx := context.Background()

	t.Run("generates embedding for valid text", func(t *testing.T) {
		embedding, err := provider.Embed(ctx, "Hello, world!")
		require.NoError(t, err)
		assert.Len(t, embedding, 384)
	})

	t.Run("returns error for empty text", func(t *testing.T) {
		_, err := provider.Embed(ctx, "")
		assert.ErrorIs(t, err, ErrEmptyText)
	})

	t.Run("generates deterministic embeddings", func(t *testing.T) {
		text := "The quick brown fox"
		emb1, err := provider.Embed(ctx, text)
		require.NoError(t, err)

		emb2, err := provider.Embed(ctx, text)
		require.NoError(t, err)

		assert.Equal(t, emb1, emb2)
	})

	t.Run("generates different embeddings for different texts", func(t *testing.T) {
		emb1, err := provider.Embed(ctx, "Hello")
		require.NoError(t, err)

		emb2, err := provider.Embed(ctx, "Goodbye")
		require.NoError(t, err)

		assert.NotEqual(t, emb1, emb2)
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := provider.Embed(ctx, "test")
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestMockProvider_EmbedBatch(t *testing.T) {
	provider := NewMockProvider(384)
	defer provider.Close()

	ctx := context.Background()

	t.Run("generates embeddings for multiple texts", func(t *testing.T) {
		texts := []string{"Hello", "World", "Test"}
		embeddings, err := provider.EmbedBatch(ctx, texts)
		require.NoError(t, err)
		assert.Len(t, embeddings, 3)
		for _, emb := range embeddings {
			assert.Len(t, emb, 384)
		}
	})

	t.Run("returns error if any text is empty", func(t *testing.T) {
		texts := []string{"Hello", "", "Test"}
		_, err := provider.EmbedBatch(ctx, texts)
		assert.ErrorIs(t, err, ErrEmptyText)
	})

	t.Run("handles empty batch", func(t *testing.T) {
		embeddings, err := provider.EmbedBatch(ctx, []string{})
		require.NoError(t, err)
		assert.Len(t, embeddings, 0)
	})
}

func TestMockProvider_Dimension(t *testing.T) {
	t.Run("returns configured dimension", func(t *testing.T) {
		provider := NewMockProvider(768)
		assert.Equal(t, 768, provider.Dimension())
	})

	t.Run("uses default dimension for invalid value", func(t *testing.T) {
		provider := NewMockProvider(0)
		assert.Equal(t, 384, provider.Dimension())
	})
}

func TestMockProvider_Close(t *testing.T) {
	provider := NewMockProvider(384)

	err := provider.Close()
	require.NoError(t, err)

	_, err = provider.Embed(context.Background(), "test")
	assert.ErrorIs(t, err, ErrProviderClosed)
}

func TestCosineSimilarity(t *testing.T) {
	t.Run("identical vectors have similarity 1", func(t *testing.T) {
		v := []float32{1, 0, 0}
		sim, err := CosineSimilarity(v, v)
		require.NoError(t, err)
		assert.InDelta(t, 1.0, sim, 0.0001)
	})

	t.Run("orthogonal vectors have similarity 0", func(t *testing.T) {
		v1 := []float32{1, 0, 0}
		v2 := []float32{0, 1, 0}
		sim, err := CosineSimilarity(v1, v2)
		require.NoError(t, err)
		assert.InDelta(t, 0.0, sim, 0.0001)
	})

	t.Run("opposite vectors have similarity -1", func(t *testing.T) {
		v1 := []float32{1, 0, 0}
		v2 := []float32{-1, 0, 0}
		sim, err := CosineSimilarity(v1, v2)
		require.NoError(t, err)
		assert.InDelta(t, -1.0, sim, 0.0001)
	})

	t.Run("returns error for mismatched dimensions", func(t *testing.T) {
		v1 := []float32{1, 0, 0}
		v2 := []float32{1, 0}
		_, err := CosineSimilarity(v1, v2)
		assert.ErrorIs(t, err, ErrDimensionMismatch)
	})

	t.Run("handles zero vectors", func(t *testing.T) {
		v1 := []float32{0, 0, 0}
		v2 := []float32{1, 0, 0}
		sim, err := CosineSimilarity(v1, v2)
		require.NoError(t, err)
		assert.Equal(t, float32(0), sim)
	})
}

func TestNormalize(t *testing.T) {
	t.Run("normalizes to unit length", func(t *testing.T) {
		v := []float32{3, 4, 0}
		normalized := Normalize(v)

		// Calculate length
		var length float32
		for _, val := range normalized {
			length += val * val
		}
		length = float32(math.Sqrt(float64(length)))

		assert.InDelta(t, 1.0, length, 0.0001)
	})

	t.Run("preserves direction", func(t *testing.T) {
		v := []float32{3, 4, 0}
		normalized := Normalize(v)

		// Check ratio is preserved
		ratio := v[0] / v[1]
		normalizedRatio := normalized[0] / normalized[1]
		assert.InDelta(t, ratio, normalizedRatio, 0.0001)
	})

	t.Run("handles zero vector", func(t *testing.T) {
		v := []float32{0, 0, 0}
		normalized := Normalize(v)
		assert.Equal(t, v, normalized)
	})

	t.Run("handles empty vector", func(t *testing.T) {
		v := []float32{}
		normalized := Normalize(v)
		assert.Equal(t, v, normalized)
	})
}

func TestMockProvider_SimilarText(t *testing.T) {
	provider := NewMockProvider(384)
	defer provider.Close()

	ctx := context.Background()

	t.Run("high similarity produces similar embeddings", func(t *testing.T) {
		text := "The quick brown fox"
		original, err := provider.Embed(ctx, text)
		require.NoError(t, err)

		similar := provider.SimilarText(text, 0.9)

		sim, err := CosineSimilarity(original, similar)
		require.NoError(t, err)
		// Should be close to 0.9 (allowing some tolerance)
		assert.Greater(t, float64(sim), 0.8)
	})

	t.Run("low similarity produces different embeddings", func(t *testing.T) {
		text := "The quick brown fox"
		original, err := provider.Embed(ctx, text)
		require.NoError(t, err)

		different := provider.SimilarText(text, 0.1)

		sim, err := CosineSimilarity(original, different)
		require.NoError(t, err)
		assert.Less(t, float64(sim), 0.5)
	})
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, "mock", cfg.Provider)
	assert.Equal(t, 384, cfg.Dimension)
	assert.Equal(t, 32, cfg.BatchSize)
}
