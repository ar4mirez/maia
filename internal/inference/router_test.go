package inference

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRouter(t *testing.T) {
	cfg := RoutingConfig{
		ModelMapping: map[string]string{
			"llama*":   "ollama",
			"gpt*":     "openrouter",
			"claude*":  "anthropic",
		},
	}

	router := NewRouter(cfg, "ollama")
	assert.NotNil(t, router)
}

func TestRouter_RegisterProvider(t *testing.T) {
	router := NewRouter(RoutingConfig{}, "test")

	provider := NewMockProvider("test-provider")
	err := router.RegisterProvider(provider)
	assert.NoError(t, err)

	providers := router.ListProviders()
	assert.Len(t, providers, 1)
	assert.Equal(t, "test-provider", providers[0].Name())
}

func TestRouter_RegisterProvider_Nil(t *testing.T) {
	router := NewRouter(RoutingConfig{}, "test")

	err := router.RegisterProvider(nil)
	assert.Error(t, err)
}

func TestRouter_Route_DefaultProvider(t *testing.T) {
	router := NewRouter(RoutingConfig{}, "default")

	defaultProvider := NewMockProvider("default")
	err := router.RegisterProvider(defaultProvider)
	require.NoError(t, err)

	provider, err := router.Route(context.Background(), "unknown-model")
	require.NoError(t, err)
	assert.Equal(t, "default", provider.Name())
}

func TestRouter_Route_WithMapping(t *testing.T) {
	cfg := RoutingConfig{
		ModelMapping: map[string]string{
			"llama*":  "ollama",
			"gpt*":    "openrouter",
		},
	}
	router := NewRouter(cfg, "ollama")

	ollamaProvider := NewMockProvider("ollama")
	openrouterProvider := NewMockProvider("openrouter")

	err := router.RegisterProvider(ollamaProvider)
	require.NoError(t, err)
	err = router.RegisterProvider(openrouterProvider)
	require.NoError(t, err)

	// Test llama model routes to ollama
	provider, err := router.Route(context.Background(), "llama2")
	require.NoError(t, err)
	assert.Equal(t, "ollama", provider.Name())

	// Test gpt model routes to openrouter
	provider, err = router.Route(context.Background(), "gpt-4")
	require.NoError(t, err)
	assert.Equal(t, "openrouter", provider.Name())
}

func TestRouter_Route_NoProviders(t *testing.T) {
	router := NewRouter(RoutingConfig{}, "")

	_, err := router.Route(context.Background(), "any-model")
	assert.Error(t, err)
}

func TestRouter_Complete(t *testing.T) {
	router := NewRouter(RoutingConfig{}, "mock")

	mockProvider := NewMockProvider("mock").
		WithResponse("Test response")
	_ = router.RegisterProvider(mockProvider)

	req := &CompletionRequest{
		Model: "any-model",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	}

	resp, err := router.Complete(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "Test response", resp.Choices[0].Message.Content)
}

func TestRouter_Stream(t *testing.T) {
	router := NewRouter(RoutingConfig{}, "mock")

	mockProvider := NewMockProvider("mock").
		WithResponse("Streaming test")
	_ = router.RegisterProvider(mockProvider)

	req := &CompletionRequest{
		Model: "any-model",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	}

	stream, err := router.Stream(context.Background(), req)
	require.NoError(t, err)
	defer stream.Close()

	// Read at least one chunk
	chunk, err := stream.Read()
	require.NoError(t, err)
	assert.NotNil(t, chunk)
}

func TestRouter_ListModels(t *testing.T) {
	router := NewRouter(RoutingConfig{}, "")

	provider1 := NewMockProvider("provider1").WithModels([]string{"model-a", "model-b"})
	provider2 := NewMockProvider("provider2").WithModels([]string{"model-c"})

	_ = router.RegisterProvider(provider1)
	_ = router.RegisterProvider(provider2)

	models, err := router.ListModels(context.Background())
	require.NoError(t, err)
	assert.Len(t, models, 3)
}

func TestRouter_Close(t *testing.T) {
	router := NewRouter(RoutingConfig{}, "")

	provider1 := NewMockProvider("provider1")
	provider2 := NewMockProvider("provider2")

	_ = router.RegisterProvider(provider1)
	_ = router.RegisterProvider(provider2)

	err := router.Close()
	assert.NoError(t, err)

	// Providers should be removed
	providers := router.ListProviders()
	assert.Empty(t, providers)
}

func TestRouter_GetProvider(t *testing.T) {
	router := NewRouter(RoutingConfig{}, "")

	mockProvider := NewMockProvider("test-provider")
	_ = router.RegisterProvider(mockProvider)

	provider, ok := router.GetProvider("test-provider")
	assert.True(t, ok)
	assert.Equal(t, "test-provider", provider.Name())

	_, ok = router.GetProvider("non-existent")
	assert.False(t, ok)
}

func TestMatchWildcard(t *testing.T) {
	tests := []struct {
		pattern string
		modelID string
		match   bool
	}{
		{"*", "anything", true},
		{"exact", "exact", true},
		{"exact", "not-exact", false},
		{"llama*", "llama2", true},
		{"llama*", "llama-7b", true},
		{"llama*", "mistral", false},
		{"*-7b", "llama-7b", true},
		{"*-7b", "mistral-7b", true},
		{"*-7b", "llama-13b", false},
		{"*llama*", "meta-llama-2", true},
		{"*llama*", "llama", true},
		{"*llama*", "mistral", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.modelID, func(t *testing.T) {
			result := matchWildcard(tt.pattern, tt.modelID)
			assert.Equal(t, tt.match, result)
		})
	}
}

func TestRouter_RouteWithOptions_ExplicitProvider(t *testing.T) {
	router := NewRouter(RoutingConfig{}, "default")

	defaultProvider := NewMockProvider("default")
	explicitProvider := NewMockProvider("explicit")
	_ = router.RegisterProvider(defaultProvider)
	_ = router.RegisterProvider(explicitProvider)

	// Route with explicit provider
	provider, err := router.RouteWithOptions(context.Background(), "any-model", "explicit")
	require.NoError(t, err)
	assert.Equal(t, "explicit", provider.Name())

	// Route without explicit provider uses default
	provider, err = router.RouteWithOptions(context.Background(), "any-model", "")
	require.NoError(t, err)
	assert.Equal(t, "default", provider.Name())
}

func TestRouter_RouteWithOptions_ExplicitProviderNotFound(t *testing.T) {
	router := NewRouter(RoutingConfig{}, "default")

	defaultProvider := NewMockProvider("default")
	_ = router.RegisterProvider(defaultProvider)

	_, err := router.RouteWithOptions(context.Background(), "any-model", "non-existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "non-existent")
}

func TestRouter_WithHealthChecker(t *testing.T) {
	cfg := HealthConfig{
		Enabled:            false,
		UnhealthyThreshold: 1,
		HealthyThreshold:   1,
	}
	hc := NewHealthChecker(cfg)

	router := NewRouter(RoutingConfig{}, "default", WithHealthChecker(hc))

	provider := NewMockProvider("test-provider")
	_ = router.RegisterProvider(provider)

	// Provider should be registered with health checker
	_, ok := hc.GetHealth("test-provider")
	assert.True(t, ok)
}

func TestRouter_SetHealthChecker(t *testing.T) {
	router := NewRouter(RoutingConfig{}, "default")

	provider1 := NewMockProvider("provider1")
	provider2 := NewMockProvider("provider2")
	_ = router.RegisterProvider(provider1)
	_ = router.RegisterProvider(provider2)

	cfg := HealthConfig{
		Enabled:            false,
		UnhealthyThreshold: 1,
		HealthyThreshold:   1,
	}
	hc := NewHealthChecker(cfg)
	router.SetHealthChecker(hc)

	// All existing providers should be registered
	_, ok := hc.GetHealth("provider1")
	assert.True(t, ok)
	_, ok = hc.GetHealth("provider2")
	assert.True(t, ok)
}

func TestRouter_Failover(t *testing.T) {
	cfg := RoutingConfig{
		ModelMapping: map[string]string{
			"test*": "primary",
		},
	}

	healthCfg := HealthConfig{
		Enabled:            false,
		UnhealthyThreshold: 1,
		HealthyThreshold:   1,
	}
	hc := NewHealthChecker(healthCfg)

	router := NewRouter(cfg, "primary", WithHealthChecker(hc), WithFailover(true))

	// Register primary and backup providers
	primaryProvider := NewMockProvider("primary").WithModels([]string{"test*"})
	backupProvider := NewMockProvider("backup").WithModels([]string{"test*"})
	_ = router.RegisterProvider(primaryProvider)
	_ = router.RegisterProvider(backupProvider)

	// Initially routes to primary
	provider, err := router.Route(context.Background(), "test-model")
	require.NoError(t, err)
	assert.Equal(t, "primary", provider.Name())

	// Mark primary as unhealthy
	primaryProvider.WithError(ErrProviderClosed)
	_ = hc.CheckNow(context.Background(), "primary")

	// Should now fail over to backup
	provider, err = router.Route(context.Background(), "test-model")
	require.NoError(t, err)
	assert.Equal(t, "backup", provider.Name())
}

func TestRouter_FailoverDisabled(t *testing.T) {
	cfg := RoutingConfig{
		ModelMapping: map[string]string{
			"test*": "primary",
		},
	}

	healthCfg := HealthConfig{
		Enabled:            false,
		UnhealthyThreshold: 1,
		HealthyThreshold:   1,
	}
	hc := NewHealthChecker(healthCfg)

	router := NewRouter(cfg, "primary", WithHealthChecker(hc), WithFailover(false))

	// Register primary and backup providers
	primaryProvider := NewMockProvider("primary").WithModels([]string{"test*"})
	backupProvider := NewMockProvider("backup").WithModels([]string{"test*"})
	_ = router.RegisterProvider(primaryProvider)
	_ = router.RegisterProvider(backupProvider)

	// Mark primary as unhealthy
	primaryProvider.WithError(ErrProviderClosed)
	_ = hc.CheckNow(context.Background(), "primary")

	// Should still route to primary when failover is disabled
	provider, err := router.Route(context.Background(), "test-model")
	require.NoError(t, err)
	assert.Equal(t, "primary", provider.Name())
}

func TestRouter_CloseStopsHealthChecker(t *testing.T) {
	healthCfg := HealthConfig{
		Enabled:            true,
		Interval:           10 * time.Millisecond,
		Timeout:            5 * time.Millisecond,
		UnhealthyThreshold: 1,
		HealthyThreshold:   1,
	}
	hc := NewHealthChecker(healthCfg)

	router := NewRouter(RoutingConfig{}, "default", WithHealthChecker(hc))

	provider := NewMockProvider("test-provider")
	_ = router.RegisterProvider(provider)

	// Start health checker
	hc.Start()

	// Close router should stop health checker
	err := router.Close()
	require.NoError(t, err)
}

func TestRouter_GetHealthChecker(t *testing.T) {
	healthCfg := HealthConfig{Enabled: false}
	hc := NewHealthChecker(healthCfg)

	router := NewRouter(RoutingConfig{}, "default", WithHealthChecker(hc))

	retrieved := router.GetHealthChecker()
	assert.Equal(t, hc, retrieved)
}

// Integration tests for failover scenarios

func TestRouter_Failover_MultipleProviders(t *testing.T) {
	cfg := RoutingConfig{
		ModelMapping: map[string]string{
			"test*": "primary",
		},
	}

	healthCfg := HealthConfig{
		Enabled:            false, // Disable background checks for predictable tests
		UnhealthyThreshold: 1,
		HealthyThreshold:   1,
	}
	hc := NewHealthChecker(healthCfg)

	router := NewRouter(cfg, "primary", WithHealthChecker(hc), WithFailover(true))

	// Register three providers
	primaryProvider := NewMockProvider("primary").WithModels([]string{"test*"})
	backupProvider := NewMockProvider("backup").WithModels([]string{"test*"})
	tertiaryProvider := NewMockProvider("tertiary").WithModels([]string{"test*"})
	_ = router.RegisterProvider(primaryProvider)
	_ = router.RegisterProvider(backupProvider)
	_ = router.RegisterProvider(tertiaryProvider)

	// Initially routes to primary
	provider, err := router.Route(context.Background(), "test-model")
	require.NoError(t, err)
	assert.Equal(t, "primary", provider.Name())

	// Mark primary as unhealthy
	primaryProvider.WithError(ErrProviderClosed)
	_ = hc.CheckNow(context.Background(), "primary")

	// Should fail over to one of the backups
	provider, err = router.Route(context.Background(), "test-model")
	require.NoError(t, err)
	assert.Contains(t, []string{"backup", "tertiary"}, provider.Name())
}

func TestRouter_Failover_AllProvidersUnhealthy(t *testing.T) {
	cfg := RoutingConfig{
		ModelMapping: map[string]string{
			"test*": "primary",
		},
	}

	healthCfg := HealthConfig{
		Enabled:            false,
		UnhealthyThreshold: 1,
		HealthyThreshold:   1,
	}
	hc := NewHealthChecker(healthCfg)

	router := NewRouter(cfg, "primary", WithHealthChecker(hc), WithFailover(true))

	// Register two providers
	primaryProvider := NewMockProvider("primary").WithModels([]string{"test*"})
	backupProvider := NewMockProvider("backup").WithModels([]string{"test*"})
	_ = router.RegisterProvider(primaryProvider)
	_ = router.RegisterProvider(backupProvider)

	// Mark both as unhealthy
	primaryProvider.WithError(ErrProviderClosed)
	backupProvider.WithError(ErrProviderClosed)
	_ = hc.CheckNow(context.Background(), "primary")
	_ = hc.CheckNow(context.Background(), "backup")

	// Should still return primary (graceful degradation)
	provider, err := router.Route(context.Background(), "test-model")
	require.NoError(t, err)
	// Returns a provider that supports the model even if unhealthy
	assert.NotNil(t, provider)
}

func TestRouter_Failover_Recovery(t *testing.T) {
	cfg := RoutingConfig{
		ModelMapping: map[string]string{
			"test*": "primary",
		},
	}

	healthCfg := HealthConfig{
		Enabled:            false,
		UnhealthyThreshold: 1,
		HealthyThreshold:   1,
	}
	hc := NewHealthChecker(healthCfg)

	router := NewRouter(cfg, "primary", WithHealthChecker(hc), WithFailover(true))

	// Register providers
	primaryProvider := NewMockProvider("primary").WithModels([]string{"test*"})
	backupProvider := NewMockProvider("backup").WithModels([]string{"test*"})
	_ = router.RegisterProvider(primaryProvider)
	_ = router.RegisterProvider(backupProvider)

	// Initially routes to primary
	provider, err := router.Route(context.Background(), "test-model")
	require.NoError(t, err)
	assert.Equal(t, "primary", provider.Name())

	// Mark primary as unhealthy
	primaryProvider.WithError(ErrProviderClosed)
	_ = hc.CheckNow(context.Background(), "primary")

	// Should fail over to backup
	provider, err = router.Route(context.Background(), "test-model")
	require.NoError(t, err)
	assert.Equal(t, "backup", provider.Name())

	// Recover primary
	primaryProvider.WithError(nil)
	_ = hc.CheckNow(context.Background(), "primary")

	// Should route back to primary after recovery
	provider, err = router.Route(context.Background(), "test-model")
	require.NoError(t, err)
	assert.Equal(t, "primary", provider.Name())
}

func TestRouter_Failover_ExplicitProviderUnhealthy(t *testing.T) {
	healthCfg := HealthConfig{
		Enabled:            false,
		UnhealthyThreshold: 1,
		HealthyThreshold:   1,
	}
	hc := NewHealthChecker(healthCfg)

	router := NewRouter(RoutingConfig{}, "default", WithHealthChecker(hc), WithFailover(true))

	// Register providers
	explicitProvider := NewMockProvider("explicit")
	defaultProvider := NewMockProvider("default")
	_ = router.RegisterProvider(explicitProvider)
	_ = router.RegisterProvider(defaultProvider)

	// Mark explicit as unhealthy
	explicitProvider.WithError(ErrProviderClosed)
	_ = hc.CheckNow(context.Background(), "explicit")

	// Explicit provider should return error when unhealthy
	_, err := router.RouteWithOptions(context.Background(), "any-model", "explicit")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrProviderUnhealthy)
}

func TestRouter_Failover_ModelNotSupportedByBackup(t *testing.T) {
	cfg := RoutingConfig{
		ModelMapping: map[string]string{
			"special*": "primary",
		},
	}

	healthCfg := HealthConfig{
		Enabled:            false,
		UnhealthyThreshold: 1,
		HealthyThreshold:   1,
	}
	hc := NewHealthChecker(healthCfg)

	router := NewRouter(cfg, "primary", WithHealthChecker(hc), WithFailover(true))

	// Primary supports special models, backup doesn't
	primaryProvider := NewMockProvider("primary").WithModels([]string{"special*"})
	backupProvider := NewMockProvider("backup").WithModels([]string{"general*"})
	_ = router.RegisterProvider(primaryProvider)
	_ = router.RegisterProvider(backupProvider)

	// Mark primary as unhealthy
	primaryProvider.WithError(ErrProviderClosed)
	_ = hc.CheckNow(context.Background(), "primary")

	// Should still return primary (no compatible backup available)
	provider, err := router.Route(context.Background(), "special-model")
	require.NoError(t, err)
	assert.Equal(t, "primary", provider.Name())
}

func TestRouter_Failover_WithUnknownHealthStatus(t *testing.T) {
	cfg := RoutingConfig{
		ModelMapping: map[string]string{
			"test*": "primary",
		},
	}

	healthCfg := HealthConfig{
		Enabled:            false,
		UnhealthyThreshold: 1,
		HealthyThreshold:   1,
	}
	hc := NewHealthChecker(healthCfg)

	router := NewRouter(cfg, "primary", WithHealthChecker(hc), WithFailover(true))

	// Register providers (health status is Unknown by default)
	primaryProvider := NewMockProvider("primary").WithModels([]string{"test*"})
	_ = router.RegisterProvider(primaryProvider)

	// Unknown status should be treated as available
	provider, err := router.Route(context.Background(), "test-model")
	require.NoError(t, err)
	assert.Equal(t, "primary", provider.Name())

	health, _ := hc.GetHealth("primary")
	assert.Equal(t, HealthStatusUnknown, health.Status)
}

func TestRouter_Complete_WithFailover(t *testing.T) {
	cfg := RoutingConfig{
		ModelMapping: map[string]string{
			"test*": "primary",
		},
	}

	healthCfg := HealthConfig{
		Enabled:            false,
		UnhealthyThreshold: 1,
		HealthyThreshold:   1,
	}
	hc := NewHealthChecker(healthCfg)

	router := NewRouter(cfg, "primary", WithHealthChecker(hc), WithFailover(true))

	// Primary and backup
	primaryProvider := NewMockProvider("primary").WithModels([]string{"test*"}).WithResponse("Primary response")
	backupProvider := NewMockProvider("backup").WithModels([]string{"test*"}).WithResponse("Backup response")
	_ = router.RegisterProvider(primaryProvider)
	_ = router.RegisterProvider(backupProvider)

	req := &CompletionRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "Hello"}},
	}

	// Should get primary response
	resp, err := router.Complete(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "Primary response", resp.Choices[0].Message.Content)

	// Mark primary as unhealthy
	primaryProvider.WithError(ErrProviderClosed)
	_ = hc.CheckNow(context.Background(), "primary")

	// Should get backup response
	resp, err = router.Complete(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "Backup response", resp.Choices[0].Message.Content)
}
