package embedding

import (
	"fmt"
)

// ProviderType represents the type of embedding provider.
type ProviderType string

const (
	// ProviderTypeMock uses deterministic mock embeddings (for testing).
	ProviderTypeMock ProviderType = "mock"

	// ProviderTypeLocal uses local ONNX model for embeddings.
	ProviderTypeLocal ProviderType = "local"

	// ProviderTypeOpenAI uses OpenAI API for embeddings.
	ProviderTypeOpenAI ProviderType = "openai"
)

// NewProvider creates a new embedding provider based on configuration.
// For "local" provider, use the local package's NewProviderFromConfig directly
// to avoid import cycles. This function is primarily for mock and remote providers.
func NewProvider(cfg Config) (Provider, error) {
	switch ProviderType(cfg.Provider) {
	case ProviderTypeMock, "":
		dim := cfg.Dimension
		if dim == 0 {
			dim = 384
		}
		return NewMockProvider(dim), nil

	case ProviderTypeLocal:
		// Local provider requires the local package which would create
		// an import cycle. Users should use local.NewProviderFromConfig directly.
		return nil, fmt.Errorf("local provider should be created using local.NewProviderFromConfig")

	case ProviderTypeOpenAI:
		// OpenAI provider not yet implemented
		return nil, fmt.Errorf("openai provider not yet implemented")

	default:
		return nil, fmt.Errorf("unknown provider type: %s", cfg.Provider)
	}
}
