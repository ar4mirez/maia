// Package vector provides vector similarity search functionality for MAIA.
package vector

import (
	"context"
	"errors"
	"sync"

	"github.com/ar4mirez/maia/internal/embedding"
)

// Common errors for vector index operations.
var (
	ErrIndexClosed     = errors.New("vector index is closed")
	ErrNotFound        = errors.New("vector not found")
	ErrDimensionMismatch = errors.New("vector dimension mismatch")
	ErrInvalidK        = errors.New("k must be positive")
)

// Index defines the interface for vector similarity search.
type Index interface {
	// Add adds a vector with the given ID to the index.
	Add(ctx context.Context, id string, vector []float32) error

	// AddBatch adds multiple vectors to the index.
	AddBatch(ctx context.Context, ids []string, vectors [][]float32) error

	// Remove removes a vector from the index.
	Remove(ctx context.Context, id string) error

	// Search finds the k nearest neighbors to the query vector.
	Search(ctx context.Context, query []float32, k int) ([]SearchResult, error)

	// Get retrieves a vector by ID.
	Get(ctx context.Context, id string) ([]float32, error)

	// Size returns the number of vectors in the index.
	Size() int

	// Dimension returns the vector dimension.
	Dimension() int

	// Close releases resources held by the index.
	Close() error
}

// SearchResult represents a single search result.
type SearchResult struct {
	ID       string
	Score    float32 // Cosine similarity score (higher is more similar)
	Distance float32 // Distance (lower is more similar)
}

// Config holds configuration for vector indices.
type Config struct {
	// Dimension of the vectors.
	Dimension int

	// M is the number of connections per node in HNSW.
	// Higher values improve recall but increase memory and build time.
	M int

	// EfConstruction is the size of the dynamic candidate list during construction.
	// Higher values improve index quality but increase build time.
	EfConstruction int

	// EfSearch is the size of the dynamic candidate list during search.
	// Higher values improve recall but increase search time.
	EfSearch int
}

// DefaultConfig returns the default vector index configuration.
func DefaultConfig(dimension int) Config {
	return Config{
		Dimension:      dimension,
		M:              16,
		EfConstruction: 200,
		EfSearch:       50,
	}
}

// BruteForceIndex is a simple brute-force vector index.
// Suitable for small datasets or as a baseline for testing.
type BruteForceIndex struct {
	dimension int
	vectors   map[string][]float32
	mu        sync.RWMutex
	closed    bool
}

// NewBruteForceIndex creates a new brute-force vector index.
func NewBruteForceIndex(dimension int) *BruteForceIndex {
	return &BruteForceIndex{
		dimension: dimension,
		vectors:   make(map[string][]float32),
	}
}

// Add adds a vector to the index.
func (idx *BruteForceIndex) Add(ctx context.Context, id string, vector []float32) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.closed {
		return ErrIndexClosed
	}

	if len(vector) != idx.dimension {
		return ErrDimensionMismatch
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Store a copy
	v := make([]float32, len(vector))
	copy(v, vector)
	idx.vectors[id] = v
	return nil
}

// AddBatch adds multiple vectors to the index.
func (idx *BruteForceIndex) AddBatch(ctx context.Context, ids []string, vectors [][]float32) error {
	if len(ids) != len(vectors) {
		return errors.New("ids and vectors must have the same length")
	}

	for i := range ids {
		if err := idx.Add(ctx, ids[i], vectors[i]); err != nil {
			return err
		}
	}
	return nil
}

// Remove removes a vector from the index.
func (idx *BruteForceIndex) Remove(ctx context.Context, id string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.closed {
		return ErrIndexClosed
	}

	if _, exists := idx.vectors[id]; !exists {
		return ErrNotFound
	}

	delete(idx.vectors, id)
	return nil
}

// Search finds the k nearest neighbors to the query vector.
func (idx *BruteForceIndex) Search(ctx context.Context, query []float32, k int) ([]SearchResult, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.closed {
		return nil, ErrIndexClosed
	}

	if len(query) != idx.dimension {
		return nil, ErrDimensionMismatch
	}

	if k <= 0 {
		return nil, ErrInvalidK
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Calculate similarity for all vectors
	type candidate struct {
		id    string
		score float32
	}
	candidates := make([]candidate, 0, len(idx.vectors))

	for id, vector := range idx.vectors {
		score, err := embedding.CosineSimilarity(query, vector)
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{id: id, score: score})
	}

	// Sort by score descending (higher similarity first)
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].score > candidates[i].score {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	// Return top k
	if k > len(candidates) {
		k = len(candidates)
	}

	results := make([]SearchResult, k)
	for i := 0; i < k; i++ {
		results[i] = SearchResult{
			ID:       candidates[i].id,
			Score:    candidates[i].score,
			Distance: 1 - candidates[i].score, // Convert similarity to distance
		}
	}

	return results, nil
}

// Get retrieves a vector by ID.
func (idx *BruteForceIndex) Get(ctx context.Context, id string) ([]float32, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.closed {
		return nil, ErrIndexClosed
	}

	vector, exists := idx.vectors[id]
	if !exists {
		return nil, ErrNotFound
	}

	// Return a copy
	result := make([]float32, len(vector))
	copy(result, vector)
	return result, nil
}

// Size returns the number of vectors in the index.
func (idx *BruteForceIndex) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.vectors)
}

// Dimension returns the vector dimension.
func (idx *BruteForceIndex) Dimension() int {
	return idx.dimension
}

// Close closes the index.
func (idx *BruteForceIndex) Close() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.closed = true
	idx.vectors = nil
	return nil
}
