package inference

import (
	"context"
	"sync"
	"time"
)

// HealthStatus represents the health status of a provider.
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusUnknown   HealthStatus = "unknown"
)

// HealthConfig holds configuration for health checking.
type HealthConfig struct {
	// Enabled controls whether health checking is active.
	Enabled bool `mapstructure:"enabled"`

	// Interval is the time between health checks.
	Interval time.Duration `mapstructure:"interval"`

	// Timeout is the timeout for each health check.
	Timeout time.Duration `mapstructure:"timeout"`

	// UnhealthyThreshold is the number of consecutive failures
	// before a provider is marked unhealthy.
	UnhealthyThreshold int `mapstructure:"unhealthy_threshold"`

	// HealthyThreshold is the number of consecutive successes
	// before an unhealthy provider is marked healthy.
	HealthyThreshold int `mapstructure:"healthy_threshold"`
}

// DefaultHealthConfig returns the default health configuration.
func DefaultHealthConfig() HealthConfig {
	return HealthConfig{
		Enabled:            true,
		Interval:           30 * time.Second,
		Timeout:            10 * time.Second,
		UnhealthyThreshold: 3,
		HealthyThreshold:   2,
	}
}

// ProviderHealth holds health information for a provider.
type ProviderHealth struct {
	Provider          Provider
	Status            HealthStatus
	LastCheck         time.Time
	LastError         error
	ConsecutiveErrors int
	ConsecutiveOK     int
}

// HealthChecker performs periodic health checks on providers.
type HealthChecker struct {
	config    HealthConfig
	providers map[string]*ProviderHealth
	mu        sync.RWMutex
	stopCh    chan struct{}
	stopped   bool
}

// NewHealthChecker creates a new health checker.
func NewHealthChecker(config HealthConfig) *HealthChecker {
	return &HealthChecker{
		config:    config,
		providers: make(map[string]*ProviderHealth),
		stopCh:    make(chan struct{}),
	}
}

// RegisterProvider adds a provider to health checking.
func (h *HealthChecker) RegisterProvider(p Provider) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.providers[p.Name()] = &ProviderHealth{
		Provider: p,
		Status:   HealthStatusUnknown,
	}
}

// UnregisterProvider removes a provider from health checking.
func (h *HealthChecker) UnregisterProvider(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.providers, name)
}

// Start begins periodic health checking.
func (h *HealthChecker) Start() {
	if !h.config.Enabled {
		return
	}

	go h.loop()
}

// Stop stops health checking.
func (h *HealthChecker) Stop() {
	h.mu.Lock()
	if h.stopped {
		h.mu.Unlock()
		return
	}
	h.stopped = true
	h.mu.Unlock()

	close(h.stopCh)
}

// GetStatus returns the health status of a provider.
func (h *HealthChecker) GetStatus(name string) HealthStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if ph, ok := h.providers[name]; ok {
		return ph.Status
	}
	return HealthStatusUnknown
}

// GetHealth returns the full health info for a provider.
func (h *HealthChecker) GetHealth(name string) (*ProviderHealth, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	ph, ok := h.providers[name]
	if !ok {
		return nil, false
	}

	// Return a copy to avoid race conditions
	return &ProviderHealth{
		Provider:          ph.Provider,
		Status:            ph.Status,
		LastCheck:         ph.LastCheck,
		LastError:         ph.LastError,
		ConsecutiveErrors: ph.ConsecutiveErrors,
		ConsecutiveOK:     ph.ConsecutiveOK,
	}, true
}

// GetAllHealth returns health info for all providers.
func (h *HealthChecker) GetAllHealth() map[string]*ProviderHealth {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make(map[string]*ProviderHealth, len(h.providers))
	for name, ph := range h.providers {
		result[name] = &ProviderHealth{
			Provider:          ph.Provider,
			Status:            ph.Status,
			LastCheck:         ph.LastCheck,
			LastError:         ph.LastError,
			ConsecutiveErrors: ph.ConsecutiveErrors,
			ConsecutiveOK:     ph.ConsecutiveOK,
		}
	}
	return result
}

// IsHealthy returns true if the provider is healthy.
func (h *HealthChecker) IsHealthy(name string) bool {
	return h.GetStatus(name) == HealthStatusHealthy
}

// CheckNow performs an immediate health check on a provider.
func (h *HealthChecker) CheckNow(ctx context.Context, name string) error {
	h.mu.RLock()
	ph, ok := h.providers[name]
	h.mu.RUnlock()

	if !ok {
		return ErrNoProviderFound
	}

	h.checkProvider(ctx, ph)
	return nil
}

// loop is the main health checking loop.
func (h *HealthChecker) loop() {
	ticker := time.NewTicker(h.config.Interval)
	defer ticker.Stop()

	// Do an initial check
	h.checkAll()

	for {
		select {
		case <-ticker.C:
			h.checkAll()
		case <-h.stopCh:
			return
		}
	}
}

// checkAll checks all registered providers.
func (h *HealthChecker) checkAll() {
	ctx, cancel := context.WithTimeout(context.Background(), h.config.Timeout)
	defer cancel()

	h.mu.RLock()
	providers := make([]*ProviderHealth, 0, len(h.providers))
	for _, ph := range h.providers {
		providers = append(providers, ph)
	}
	h.mu.RUnlock()

	// Check each provider concurrently
	var wg sync.WaitGroup
	for _, ph := range providers {
		wg.Add(1)
		go func(ph *ProviderHealth) {
			defer wg.Done()
			h.checkProvider(ctx, ph)
		}(ph)
	}
	wg.Wait()
}

// checkProvider checks a single provider's health.
func (h *HealthChecker) checkProvider(ctx context.Context, ph *ProviderHealth) {
	err := ph.Provider.Health(ctx)

	h.mu.Lock()
	defer h.mu.Unlock()

	ph.LastCheck = time.Now()
	ph.LastError = err

	if err != nil {
		ph.ConsecutiveErrors++
		ph.ConsecutiveOK = 0

		if ph.ConsecutiveErrors >= h.config.UnhealthyThreshold {
			ph.Status = HealthStatusUnhealthy
		}
	} else {
		ph.ConsecutiveOK++
		ph.ConsecutiveErrors = 0

		if ph.Status == HealthStatusUnknown ||
			(ph.Status == HealthStatusUnhealthy && ph.ConsecutiveOK >= h.config.HealthyThreshold) {
			ph.Status = HealthStatusHealthy
		} else if ph.Status != HealthStatusHealthy {
			ph.Status = HealthStatusHealthy
		}
	}
}
