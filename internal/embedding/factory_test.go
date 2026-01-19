package embedding

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProvider_Mock(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		wantDim   int
		wantErr   bool
	}{
		{
			name: "explicit mock provider",
			config: Config{
				Provider:  "mock",
				Dimension: 384,
			},
			wantDim: 384,
			wantErr: false,
		},
		{
			name: "empty provider defaults to mock",
			config: Config{
				Provider:  "",
				Dimension: 256,
			},
			wantDim: 256,
			wantErr: false,
		},
		{
			name: "mock with default dimension",
			config: Config{
				Provider: "mock",
			},
			wantDim: 384,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewProvider(tt.config)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, provider)

			assert.Equal(t, tt.wantDim, provider.Dimension())

			// Test embedding generation
			ctx := context.Background()
			embedding, err := provider.Embed(ctx, "test text")
			require.NoError(t, err)
			assert.Len(t, embedding, tt.wantDim)

			// Clean up
			err = provider.Close()
			require.NoError(t, err)
		})
	}
}

func TestNewProvider_Local(t *testing.T) {
	config := Config{
		Provider: "local",
	}

	_, err := NewProvider(config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "local.NewProviderFromConfig")
}

func TestNewProvider_OpenAI(t *testing.T) {
	config := Config{
		Provider: "openai",
	}

	_, err := NewProvider(config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}

func TestNewProvider_Unknown(t *testing.T) {
	config := Config{
		Provider: "unknown_provider",
	}

	_, err := NewProvider(config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider type")
}

func TestProviderType_Constants(t *testing.T) {
	assert.Equal(t, ProviderType("mock"), ProviderTypeMock)
	assert.Equal(t, ProviderType("local"), ProviderTypeLocal)
	assert.Equal(t, ProviderType("openai"), ProviderTypeOpenAI)
}
