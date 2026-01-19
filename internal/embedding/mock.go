package embedding

import (
	"context"
	"hash/fnv"
	"math"
	"sync"
)

// MockProvider is a deterministic embedding provider for testing.
// It generates consistent embeddings based on text content hash.
type MockProvider struct {
	dimension int
	closed    bool
	mu        sync.RWMutex
}

// NewMockProvider creates a new mock embedding provider.
func NewMockProvider(dimension int) *MockProvider {
	if dimension <= 0 {
		dimension = 384
	}
	return &MockProvider{
		dimension: dimension,
	}
}

// Embed generates a deterministic embedding based on text hash.
func (p *MockProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return nil, ErrProviderClosed
	}
	p.mu.RUnlock()

	if text == "" {
		return nil, ErrEmptyText
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return p.generateEmbedding(text), nil
}

// EmbedBatch generates embeddings for multiple texts.
func (p *MockProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return nil, ErrProviderClosed
	}
	p.mu.RUnlock()

	results := make([][]float32, len(texts))
	for i, text := range texts {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if text == "" {
			return nil, ErrEmptyText
		}
		results[i] = p.generateEmbedding(text)
	}
	return results, nil
}

// Dimension returns the embedding dimension.
func (p *MockProvider) Dimension() int {
	return p.dimension
}

// Close closes the provider.
func (p *MockProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	return nil
}

// generateEmbedding creates a deterministic embedding from text.
// Uses FNV hash to seed a simple PRNG for reproducible results.
func (p *MockProvider) generateEmbedding(text string) []float32 {
	h := fnv.New64a()
	h.Write([]byte(text))
	seed := h.Sum64()

	embedding := make([]float32, p.dimension)

	// Simple LCG-based PRNG for deterministic values
	state := seed
	for i := 0; i < p.dimension; i++ {
		// LCG parameters (same as glibc)
		state = state*1103515245 + 12345
		// Convert to float32 in range [-1, 1]
		normalized := float64(state&0x7FFFFFFF) / float64(0x7FFFFFFF)
		embedding[i] = float32(normalized*2 - 1)
	}

	// Normalize to unit length
	return Normalize(embedding)
}

// SimilarText generates an embedding that is similar to the given text.
// Useful for testing similarity search with controlled results.
func (p *MockProvider) SimilarText(text string, similarity float64) []float32 {
	base := p.generateEmbedding(text)

	// Generate a random perturbation
	h := fnv.New64a()
	h.Write([]byte(text + "_perturb"))
	seed := h.Sum64()

	perturbation := make([]float32, p.dimension)
	state := seed
	for i := 0; i < p.dimension; i++ {
		state = state*1103515245 + 12345
		normalized := float64(state&0x7FFFFFFF) / float64(0x7FFFFFFF)
		perturbation[i] = float32(normalized*2 - 1)
	}
	perturbation = Normalize(perturbation)

	// Blend base and perturbation based on similarity
	// Higher similarity = more of the base vector
	result := make([]float32, p.dimension)
	factor := float32(math.Sqrt(1 - similarity*similarity))
	for i := range result {
		result[i] = float32(similarity)*base[i] + factor*perturbation[i]
	}

	return Normalize(result)
}
