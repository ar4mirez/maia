package inference

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHealthChecker(t *testing.T) {
	cfg := DefaultHealthConfig()
	hc := NewHealthChecker(cfg)

	assert.NotNil(t, hc)
	assert.Equal(t, cfg.Interval, hc.config.Interval)
	assert.Equal(t, cfg.Timeout, hc.config.Timeout)
}

func TestHealthChecker_RegisterProvider(t *testing.T) {
	hc := NewHealthChecker(DefaultHealthConfig())

	provider := NewMockProvider("test-provider")
	hc.RegisterProvider(provider)

	health, ok := hc.GetHealth("test-provider")
	require.True(t, ok)
	assert.Equal(t, HealthStatusUnknown, health.Status)
}

func TestHealthChecker_UnregisterProvider(t *testing.T) {
	hc := NewHealthChecker(DefaultHealthConfig())

	provider := NewMockProvider("test-provider")
	hc.RegisterProvider(provider)

	hc.UnregisterProvider("test-provider")

	_, ok := hc.GetHealth("test-provider")
	assert.False(t, ok)
}

func TestHealthChecker_GetStatus(t *testing.T) {
	hc := NewHealthChecker(DefaultHealthConfig())

	// Unknown provider
	status := hc.GetStatus("unknown")
	assert.Equal(t, HealthStatusUnknown, status)

	// Registered provider (initial state)
	provider := NewMockProvider("test-provider")
	hc.RegisterProvider(provider)

	status = hc.GetStatus("test-provider")
	assert.Equal(t, HealthStatusUnknown, status)
}

func TestHealthChecker_CheckNow(t *testing.T) {
	cfg := DefaultHealthConfig()
	cfg.Enabled = false // Don't start background loop
	hc := NewHealthChecker(cfg)

	provider := NewMockProvider("test-provider")
	hc.RegisterProvider(provider)

	// Check now
	err := hc.CheckNow(context.Background(), "test-provider")
	require.NoError(t, err)

	// Should be healthy now
	health, ok := hc.GetHealth("test-provider")
	require.True(t, ok)
	assert.Equal(t, HealthStatusHealthy, health.Status)
	assert.NotZero(t, health.LastCheck)
	assert.Nil(t, health.LastError)
}

func TestHealthChecker_CheckNow_UnhealthyProvider(t *testing.T) {
	cfg := DefaultHealthConfig()
	cfg.Enabled = false
	cfg.UnhealthyThreshold = 1 // Mark unhealthy after 1 failure
	hc := NewHealthChecker(cfg)

	provider := NewMockProvider("test-provider").
		WithError(errors.New("provider unavailable"))
	hc.RegisterProvider(provider)

	// Check now
	err := hc.CheckNow(context.Background(), "test-provider")
	require.NoError(t, err)

	// Should be unhealthy
	health, ok := hc.GetHealth("test-provider")
	require.True(t, ok)
	assert.Equal(t, HealthStatusUnhealthy, health.Status)
	assert.NotNil(t, health.LastError)
	assert.Equal(t, 1, health.ConsecutiveErrors)
}

func TestHealthChecker_CheckNow_UnknownProvider(t *testing.T) {
	hc := NewHealthChecker(DefaultHealthConfig())

	err := hc.CheckNow(context.Background(), "unknown")
	assert.ErrorIs(t, err, ErrNoProviderFound)
}

func TestHealthChecker_IsHealthy(t *testing.T) {
	cfg := DefaultHealthConfig()
	cfg.Enabled = false
	hc := NewHealthChecker(cfg)

	provider := NewMockProvider("test-provider")
	hc.RegisterProvider(provider)

	// Initial state: unknown (not healthy)
	assert.False(t, hc.IsHealthy("test-provider"))

	// After check: healthy
	_ = hc.CheckNow(context.Background(), "test-provider")
	assert.True(t, hc.IsHealthy("test-provider"))
}

func TestHealthChecker_GetAllHealth(t *testing.T) {
	cfg := DefaultHealthConfig()
	cfg.Enabled = false
	hc := NewHealthChecker(cfg)

	provider1 := NewMockProvider("provider1")
	provider2 := NewMockProvider("provider2")
	hc.RegisterProvider(provider1)
	hc.RegisterProvider(provider2)

	allHealth := hc.GetAllHealth()
	assert.Len(t, allHealth, 2)
	assert.Contains(t, allHealth, "provider1")
	assert.Contains(t, allHealth, "provider2")
}

func TestHealthChecker_ThresholdBehavior(t *testing.T) {
	cfg := HealthConfig{
		Enabled:            false,
		Interval:           time.Second,
		Timeout:            time.Second,
		UnhealthyThreshold: 2,
		HealthyThreshold:   2,
	}
	hc := NewHealthChecker(cfg)

	// Create a provider that can toggle between healthy and unhealthy
	provider := NewMockProvider("test-provider")
	hc.RegisterProvider(provider)

	// First successful check
	_ = hc.CheckNow(context.Background(), "test-provider")
	health, _ := hc.GetHealth("test-provider")
	assert.Equal(t, HealthStatusHealthy, health.Status)

	// Simulate failure
	provider.WithError(errors.New("fail"))
	_ = hc.CheckNow(context.Background(), "test-provider")
	health, _ = hc.GetHealth("test-provider")
	// Should still be healthy (threshold is 2)
	assert.Equal(t, 1, health.ConsecutiveErrors)

	// Second failure - now should be unhealthy
	_ = hc.CheckNow(context.Background(), "test-provider")
	health, _ = hc.GetHealth("test-provider")
	assert.Equal(t, HealthStatusUnhealthy, health.Status)
	assert.Equal(t, 2, health.ConsecutiveErrors)

	// Recovery - first success
	provider.WithError(nil)
	_ = hc.CheckNow(context.Background(), "test-provider")
	health, _ = hc.GetHealth("test-provider")
	assert.Equal(t, 1, health.ConsecutiveOK)

	// Second success - should be healthy again
	_ = hc.CheckNow(context.Background(), "test-provider")
	health, _ = hc.GetHealth("test-provider")
	assert.Equal(t, HealthStatusHealthy, health.Status)
}

func TestHealthChecker_StartStop(t *testing.T) {
	cfg := HealthConfig{
		Enabled:            true,
		Interval:           50 * time.Millisecond,
		Timeout:            10 * time.Millisecond,
		UnhealthyThreshold: 1,
		HealthyThreshold:   1,
	}
	hc := NewHealthChecker(cfg)

	provider := NewMockProvider("test-provider")
	hc.RegisterProvider(provider)

	// Start health checking
	hc.Start()

	// Wait for at least one check
	time.Sleep(100 * time.Millisecond)

	// Should have been checked
	health, ok := hc.GetHealth("test-provider")
	require.True(t, ok)
	assert.NotZero(t, health.LastCheck)

	// Stop health checking
	hc.Stop()
}

func TestHealthChecker_DisabledDoesNotStart(t *testing.T) {
	cfg := HealthConfig{
		Enabled:  false,
		Interval: 10 * time.Millisecond,
	}
	hc := NewHealthChecker(cfg)

	provider := NewMockProvider("test-provider")
	hc.RegisterProvider(provider)

	// Start should do nothing when disabled
	hc.Start()

	// Wait a bit
	time.Sleep(50 * time.Millisecond)

	// Should not have been checked
	health, _ := hc.GetHealth("test-provider")
	assert.Zero(t, health.LastCheck)
}

func TestDefaultHealthConfig(t *testing.T) {
	cfg := DefaultHealthConfig()

	assert.True(t, cfg.Enabled)
	assert.Equal(t, 30*time.Second, cfg.Interval)
	assert.Equal(t, 10*time.Second, cfg.Timeout)
	assert.Equal(t, 3, cfg.UnhealthyThreshold)
	assert.Equal(t, 2, cfg.HealthyThreshold)
}
