package inference

import (
	"fmt"
)

// ProviderType represents the type of inference provider.
type ProviderType string

const (
	// ProviderTypeMock uses deterministic mock responses (for testing).
	ProviderTypeMock ProviderType = "mock"

	// ProviderTypeOllama uses Ollama for local inference.
	ProviderTypeOllama ProviderType = "ollama"

	// ProviderTypeOpenRouter uses OpenRouter for multi-model API access.
	ProviderTypeOpenRouter ProviderType = "openrouter"

	// ProviderTypeAnthropic uses Anthropic API for Claude models.
	ProviderTypeAnthropic ProviderType = "anthropic"
)

// NewProvider creates a new inference provider based on configuration.
// For providers in separate packages (to avoid import cycles), use
// the provider-specific NewProvider functions directly.
func NewProvider(name string, cfg ProviderConfig) (Provider, error) {
	switch ProviderType(cfg.Type) {
	case ProviderTypeMock, "":
		return NewMockProvider(name), nil

	case ProviderTypeOllama:
		// Ollama provider is in providers/ollama package
		// This is here for documentation; use ollama.NewProvider directly
		return nil, fmt.Errorf("ollama provider should be created using ollama.NewProvider")

	case ProviderTypeOpenRouter:
		// OpenRouter provider is in providers/openrouter package
		return nil, fmt.Errorf("openrouter provider should be created using openrouter.NewProvider")

	case ProviderTypeAnthropic:
		// Anthropic provider is in providers/anthropic package
		return nil, fmt.Errorf("anthropic provider should be created using anthropic.NewProvider")

	default:
		return nil, fmt.Errorf("unknown provider type: %s", cfg.Type)
	}
}

// NewProviderFromType creates a provider based on type string.
// This is a convenience function that delegates to the appropriate
// provider package. It requires the caller to import the provider packages.
func NewProviderFromType(name string, cfg ProviderConfig) (Provider, error) {
	// For now, we only support mock directly.
	// Other providers must be created via their specific packages.
	if cfg.Type == "" || cfg.Type == string(ProviderTypeMock) {
		return NewMockProvider(name), nil
	}

	return nil, fmt.Errorf(
		"provider type %q requires direct import; use the provider-specific package",
		cfg.Type,
	)
}
