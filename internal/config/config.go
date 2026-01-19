// Package config provides configuration management for MAIA.
// It supports loading configuration from environment variables and config files.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for MAIA.
type Config struct {
	// Server configuration
	Server ServerConfig `mapstructure:"server"`

	// Storage configuration
	Storage StorageConfig `mapstructure:"storage"`

	// Embedding configuration
	Embedding EmbeddingConfig `mapstructure:"embedding"`

	// Logging configuration
	Log LogConfig `mapstructure:"log"`

	// Memory configuration
	Memory MemoryConfig `mapstructure:"memory"`

	// Proxy configuration
	Proxy ProxyConfig `mapstructure:"proxy"`

	// Security configuration
	Security SecurityConfig `mapstructure:"security"`
}

// ServerConfig holds HTTP and gRPC server settings.
type ServerConfig struct {
	HTTPPort             int           `mapstructure:"http_port"`
	GRPCPort             int           `mapstructure:"grpc_port"`
	MaxConcurrentReqs    int           `mapstructure:"max_concurrent_requests"`
	RequestTimeout       time.Duration `mapstructure:"request_timeout"`
	EnableTracing        bool          `mapstructure:"enable_tracing"`
	CORSOrigins          []string      `mapstructure:"cors_origins"`
	ShutdownGracePeriod  time.Duration `mapstructure:"shutdown_grace_period"`
}

// StorageConfig holds storage backend settings.
type StorageConfig struct {
	DataDir           string        `mapstructure:"data_dir"`
	SyncWrites        bool          `mapstructure:"sync_writes"`
	CompactionInterval time.Duration `mapstructure:"compaction_interval"`
	MaxTableSize      int64         `mapstructure:"max_table_size"`
}

// EmbeddingConfig holds embedding model settings.
type EmbeddingConfig struct {
	Model         string `mapstructure:"model"` // local, openai, voyage
	OpenAIKey     string `mapstructure:"openai_api_key"`
	VoyageKey     string `mapstructure:"voyage_api_key"`
	LocalModelPath string `mapstructure:"local_model_path"`
	Dimensions    int    `mapstructure:"dimensions"`
	BatchSize     int    `mapstructure:"batch_size"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level  string `mapstructure:"level"`  // debug, info, warn, error
	Format string `mapstructure:"format"` // json, console
	Output string `mapstructure:"output"` // stdout, file path
}

// MemoryConfig holds memory/context settings.
type MemoryConfig struct {
	DefaultNamespace string `mapstructure:"default_namespace"`
	DefaultTokenBudget int  `mapstructure:"default_token_budget"`
	MaxMemorySize    int    `mapstructure:"max_memory_size"`
	ConsolidationInterval time.Duration `mapstructure:"consolidation_interval"`
}

// ProxyConfig holds OpenAI-compatible proxy settings.
type ProxyConfig struct {
	Backend         string `mapstructure:"backend"`
	AutoRemember    bool   `mapstructure:"auto_remember"`
	AutoContext     bool   `mapstructure:"auto_context"`
	ContextPosition string `mapstructure:"context_position"` // system, first_user, before_last
	TokenBudget     int    `mapstructure:"token_budget"`
}

// SecurityConfig holds security settings.
type SecurityConfig struct {
	APIKey       string `mapstructure:"api_key"`
	EnableTLS    bool   `mapstructure:"enable_tls"`
	TLSCertPath  string `mapstructure:"tls_cert_path"`
	TLSKeyPath   string `mapstructure:"tls_key_path"`
	RateLimitRPS int    `mapstructure:"rate_limit_rps"`
}

// Default configuration values.
var defaults = map[string]interface{}{
	// Server defaults
	"server.http_port":               8080,
	"server.grpc_port":               9090,
	"server.max_concurrent_requests": 100,
	"server.request_timeout":         "30s",
	"server.enable_tracing":          false,
	"server.cors_origins":            []string{"*"},
	"server.shutdown_grace_period":   "10s",

	// Storage defaults
	"storage.data_dir":            "./data",
	"storage.sync_writes":         false,
	"storage.compaction_interval": "1h",
	"storage.max_table_size":      int64(64 << 20), // 64MB

	// Embedding defaults
	"embedding.model":      "local",
	"embedding.dimensions": 384, // all-MiniLM-L6-v2 dimension
	"embedding.batch_size": 32,

	// Log defaults
	"log.level":  "info",
	"log.format": "console",
	"log.output": "stdout",

	// Memory defaults
	"memory.default_namespace":       "default",
	"memory.default_token_budget":    4000,
	"memory.max_memory_size":         10000,
	"memory.consolidation_interval":  "24h",

	// Proxy defaults
	"proxy.backend":          "",
	"proxy.auto_remember":    true,
	"proxy.auto_context":     true,
	"proxy.context_position": "system",
	"proxy.token_budget":     4000,

	// Security defaults
	"security.api_key":        "",
	"security.enable_tls":     false,
	"security.rate_limit_rps": 100,
}

// Load loads configuration from environment variables and optional config file.
// Environment variables are prefixed with MAIA_ and use underscores.
// Example: MAIA_SERVER_HTTP_PORT=8080
func Load() (*Config, error) {
	v := viper.New()

	// Set defaults
	for key, value := range defaults {
		v.SetDefault(key, value)
	}

	// Environment variables
	v.SetEnvPrefix("MAIA")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Map legacy flat env vars to nested structure
	bindLegacyEnvVars(v)

	// Try to read config file (optional)
	v.SetConfigName("maia")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")
	v.AddConfigPath("/etc/maia")
	v.AddConfigPath("$HOME/.maia")

	// It's okay if config file doesn't exist
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// bindLegacyEnvVars maps flat MAIA_* env vars to nested structure for backward compatibility.
func bindLegacyEnvVars(v *viper.Viper) {
	// Map legacy env vars
	legacyMappings := map[string]string{
		"DATA_DIR":               "storage.data_dir",
		"HTTP_PORT":              "server.http_port",
		"GRPC_PORT":              "server.grpc_port",
		"LOG_LEVEL":              "log.level",
		"LOG_FORMAT":             "log.format",
		"EMBEDDING_MODEL":        "embedding.model",
		"OPENAI_API_KEY":         "embedding.openai_api_key",
		"VOYAGE_API_KEY":         "embedding.voyage_api_key",
		"DEFAULT_NAMESPACE":      "memory.default_namespace",
		"TOKEN_BUDGET":           "memory.default_token_budget",
		"API_KEY":                "security.api_key",
		"ENABLE_TLS":             "security.enable_tls",
		"TLS_CERT_PATH":          "security.tls_cert_path",
		"TLS_KEY_PATH":           "security.tls_key_path",
		"CORS_ORIGINS":           "server.cors_origins",
		"MAX_CONCURRENT_REQUESTS": "server.max_concurrent_requests",
		"REQUEST_TIMEOUT":        "server.request_timeout",
		"ENABLE_TRACING":         "server.enable_tracing",
		"PROXY_BACKEND":          "proxy.backend",
		"PROXY_AUTO_REMEMBER":    "proxy.auto_remember",
		"PROXY_AUTO_CONTEXT":     "proxy.auto_context",
		"PROXY_CONTEXT_POSITION": "proxy.context_position",
	}

	for envSuffix, configKey := range legacyMappings {
		_ = v.BindEnv(configKey, "MAIA_"+envSuffix)
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	// Validate server config
	if c.Server.HTTPPort < 1 || c.Server.HTTPPort > 65535 {
		return fmt.Errorf("invalid HTTP port: %d", c.Server.HTTPPort)
	}
	if c.Server.GRPCPort < 1 || c.Server.GRPCPort > 65535 {
		return fmt.Errorf("invalid gRPC port: %d", c.Server.GRPCPort)
	}
	if c.Server.HTTPPort == c.Server.GRPCPort {
		return fmt.Errorf("HTTP and gRPC ports must be different")
	}

	// Validate storage config
	if c.Storage.DataDir == "" {
		return fmt.Errorf("data directory is required")
	}

	// Validate embedding config
	validModels := map[string]bool{"local": true, "openai": true, "voyage": true}
	if !validModels[c.Embedding.Model] {
		return fmt.Errorf("invalid embedding model: %s (valid: local, openai, voyage)", c.Embedding.Model)
	}
	if c.Embedding.Model == "openai" && c.Embedding.OpenAIKey == "" {
		return fmt.Errorf("OpenAI API key required when using openai embedding model")
	}
	if c.Embedding.Model == "voyage" && c.Embedding.VoyageKey == "" {
		return fmt.Errorf("Voyage API key required when using voyage embedding model")
	}

	// Validate log config
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[c.Log.Level] {
		return fmt.Errorf("invalid log level: %s (valid: debug, info, warn, error)", c.Log.Level)
	}
	validFormats := map[string]bool{"json": true, "console": true}
	if !validFormats[c.Log.Format] {
		return fmt.Errorf("invalid log format: %s (valid: json, console)", c.Log.Format)
	}

	// Validate memory config
	if c.Memory.DefaultTokenBudget < 100 {
		return fmt.Errorf("token budget too small: %d (minimum: 100)", c.Memory.DefaultTokenBudget)
	}

	// Validate TLS config
	if c.Security.EnableTLS {
		if c.Security.TLSCertPath == "" || c.Security.TLSKeyPath == "" {
			return fmt.Errorf("TLS cert and key paths required when TLS is enabled")
		}
	}

	return nil
}

// String returns a string representation of the config (without sensitive values).
func (c *Config) String() string {
	return fmt.Sprintf(
		"Config{Server: {HTTP: %d, gRPC: %d}, Storage: {Dir: %s}, Embedding: {Model: %s}, Log: {Level: %s}}",
		c.Server.HTTPPort,
		c.Server.GRPCPort,
		c.Storage.DataDir,
		c.Embedding.Model,
		c.Log.Level,
	)
}
