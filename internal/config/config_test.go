// Package config provides configuration management for MAIA.
package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear any existing MAIA_ env vars that might interfere
	clearEnvVars(t)

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify server defaults
	assert.Equal(t, 8080, cfg.Server.HTTPPort)
	assert.Equal(t, 9090, cfg.Server.GRPCPort)
	assert.Equal(t, 100, cfg.Server.MaxConcurrentReqs)
	assert.False(t, cfg.Server.EnableTracing)
	assert.Contains(t, cfg.Server.CORSOrigins, "*")

	// Verify storage defaults
	assert.Equal(t, "./data", cfg.Storage.DataDir)
	assert.False(t, cfg.Storage.SyncWrites)

	// Verify embedding defaults
	assert.Equal(t, "local", cfg.Embedding.Model)
	assert.Equal(t, 384, cfg.Embedding.Dimensions)
	assert.Equal(t, 32, cfg.Embedding.BatchSize)

	// Verify log defaults
	assert.Equal(t, "info", cfg.Log.Level)
	assert.Equal(t, "console", cfg.Log.Format)
	assert.Equal(t, "stdout", cfg.Log.Output)

	// Verify memory defaults
	assert.Equal(t, "default", cfg.Memory.DefaultNamespace)
	assert.Equal(t, 4000, cfg.Memory.DefaultTokenBudget)
	assert.Equal(t, 10000, cfg.Memory.MaxMemorySize)

	// Verify proxy defaults
	assert.True(t, cfg.Proxy.AutoRemember)
	assert.True(t, cfg.Proxy.AutoContext)
	assert.Equal(t, "system", cfg.Proxy.ContextPosition)
	assert.Equal(t, 4000, cfg.Proxy.TokenBudget)

	// Verify security defaults
	assert.False(t, cfg.Security.EnableTLS)
	assert.Equal(t, 100, cfg.Security.RateLimitRPS)
}

func TestLoad_EnvVarOverrides(t *testing.T) {
	clearEnvVars(t)

	// Set environment variables using nested format
	t.Setenv("MAIA_SERVER_HTTP_PORT", "3000")
	t.Setenv("MAIA_SERVER_GRPC_PORT", "3001")
	t.Setenv("MAIA_STORAGE_DATA_DIR", "/tmp/maia-test")
	t.Setenv("MAIA_LOG_LEVEL", "debug")
	t.Setenv("MAIA_EMBEDDING_MODEL", "local")
	t.Setenv("MAIA_MEMORY_DEFAULT_TOKEN_BUDGET", "8000")

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, 3000, cfg.Server.HTTPPort)
	assert.Equal(t, 3001, cfg.Server.GRPCPort)
	assert.Equal(t, "/tmp/maia-test", cfg.Storage.DataDir)
	assert.Equal(t, "debug", cfg.Log.Level)
	assert.Equal(t, "local", cfg.Embedding.Model)
	assert.Equal(t, 8000, cfg.Memory.DefaultTokenBudget)
}

func TestLoad_LegacyEnvVars(t *testing.T) {
	clearEnvVars(t)

	// Set legacy environment variables
	t.Setenv("MAIA_HTTP_PORT", "4000")
	t.Setenv("MAIA_GRPC_PORT", "4001")
	t.Setenv("MAIA_DATA_DIR", "/tmp/legacy-test")
	t.Setenv("MAIA_LOG_LEVEL", "warn")
	t.Setenv("MAIA_TOKEN_BUDGET", "2000")

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, 4000, cfg.Server.HTTPPort)
	assert.Equal(t, 4001, cfg.Server.GRPCPort)
	assert.Equal(t, "/tmp/legacy-test", cfg.Storage.DataDir)
	assert.Equal(t, "warn", cfg.Log.Level)
	assert.Equal(t, 2000, cfg.Memory.DefaultTokenBudget)
}

func TestLoad_ConfigFile(t *testing.T) {
	clearEnvVars(t)

	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "maia.yaml")

	configContent := `
server:
  http_port: 5000
  grpc_port: 5001
storage:
  data_dir: /custom/data
log:
  level: error
  format: json
embedding:
  model: local
  dimensions: 768
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Change to temp dir so viper can find the config
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(origDir)
	}()
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, 5000, cfg.Server.HTTPPort)
	assert.Equal(t, 5001, cfg.Server.GRPCPort)
	assert.Equal(t, "/custom/data", cfg.Storage.DataDir)
	assert.Equal(t, "error", cfg.Log.Level)
	assert.Equal(t, "json", cfg.Log.Format)
	assert.Equal(t, 768, cfg.Embedding.Dimensions)
}

func TestConfig_Validate_ValidConfig(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			HTTPPort: 8080,
			GRPCPort: 9090,
		},
		Storage: StorageConfig{
			DataDir: "./data",
		},
		Embedding: EmbeddingConfig{
			Model: "local",
		},
		Log: LogConfig{
			Level:  "info",
			Format: "console",
		},
		Memory: MemoryConfig{
			DefaultTokenBudget: 4000,
		},
		Security: SecurityConfig{
			EnableTLS: false,
		},
	}

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestConfig_Validate_InvalidHTTPPort(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"port too low", 0},
		{"port negative", -1},
		{"port too high", 65536},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.Server.HTTPPort = tt.port

			err := cfg.Validate()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "invalid HTTP port")
		})
	}
}

func TestConfig_Validate_InvalidGRPCPort(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"port too low", 0},
		{"port negative", -1},
		{"port too high", 65536},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.Server.GRPCPort = tt.port

			err := cfg.Validate()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "invalid gRPC port")
		})
	}
}

func TestConfig_Validate_SamePorts(t *testing.T) {
	cfg := validConfig()
	cfg.Server.HTTPPort = 8080
	cfg.Server.GRPCPort = 8080

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP and gRPC ports must be different")
}

func TestConfig_Validate_EmptyDataDir(t *testing.T) {
	cfg := validConfig()
	cfg.Storage.DataDir = ""

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "data directory is required")
}

func TestConfig_Validate_InvalidEmbeddingModel(t *testing.T) {
	cfg := validConfig()
	cfg.Embedding.Model = "invalid"

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid embedding model")
}

func TestConfig_Validate_OpenAIMissingKey(t *testing.T) {
	cfg := validConfig()
	cfg.Embedding.Model = "openai"
	cfg.Embedding.OpenAIKey = ""

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "OpenAI API key required")
}

func TestConfig_Validate_VoyageMissingKey(t *testing.T) {
	cfg := validConfig()
	cfg.Embedding.Model = "voyage"
	cfg.Embedding.VoyageKey = ""

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Voyage API key required")
}

func TestConfig_Validate_OpenAIWithKey(t *testing.T) {
	cfg := validConfig()
	cfg.Embedding.Model = "openai"
	cfg.Embedding.OpenAIKey = "sk-test-key"

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestConfig_Validate_VoyageWithKey(t *testing.T) {
	cfg := validConfig()
	cfg.Embedding.Model = "voyage"
	cfg.Embedding.VoyageKey = "voyage-test-key"

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestConfig_Validate_InvalidLogLevel(t *testing.T) {
	cfg := validConfig()
	cfg.Log.Level = "invalid"

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid log level")
}

func TestConfig_Validate_InvalidLogFormat(t *testing.T) {
	cfg := validConfig()
	cfg.Log.Format = "xml"

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid log format")
}

func TestConfig_Validate_TokenBudgetTooSmall(t *testing.T) {
	cfg := validConfig()
	cfg.Memory.DefaultTokenBudget = 50

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "token budget too small")
}

func TestConfig_Validate_TLSMissingCert(t *testing.T) {
	cfg := validConfig()
	cfg.Security.EnableTLS = true
	cfg.Security.TLSCertPath = ""
	cfg.Security.TLSKeyPath = "/path/to/key"

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TLS cert and key paths required")
}

func TestConfig_Validate_TLSMissingKey(t *testing.T) {
	cfg := validConfig()
	cfg.Security.EnableTLS = true
	cfg.Security.TLSCertPath = "/path/to/cert"
	cfg.Security.TLSKeyPath = ""

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TLS cert and key paths required")
}

func TestConfig_Validate_TLSValidPaths(t *testing.T) {
	cfg := validConfig()
	cfg.Security.EnableTLS = true
	cfg.Security.TLSCertPath = "/path/to/cert"
	cfg.Security.TLSKeyPath = "/path/to/key"

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestConfig_String(t *testing.T) {
	cfg := validConfig()

	str := cfg.String()
	assert.Contains(t, str, "HTTP: 8080")
	assert.Contains(t, str, "gRPC: 9090")
	assert.Contains(t, str, "Dir: ./data")
	assert.Contains(t, str, "Model: local")
	assert.Contains(t, str, "Level: info")
	// Should not contain sensitive info
	assert.NotContains(t, str, "api_key")
	assert.NotContains(t, str, "openai")
}

func TestConfig_Validate_AllLogLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error"}

	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			cfg := validConfig()
			cfg.Log.Level = level

			err := cfg.Validate()
			assert.NoError(t, err)
		})
	}
}

func TestConfig_Validate_AllLogFormats(t *testing.T) {
	formats := []string{"json", "console"}

	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			cfg := validConfig()
			cfg.Log.Format = format

			err := cfg.Validate()
			assert.NoError(t, err)
		})
	}
}

func TestConfig_Validate_AllEmbeddingModels(t *testing.T) {
	tests := []struct {
		model string
		key   string
	}{
		{"local", ""},
		{"openai", "sk-test"},
		{"voyage", "voyage-test"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			cfg := validConfig()
			cfg.Embedding.Model = tt.model
			cfg.Embedding.OpenAIKey = ""
			cfg.Embedding.VoyageKey = ""

			if tt.model == "openai" {
				cfg.Embedding.OpenAIKey = tt.key
			}
			if tt.model == "voyage" {
				cfg.Embedding.VoyageKey = tt.key
			}

			err := cfg.Validate()
			assert.NoError(t, err)
		})
	}
}

// validConfig returns a valid configuration for testing.
func validConfig() *Config {
	return &Config{
		Server: ServerConfig{
			HTTPPort: 8080,
			GRPCPort: 9090,
		},
		Storage: StorageConfig{
			DataDir: "./data",
		},
		Embedding: EmbeddingConfig{
			Model: "local",
		},
		Log: LogConfig{
			Level:  "info",
			Format: "console",
		},
		Memory: MemoryConfig{
			DefaultTokenBudget: 4000,
		},
		Security: SecurityConfig{
			EnableTLS: false,
		},
	}
}

// clearEnvVars unsets all MAIA_ environment variables.
func clearEnvVars(t *testing.T) {
	t.Helper()

	envVars := []string{
		"MAIA_SERVER_HTTP_PORT",
		"MAIA_SERVER_GRPC_PORT",
		"MAIA_STORAGE_DATA_DIR",
		"MAIA_LOG_LEVEL",
		"MAIA_LOG_FORMAT",
		"MAIA_EMBEDDING_MODEL",
		"MAIA_EMBEDDING_OPENAI_API_KEY",
		"MAIA_EMBEDDING_VOYAGE_API_KEY",
		"MAIA_MEMORY_DEFAULT_TOKEN_BUDGET",
		"MAIA_SECURITY_ENABLE_TLS",
		"MAIA_HTTP_PORT",
		"MAIA_GRPC_PORT",
		"MAIA_DATA_DIR",
		"MAIA_TOKEN_BUDGET",
		"MAIA_API_KEY",
	}

	for _, env := range envVars {
		t.Setenv(env, "")
		os.Unsetenv(env)
	}
}
