// Package embedding provides text embedding generation for MAIA.
package embedding

import (
	"context"
	"errors"
)

// Common errors for embedding operations.
var (
	ErrEmptyText       = errors.New("text cannot be empty")
	ErrProviderClosed  = errors.New("embedding provider is closed")
	ErrDimensionMismatch = errors.New("embedding dimension mismatch")
)

// Provider defines the interface for embedding generation.
type Provider interface {
	// Embed generates an embedding vector for the given text.
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch generates embeddings for multiple texts.
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

	// Dimension returns the dimension of the embedding vectors.
	Dimension() int

	// Close releases any resources held by the provider.
	Close() error
}

// Config holds configuration for embedding providers.
type Config struct {
	// Provider specifies which embedding provider to use.
	// Supported values: "local", "openai", "mock"
	Provider string `mapstructure:"provider"`

	// ModelPath is the path to the local model file (for local provider).
	ModelPath string `mapstructure:"model_path"`

	// Dimension is the embedding dimension (required for mock provider).
	Dimension int `mapstructure:"dimension"`

	// APIKey for remote providers (OpenAI, etc.)
	APIKey string `mapstructure:"api_key"`

	// BaseURL for remote providers.
	BaseURL string `mapstructure:"base_url"`

	// Model name for remote providers.
	Model string `mapstructure:"model"`

	// BatchSize for batch operations.
	BatchSize int `mapstructure:"batch_size"`
}

// DefaultConfig returns the default embedding configuration.
func DefaultConfig() Config {
	return Config{
		Provider:  "mock",
		Dimension: 384, // Common dimension for small models
		BatchSize: 32,
	}
}

// CosineSimilarity calculates the cosine similarity between two vectors.
// Returns a value between -1 and 1, where 1 means identical direction.
func CosineSimilarity(a, b []float32) (float32, error) {
	if len(a) != len(b) {
		return 0, ErrDimensionMismatch
	}
	if len(a) == 0 {
		return 0, errors.New("vectors cannot be empty")
	}

	var dotProduct, normA, normB float32
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0, nil
	}

	return dotProduct / (sqrt(normA) * sqrt(normB)), nil
}

// sqrt is a simple square root implementation for float32.
func sqrt(x float32) float32 {
	if x <= 0 {
		return 0
	}
	// Newton's method
	z := x / 2
	for i := 0; i < 10; i++ {
		z = z - (z*z-x)/(2*z)
	}
	return z
}

// Normalize normalizes a vector to unit length.
func Normalize(v []float32) []float32 {
	if len(v) == 0 {
		return v
	}

	var norm float32
	for _, val := range v {
		norm += val * val
	}
	norm = sqrt(norm)

	if norm == 0 {
		return v
	}

	result := make([]float32, len(v))
	for i, val := range v {
		result[i] = val / norm
	}
	return result
}
