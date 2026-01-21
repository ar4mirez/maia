package tenant

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// ContextKey is the type for tenant context keys.
type ContextKey string

const (
	// TenantKey is the context key for the tenant.
	TenantKey ContextKey = "tenant"
	// TenantIDKey is the context key for the tenant ID.
	TenantIDKey ContextKey = "tenant_id"
	// APIKeyKey is the context key for the API key.
	APIKeyKey ContextKey = "api_key"
)

// HeaderTenantID is the header name for tenant ID.
const HeaderTenantID = "X-MAIA-Tenant-ID"

// MiddlewareConfig configures the tenant middleware.
type MiddlewareConfig struct {
	// Manager is the tenant manager.
	Manager Manager
	// APIKeyManager is the API key manager for looking up tenants by API key.
	// If nil, API key to tenant lookup is disabled.
	APIKeyManager APIKeyManager
	// Enabled controls whether tenant middleware is active.
	Enabled bool
	// DefaultTenantID is used when no tenant is specified (for backward compatibility).
	DefaultTenantID string
	// RequireTenant controls whether requests without a tenant should fail.
	RequireTenant bool
	// SkipPaths are paths that skip tenant validation (e.g., health checks).
	SkipPaths []string
	// EnableAPIKeyLookup controls whether to lookup tenant by API key.
	EnableAPIKeyLookup bool
}

// Middleware returns a Gin middleware for tenant identification and validation.
func Middleware(config MiddlewareConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip if middleware is disabled
		if !config.Enabled {
			c.Next()
			return
		}

		// Skip certain paths (health checks, metrics, etc.)
		for _, path := range config.SkipPaths {
			if strings.HasPrefix(c.Request.URL.Path, path) {
				c.Next()
				return
			}
		}

		// Try to get tenant from various sources in order of priority:
		// 1. X-MAIA-Tenant-ID header (explicit tenant ID)
		// 2. API key lookup (if enabled)
		// 3. Default tenant ID (fallback)
		var tenant *Tenant
		var tenantID string
		var apiKeyObj *APIKey
		var err error

		// 1. Try explicit tenant ID header first
		tenantID = c.GetHeader(HeaderTenantID)

		// 2. If API key lookup is enabled, get the API key object
		rawAPIKey := extractAPIKey(c)
		if config.EnableAPIKeyLookup && config.APIKeyManager != nil && rawAPIKey != "" {
			// Get the API key object for scope checking
			apiKeyObj, err = config.APIKeyManager.GetAPIKey(c.Request.Context(), rawAPIKey)
			if err == nil && apiKeyObj != nil {
				// If no explicit tenant ID, use the one from the API key
				if tenantID == "" {
					tenant, err = config.APIKeyManager.GetTenantByAPIKey(c.Request.Context(), rawAPIKey)
					if err == nil && tenant != nil {
						tenantID = tenant.ID
					}
				}
				// Update last used timestamp asynchronously (best-effort)
				go func(key string) {
					_ = config.APIKeyManager.UpdateAPIKeyLastUsed(c.Request.Context(), key)
				}(rawAPIKey)
			}
			// If API key lookup fails, continue - might use default tenant
		}

		// 3. Fall back to default tenant ID
		if tenantID == "" {
			tenantID = config.DefaultTenantID
		}

		// If no tenant and it's required, fail
		if tenantID == "" && config.RequireTenant {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "tenant identification required",
				"hint":  "provide X-MAIA-Tenant-ID header or use an API key associated with a tenant",
			})
			return
		}

		// If still no tenant, proceed without tenant context
		if tenantID == "" {
			c.Next()
			return
		}

		// Get tenant from manager if we don't have it yet
		if tenant == nil {
			tenant, err = config.Manager.Get(c.Request.Context(), tenantID)
			if err != nil {
				if _, ok := err.(*ErrNotFound); ok {
					c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
						"error": "invalid tenant",
					})
					return
				}
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": "failed to retrieve tenant",
				})
				return
			}
		}

		// Check tenant status
		if tenant.Status == StatusSuspended {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "tenant is suspended",
			})
			return
		}

		if tenant.Status == StatusPendingDeletion {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "tenant is pending deletion",
			})
			return
		}

		// Set tenant in context
		c.Set(string(TenantKey), tenant)
		c.Set(string(TenantIDKey), tenantID)

		// Set API key in context if available (for scope checking)
		if apiKeyObj != nil {
			c.Set(string(APIKeyKey), apiKeyObj)
		}

		c.Next()
	}
}

// extractAPIKey extracts the API key from the request.
func extractAPIKey(c *gin.Context) string {
	// Try X-API-Key header
	apiKey := c.GetHeader("X-API-Key")
	if apiKey != "" {
		return apiKey
	}

	// Try Authorization Bearer token
	auth := c.GetHeader("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}

	// Try query parameter
	apiKey = c.Query("api_key")
	if apiKey != "" {
		return apiKey
	}

	return ""
}

// GetTenant retrieves the tenant from Gin context.
func GetTenant(c *gin.Context) *Tenant {
	if t, ok := c.Get(string(TenantKey)); ok {
		if tenant, ok := t.(*Tenant); ok {
			return tenant
		}
	}
	return nil
}

// GetTenantID retrieves the tenant ID from Gin context.
func GetTenantID(c *gin.Context) string {
	if id, ok := c.Get(string(TenantIDKey)); ok {
		if tenantID, ok := id.(string); ok {
			return tenantID
		}
	}
	return ""
}

// GetAPIKeyFromContext retrieves the API key from Gin context.
func GetAPIKeyFromContext(c *gin.Context) *APIKey {
	if k, ok := c.Get(string(APIKeyKey)); ok {
		if apiKey, ok := k.(*APIKey); ok {
			return apiKey
		}
	}
	return nil
}

// QuotaMiddleware returns a middleware that checks tenant quotas before operations.
func QuotaMiddleware(manager Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenant := GetTenant(c)
		if tenant == nil {
			c.Next()
			return
		}

		// Only check quotas for write operations
		if c.Request.Method != http.MethodPost && c.Request.Method != http.MethodPut {
			c.Next()
			return
		}

		// Skip quota check for premium tenants with unlimited quotas
		if tenant.Quotas.MaxMemories == 0 {
			c.Next()
			return
		}

		// Get current usage
		usage, err := manager.GetUsage(c.Request.Context(), tenant.ID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "failed to check quota",
			})
			return
		}

		// Check memory quota (simple check - more sophisticated would check request body)
		if usage.MemoryCount >= tenant.Quotas.MaxMemories {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "memory quota exceeded",
				"limit":   tenant.Quotas.MaxMemories,
				"current": usage.MemoryCount,
			})
			return
		}

		// Check storage quota
		if tenant.Quotas.MaxStorageBytes > 0 && usage.StorageBytes >= tenant.Quotas.MaxStorageBytes {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "storage quota exceeded",
				"limit":   tenant.Quotas.MaxStorageBytes,
				"current": usage.StorageBytes,
			})
			return
		}

		c.Next()
	}
}

// ScopeMiddlewareConfig configures the scope enforcement middleware.
type ScopeMiddlewareConfig struct {
	// Enabled controls whether scope checking is active.
	Enabled bool
	// RouteScopes maps route patterns to required scopes.
	// Key format: "METHOD /path" (e.g., "GET /v1/memories")
	// Value is a list of scopes, any of which grants access.
	RouteScopes map[string][]string
	// SkipPaths are paths that skip scope checking.
	SkipPaths []string
}

// DefaultRouteScopes returns the default route-to-scope mappings for MAIA.
func DefaultRouteScopes() map[string][]string {
	return map[string][]string{
		// Memory operations
		"POST /v1/memories":   {ScopeWrite, ScopeAll},
		"GET /v1/memories":    {ScopeRead, ScopeSearch, ScopeAll},
		"PUT /v1/memories":    {ScopeWrite, ScopeAll},
		"DELETE /v1/memories": {ScopeDelete, ScopeAll},

		// Namespace operations
		"POST /v1/namespaces":   {ScopeWrite, ScopeAll},
		"GET /v1/namespaces":    {ScopeRead, ScopeAll},
		"PUT /v1/namespaces":    {ScopeWrite, ScopeAll},
		"DELETE /v1/namespaces": {ScopeDelete, ScopeAll},

		// Context assembly
		"POST /v1/context": {ScopeContext, ScopeRead, ScopeAll},

		// Search
		"POST /v1/search": {ScopeSearch, ScopeRead, ScopeAll},

		// Inference
		"POST /v1/chat/completions":   {ScopeInference, ScopeAll},
		"GET /v1/inference/health":    {ScopeInference, ScopeStats, ScopeAll},
		"POST /v1/inference/health":   {ScopeInference, ScopeAll},
		"GET /v1/inference/cache":     {ScopeInference, ScopeStats, ScopeAll},
		"POST /v1/inference/cache":    {ScopeInference, ScopeAll},

		// Stats
		"GET /v1/stats": {ScopeStats, ScopeRead, ScopeAll},

		// Admin operations
		"POST /admin/tenants":   {ScopeAdmin, ScopeAll},
		"GET /admin/tenants":    {ScopeAdmin, ScopeAll},
		"PUT /admin/tenants":    {ScopeAdmin, ScopeAll},
		"DELETE /admin/tenants": {ScopeAdmin, ScopeAll},
		"POST /admin/apikeys":   {ScopeAdmin, ScopeAll},
		"GET /admin/apikeys":    {ScopeAdmin, ScopeAll},
		"DELETE /admin/apikeys": {ScopeAdmin, ScopeAll},
	}
}

// ScopeMiddleware returns a middleware that checks API key scopes.
func ScopeMiddleware(config ScopeMiddlewareConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip if middleware is disabled
		if !config.Enabled {
			c.Next()
			return
		}

		// Skip certain paths
		for _, path := range config.SkipPaths {
			if strings.HasPrefix(c.Request.URL.Path, path) {
				c.Next()
				return
			}
		}

		// Get the API key from context
		apiKey := GetAPIKeyFromContext(c)

		// If no API key, allow (auth middleware handles authentication)
		// Scope checking only applies to API key authenticated requests
		if apiKey == nil {
			c.Next()
			return
		}

		// Find the required scopes for this route
		requiredScopes := findRequiredScopes(config.RouteScopes, c.Request.Method, c.Request.URL.Path)

		// If no specific scopes required for this route, allow
		if len(requiredScopes) == 0 {
			c.Next()
			return
		}

		// Check if API key has any of the required scopes
		if !apiKey.HasAnyScope(requiredScopes...) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":    "insufficient scope",
				"code":     "INSUFFICIENT_SCOPE",
				"required": requiredScopes,
				"provided": apiKey.Scopes,
			})
			return
		}

		c.Next()
	}
}

// findRequiredScopes finds the required scopes for a given method and path.
func findRequiredScopes(routeScopes map[string][]string, method, path string) []string {
	// Try exact match first
	key := method + " " + path
	if scopes, ok := routeScopes[key]; ok {
		return scopes
	}

	// Try prefix matching (for paths with IDs like /v1/memories/:id)
	for routeKey, scopes := range routeScopes {
		parts := strings.SplitN(routeKey, " ", 2)
		if len(parts) != 2 {
			continue
		}
		routeMethod, routePath := parts[0], parts[1]

		// Method must match
		if routeMethod != method {
			continue
		}

		// Check if path starts with the route path (handles /v1/memories and /v1/memories/:id)
		if strings.HasPrefix(path, routePath) {
			return scopes
		}
	}

	return nil
}

// RequireScope returns a middleware that requires a specific scope.
// This is useful for routes that need scope checking without the full config.
func RequireScope(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := GetAPIKeyFromContext(c)

		// If no API key, allow (auth middleware handles authentication)
		if apiKey == nil {
			c.Next()
			return
		}

		// Check if API key has the required scope
		if !apiKey.HasScope(scope) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":    "insufficient scope",
				"code":     "INSUFFICIENT_SCOPE",
				"required": scope,
				"provided": apiKey.Scopes,
			})
			return
		}

		c.Next()
	}
}
