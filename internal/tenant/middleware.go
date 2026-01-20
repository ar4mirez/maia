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
)

// HeaderTenantID is the header name for tenant ID.
const HeaderTenantID = "X-MAIA-Tenant-ID"

// MiddlewareConfig configures the tenant middleware.
type MiddlewareConfig struct {
	// Manager is the tenant manager.
	Manager Manager
	// Enabled controls whether tenant middleware is active.
	Enabled bool
	// DefaultTenantID is used when no tenant is specified (for backward compatibility).
	DefaultTenantID string
	// RequireTenant controls whether requests without a tenant should fail.
	RequireTenant bool
	// SkipPaths are paths that skip tenant validation (e.g., health checks).
	SkipPaths []string
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

		// Extract tenant ID from header or use default
		tenantID := c.GetHeader(HeaderTenantID)
		if tenantID == "" {
			tenantID = config.DefaultTenantID
		}

		// If no tenant and it's required, fail
		if tenantID == "" && config.RequireTenant {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "tenant identification required",
				"hint":  "provide X-MAIA-Tenant-ID header",
			})
			return
		}

		// If still no tenant, proceed without tenant context
		if tenantID == "" {
			c.Next()
			return
		}

		// Get tenant from manager
		tenant, err := config.Manager.Get(c.Request.Context(), tenantID)
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

		c.Next()
	}
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
