package inference

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// DefaultRouter routes completion requests to providers based on model patterns.
type DefaultRouter struct {
	providers       map[string]Provider
	modelMapping    map[string]string // pattern -> provider name
	defaultProvider string
	healthChecker   *HealthChecker
	failoverEnabled bool
	mu              sync.RWMutex
}

// RouterOption is a functional option for configuring the router.
type RouterOption func(*DefaultRouter)

// WithHealthChecker sets the health checker for the router.
func WithHealthChecker(hc *HealthChecker) RouterOption {
	return func(r *DefaultRouter) {
		r.healthChecker = hc
	}
}

// WithFailover enables or disables failover.
func WithFailover(enabled bool) RouterOption {
	return func(r *DefaultRouter) {
		r.failoverEnabled = enabled
	}
}

// NewRouter creates a new router with the given routing configuration.
func NewRouter(cfg RoutingConfig, defaultProvider string, opts ...RouterOption) *DefaultRouter {
	r := &DefaultRouter{
		providers:       make(map[string]Provider),
		modelMapping:    cfg.ModelMapping,
		defaultProvider: defaultProvider,
		failoverEnabled: true, // Enable failover by default
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// RegisterProvider adds a provider to the router.
func (r *DefaultRouter) RegisterProvider(p Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if p == nil {
		return fmt.Errorf("provider cannot be nil")
	}

	name := p.Name()
	if name == "" {
		return fmt.Errorf("provider name cannot be empty")
	}

	r.providers[name] = p

	// Register with health checker if available
	if r.healthChecker != nil {
		r.healthChecker.RegisterProvider(p)
	}

	return nil
}

// SetHealthChecker sets the health checker and registers all existing providers.
func (r *DefaultRouter) SetHealthChecker(hc *HealthChecker) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.healthChecker = hc
	for _, p := range r.providers {
		hc.RegisterProvider(p)
	}
}

// GetHealthChecker returns the health checker.
func (r *DefaultRouter) GetHealthChecker() *HealthChecker {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.healthChecker
}

// Route selects the appropriate provider for a model.
func (r *DefaultRouter) Route(ctx context.Context, modelID string) (Provider, error) {
	return r.RouteWithOptions(ctx, modelID, "")
}

// RouteWithOptions selects the appropriate provider with explicit provider override.
func (r *DefaultRouter) RouteWithOptions(ctx context.Context, modelID string, explicitProvider string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.providers) == 0 {
		return nil, fmt.Errorf("no providers registered")
	}

	// If explicit provider is specified, use it directly
	if explicitProvider != "" {
		if p, ok := r.providers[explicitProvider]; ok {
			// Check health if failover is enabled and health checker exists
			if r.failoverEnabled && r.healthChecker != nil {
				if !r.healthChecker.IsHealthy(explicitProvider) {
					return nil, fmt.Errorf("%w: %s", ErrProviderUnhealthy, explicitProvider)
				}
			}
			return p, nil
		}
		return nil, fmt.Errorf("%w: %s", ErrNoProviderFound, explicitProvider)
	}

	// First, check model mapping rules
	providerName := r.matchModelPattern(modelID)

	// If no mapping found, use default provider
	if providerName == "" {
		providerName = r.defaultProvider
	}

	// Look up provider with health check and failover
	if providerName != "" {
		if p, ok := r.providers[providerName]; ok {
			if r.isProviderAvailable(providerName) {
				return p, nil
			}
			// Provider is unhealthy, try to find fallback
			if r.failoverEnabled {
				if fallback := r.findHealthyFallback(modelID, providerName); fallback != nil {
					return fallback, nil
				}
			}
			// Return the provider anyway if no fallback found
			return p, nil
		}
	}

	// Fall back to any healthy provider that supports the model
	for _, p := range r.providers {
		if p.SupportsModel(modelID) && r.isProviderAvailable(p.Name()) {
			return p, nil
		}
	}

	// Fall back to any provider that supports the model (even unhealthy)
	for _, p := range r.providers {
		if p.SupportsModel(modelID) {
			return p, nil
		}
	}

	// Fall back to default provider regardless of model support
	if r.defaultProvider != "" {
		if p, ok := r.providers[r.defaultProvider]; ok {
			return p, nil
		}
	}

	// Return first available provider as last resort
	for _, p := range r.providers {
		return p, nil
	}

	return nil, ErrNoProviderFound
}

// isProviderAvailable checks if a provider is healthy or health checking is disabled.
func (r *DefaultRouter) isProviderAvailable(name string) bool {
	if r.healthChecker == nil || !r.failoverEnabled {
		return true
	}
	status := r.healthChecker.GetStatus(name)
	return status == HealthStatusHealthy || status == HealthStatusUnknown
}

// findHealthyFallback finds a healthy provider that can handle the model.
func (r *DefaultRouter) findHealthyFallback(modelID string, excludeProvider string) Provider {
	for name, p := range r.providers {
		if name == excludeProvider {
			continue
		}
		if p.SupportsModel(modelID) && r.isProviderAvailable(name) {
			return p
		}
	}
	return nil
}

// matchModelPattern finds the provider for a model based on patterns.
func (r *DefaultRouter) matchModelPattern(modelID string) string {
	// Try exact match first
	if provider, ok := r.modelMapping[modelID]; ok {
		return provider
	}

	// Try wildcard patterns
	for pattern, provider := range r.modelMapping {
		if matchWildcard(pattern, modelID) {
			return provider
		}
	}

	return ""
}

// matchWildcard checks if a model ID matches a pattern with optional wildcards.
// Supports:
// - Exact match: "llama2" matches "llama2"
// - Prefix wildcard: "llama*" matches "llama2", "llama3", "llama-7b"
// - Suffix wildcard: "*-7b" matches "llama-7b", "mistral-7b"
// - Contains wildcard: "*llama*" matches "meta-llama-2"
// - Universal wildcard: "*" matches everything
func matchWildcard(pattern, modelID string) bool {
	if pattern == "*" {
		return true
	}

	if pattern == modelID {
		return true
	}

	// Check for prefix wildcard
	if strings.HasSuffix(pattern, "*") && !strings.HasPrefix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(modelID, prefix)
	}

	// Check for suffix wildcard
	if strings.HasPrefix(pattern, "*") && !strings.HasSuffix(pattern, "*") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(modelID, suffix)
	}

	// Check for contains wildcard
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		middle := strings.Trim(pattern, "*")
		return strings.Contains(modelID, middle)
	}

	return false
}

// ListProviders returns all registered providers.
func (r *DefaultRouter) ListProviders() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providers := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		providers = append(providers, p)
	}
	return providers
}

// GetProvider returns a specific provider by name.
func (r *DefaultRouter) GetProvider(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[name]
	return p, ok
}

// Close closes all registered providers and stops health checking.
func (r *DefaultRouter) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Stop health checker first
	if r.healthChecker != nil {
		r.healthChecker.Stop()
	}

	var errs []string
	for name, p := range r.providers {
		if err := p.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
		}
	}

	r.providers = make(map[string]Provider)

	if len(errs) > 0 {
		return fmt.Errorf("errors closing providers: %s", strings.Join(errs, "; "))
	}
	return nil
}

// Complete routes and executes a completion request.
func (r *DefaultRouter) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	provider, err := r.Route(ctx, req.Model)
	if err != nil {
		return nil, fmt.Errorf("route: %w", err)
	}
	return provider.Complete(ctx, req)
}

// Stream routes and executes a streaming completion request.
func (r *DefaultRouter) Stream(ctx context.Context, req *CompletionRequest) (StreamReader, error) {
	provider, err := r.Route(ctx, req.Model)
	if err != nil {
		return nil, fmt.Errorf("route: %w", err)
	}
	return provider.Stream(ctx, req)
}

// ListModels returns all models from all registered providers.
func (r *DefaultRouter) ListModels(ctx context.Context) ([]Model, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var allModels []Model
	for _, p := range r.providers {
		models, err := p.ListModels(ctx)
		if err != nil {
			continue // Skip providers that fail
		}
		allModels = append(allModels, models...)
	}
	return allModels, nil
}
