package local

import (
	"fmt"
	"os"

	"github.com/ar4mirez/maia/internal/embedding"
)

// NewProviderFromConfig creates a new local embedding provider from config.
// This is the primary entry point for creating a local provider.
func NewProviderFromConfig(cfg ProviderConfig) (*Provider, error) {
	// If paths are not specified, use default locations
	if cfg.ModelPath == "" || cfg.VocabPath == "" {
		files, err := EnsureModelFiles()
		if err != nil {
			return nil, fmt.Errorf("failed to get model files: %w", err)
		}
		if cfg.ModelPath == "" {
			cfg.ModelPath = files.ModelPath
		}
		if cfg.VocabPath == "" {
			cfg.VocabPath = files.VocabPath
		}
	}

	// Load vocabulary
	vocabTxt, err := os.ReadFile(cfg.VocabPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read vocabulary: %w", err)
	}

	// Parse vocab.txt format to JSON
	vocabJSON, err := ParseVocabTxt(vocabTxt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse vocabulary: %w", err)
	}

	// Set defaults
	if cfg.Dimension == 0 {
		cfg.Dimension = 384 // all-MiniLM-L6-v2 dimension
	}
	if cfg.MaxLength == 0 {
		cfg.MaxLength = 256
	}

	return NewProvider(cfg, vocabJSON)
}

// NewProviderFromEmbeddingConfig creates a local provider from embedding.Config.
func NewProviderFromEmbeddingConfig(cfg embedding.Config) (*Provider, error) {
	localCfg := ProviderConfig{
		ModelPath:   cfg.ModelPath,
		Dimension:   cfg.Dimension,
		MaxLength:   256,
		DoLowerCase: true,
	}

	return NewProviderFromConfig(localCfg)
}
