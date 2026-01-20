// Package inference provides LLM inference capabilities for MAIA.
// It supports multiple backends (Ollama, OpenRouter, Anthropic) with
// automatic routing based on model names.
package inference

import (
	"context"
	"errors"
	"time"
)

// Common errors for inference operations.
var (
	ErrProviderClosed    = errors.New("inference provider is closed")
	ErrNoProviderFound   = errors.New("no provider found for model")
	ErrEmptyMessages     = errors.New("messages cannot be empty")
	ErrInvalidModel      = errors.New("invalid model specified")
	ErrProviderUnhealthy = errors.New("provider is unhealthy")
	ErrStreamNotSupported = errors.New("streaming not supported by this provider")
)

// Provider defines the interface for inference backends.
type Provider interface {
	// Complete performs a non-streaming completion request.
	Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)

	// Stream performs a streaming completion request.
	// Returns a StreamReader that yields chunks until EOF.
	Stream(ctx context.Context, req *CompletionRequest) (StreamReader, error)

	// ListModels returns the models available from this provider.
	ListModels(ctx context.Context) ([]Model, error)

	// Name returns the provider name (e.g., "ollama", "openrouter").
	Name() string

	// SupportsModel returns true if this provider can handle the given model.
	SupportsModel(modelID string) bool

	// Health checks if the provider is available and healthy.
	Health(ctx context.Context) error

	// Close releases any resources held by the provider.
	Close() error
}

// StreamReader reads streaming completion chunks.
type StreamReader interface {
	// Read returns the next chunk from the stream.
	// Returns io.EOF when the stream is complete.
	Read() (*CompletionChunk, error)

	// Close closes the stream and releases resources.
	Close() error
}

// Router routes completion requests to appropriate providers.
type Router interface {
	// Route selects the appropriate provider for a model.
	Route(ctx context.Context, modelID string) (Provider, error)

	// RegisterProvider adds a provider to the router.
	RegisterProvider(p Provider) error

	// ListProviders returns all registered providers.
	ListProviders() []Provider

	// Close closes all registered providers.
	Close() error
}

// CompletionRequest represents a chat completion request.
type CompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature *float64  `json:"temperature,omitempty"`
	TopP        *float64  `json:"top_p,omitempty"`
	MaxTokens   *int      `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
	Stop        []string  `json:"stop,omitempty"`
	User        string    `json:"user,omitempty"`

	// MAIA-specific fields (not sent to providers)
	Namespace   string `json:"-"`
	TokenBudget int    `json:"-"`
}

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

// CompletionResponse represents a chat completion response.
type CompletionResponse struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"`
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	SystemFingerprint string   `json:"system_fingerprint,omitempty"`
	Choices           []Choice `json:"choices"`
	Usage             *Usage   `json:"usage,omitempty"`
}

// Choice represents a completion choice.
type Choice struct {
	Index        int      `json:"index"`
	Message      *Message `json:"message,omitempty"`
	Delta        *Message `json:"delta,omitempty"`
	FinishReason string   `json:"finish_reason,omitempty"`
}

// Usage represents token usage information.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// CompletionChunk represents a streaming completion chunk.
type CompletionChunk struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"`
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	SystemFingerprint string   `json:"system_fingerprint,omitempty"`
	Choices           []Choice `json:"choices"`
}

// Model represents an available model.
type Model struct {
	ID       string `json:"id"`
	Object   string `json:"object"`
	Created  int64  `json:"created,omitempty"`
	OwnedBy  string `json:"owned_by,omitempty"`
	Provider string `json:"provider,omitempty"`
}

// Config holds configuration for the inference system.
type Config struct {
	// Enabled controls whether inference is active.
	Enabled bool `mapstructure:"enabled"`

	// DefaultProvider is the fallback provider when no routing rule matches.
	DefaultProvider string `mapstructure:"default_provider"`

	// Providers maps provider names to their configurations.
	Providers map[string]ProviderConfig `mapstructure:"providers"`

	// Routing holds routing configuration.
	Routing RoutingConfig `mapstructure:"routing"`

	// Cache holds caching configuration.
	Cache CacheConfig `mapstructure:"cache"`
}

// ProviderConfig holds configuration for a single provider.
type ProviderConfig struct {
	// Type specifies the provider type: "ollama", "openrouter", "anthropic".
	Type string `mapstructure:"type"`

	// BaseURL is the API endpoint URL.
	BaseURL string `mapstructure:"base_url"`

	// APIKey is the API key for authenticated providers.
	APIKey string `mapstructure:"api_key"`

	// Models is an optional list of models this provider supports.
	// If empty, the provider reports its own model list.
	Models []string `mapstructure:"models"`

	// Timeout is the request timeout.
	Timeout time.Duration `mapstructure:"timeout"`

	// MaxRetries is the number of retry attempts on failure.
	MaxRetries int `mapstructure:"max_retries"`
}

// RoutingConfig holds routing configuration.
type RoutingConfig struct {
	// ModelMapping maps model patterns to provider names.
	// Patterns support wildcards: "llama*" matches "llama2", "llama3", etc.
	ModelMapping map[string]string `mapstructure:"model_mapping"`
}

// CacheConfig holds caching configuration.
type CacheConfig struct {
	// Enabled controls whether response caching is active.
	Enabled bool `mapstructure:"enabled"`

	// TTL is the cache entry time-to-live.
	TTL time.Duration `mapstructure:"ttl"`

	// Namespace is the MAIA namespace for cached responses.
	Namespace string `mapstructure:"namespace"`

	// MaxEntries is the maximum number of cached responses.
	MaxEntries int `mapstructure:"max_entries"`
}

// DefaultConfig returns the default inference configuration.
func DefaultConfig() Config {
	return Config{
		Enabled:         false, // Opt-in
		DefaultProvider: "ollama",
		Providers:       make(map[string]ProviderConfig),
		Routing: RoutingConfig{
			ModelMapping: make(map[string]string),
		},
		Cache: CacheConfig{
			Enabled:   false,
			TTL:       24 * time.Hour,
			Namespace: "inference:cache",
		},
	}
}

// Accumulator accumulates streaming chunks into a complete response.
type Accumulator struct {
	ID                string
	Object            string
	Created           int64
	Model             string
	SystemFingerprint string
	Contents          []string
	FinishReasons     []string
}

// NewAccumulator creates a new accumulator.
func NewAccumulator() *Accumulator {
	return &Accumulator{
		Contents:      []string{},
		FinishReasons: []string{},
	}
}

// Add adds a chunk to the accumulator.
func (a *Accumulator) Add(chunk *CompletionChunk) {
	if a.ID == "" {
		a.ID = chunk.ID
		a.Object = chunk.Object
		a.Created = chunk.Created
		a.Model = chunk.Model
		a.SystemFingerprint = chunk.SystemFingerprint
	}

	for i, choice := range chunk.Choices {
		for len(a.Contents) <= i {
			a.Contents = append(a.Contents, "")
			a.FinishReasons = append(a.FinishReasons, "")
		}

		if choice.Delta != nil {
			a.Contents[i] += choice.Delta.Content
		}

		if choice.FinishReason != "" {
			a.FinishReasons[i] = choice.FinishReason
		}
	}
}

// ToResponse converts the accumulated data to a CompletionResponse.
func (a *Accumulator) ToResponse() *CompletionResponse {
	choices := make([]Choice, len(a.Contents))
	for i := range choices {
		choices[i] = Choice{
			Index: i,
			Message: &Message{
				Role:    "assistant",
				Content: a.Contents[i],
			},
			FinishReason: a.FinishReasons[i],
		}
	}

	return &CompletionResponse{
		ID:                a.ID,
		Object:            "chat.completion",
		Created:           a.Created,
		Model:             a.Model,
		SystemFingerprint: a.SystemFingerprint,
		Choices:           choices,
	}
}

// GetContent returns the accumulated content for the first choice.
func (a *Accumulator) GetContent() string {
	if len(a.Contents) == 0 {
		return ""
	}
	return a.Contents[0]
}

