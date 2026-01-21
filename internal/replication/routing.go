package replication

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RoutingMiddleware routes requests based on tenant placement.
type RoutingMiddleware struct {
	manager       ReplicationManager
	cache         *PlacementCache
	regionToURL   map[string]string
	currentRegion string
	logger        *zap.Logger
}

// RoutingConfig configures the routing middleware.
type RoutingConfig struct {
	// Manager is the replication manager.
	Manager ReplicationManager

	// Cache is the placement cache (optional, created if nil).
	Cache *PlacementCache

	// RegionToURL maps region names to base URLs.
	RegionToURL map[string]string

	// Logger for routing operations.
	Logger *zap.Logger
}

// NewRoutingMiddleware creates a new routing middleware.
func NewRoutingMiddleware(cfg *RoutingConfig) *RoutingMiddleware {
	cache := cfg.Cache
	if cache == nil {
		cache = NewPlacementCache(&PlacementCacheConfig{
			Manager: cfg.Manager,
		})
	}

	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	return &RoutingMiddleware{
		manager:       cfg.Manager,
		cache:         cache,
		regionToURL:   cfg.RegionToURL,
		currentRegion: cfg.Manager.Region(),
		logger:        logger,
	}
}

// RouteRequest is the gin middleware that handles request routing.
func (r *RoutingMiddleware) RouteRequest() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip routing for certain paths
		if r.shouldSkipRouting(c.Request.URL.Path) {
			c.Next()
			return
		}

		// Extract tenant ID from request
		tenantID := r.extractTenantID(c)
		if tenantID == "" {
			// No tenant context, allow request to proceed
			c.Next()
			return
		}

		// Get placement for tenant
		placement, err := r.cache.Get(c.Request.Context(), tenantID)
		if err != nil {
			// If we can't get placement, allow request to proceed locally
			r.logger.Debug("failed to get tenant placement, proceeding locally",
				zap.String("tenant_id", tenantID),
				zap.Error(err))
			c.Next()
			return
		}

		// Check if this is a write request
		if r.isWriteRequest(c) {
			if placement.PrimaryRegion != r.currentRegion {
				// Redirect to primary region
				r.redirectToPrimary(c, placement)
				return
			}
		} else {
			// Read request - check if we should route to preferred region
			preferredRegion := c.GetHeader("X-MAIA-Preferred-Region")
			if preferredRegion != "" && preferredRegion != r.currentRegion {
				if r.canServeFromRegion(placement, preferredRegion) {
					r.redirectToRegion(c, preferredRegion)
					return
				}
			}
		}

		c.Next()
	}
}

// shouldSkipRouting returns true for paths that shouldn't be routed.
func (r *RoutingMiddleware) shouldSkipRouting(path string) bool {
	skipPrefixes := []string{
		"/health",
		"/ready",
		"/metrics",
		"/admin",
		"/v1/replication",
	}

	for _, prefix := range skipPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// extractTenantID gets the tenant ID from request context or headers.
func (r *RoutingMiddleware) extractTenantID(c *gin.Context) string {
	// First try context (set by tenant middleware)
	if tenantID, exists := c.Get("tenant_id"); exists {
		if id, ok := tenantID.(string); ok {
			return id
		}
	}

	// Then try header
	if tenantID := c.GetHeader("X-MAIA-Tenant-ID"); tenantID != "" {
		return tenantID
	}

	return ""
}

// isWriteRequest returns true for requests that modify data.
func (r *RoutingMiddleware) isWriteRequest(c *gin.Context) bool {
	switch c.Request.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// canServeFromRegion checks if the given region can serve the tenant's data.
func (r *RoutingMiddleware) canServeFromRegion(placement *TenantPlacement, region string) bool {
	if placement.PrimaryRegion == region {
		return true
	}

	for _, replica := range placement.Replicas {
		if replica == region {
			return true
		}
	}

	return false
}

// redirectToPrimary redirects the request to the primary region.
func (r *RoutingMiddleware) redirectToPrimary(c *gin.Context, placement *TenantPlacement) {
	baseURL, ok := r.regionToURL[placement.PrimaryRegion]
	if !ok {
		r.logger.Error("no URL configured for primary region",
			zap.String("region", placement.PrimaryRegion),
			zap.String("tenant_id", placement.TenantID))
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "REGION_UNAVAILABLE",
			"message": "Primary region is not available",
			"region":  placement.PrimaryRegion,
		})
		c.Abort()
		return
	}

	redirectURL := r.buildRedirectURL(baseURL, c.Request)
	r.logger.Debug("redirecting write to primary region",
		zap.String("tenant_id", placement.TenantID),
		zap.String("target_region", placement.PrimaryRegion),
		zap.String("redirect_url", redirectURL))

	c.Header("X-MAIA-Redirect-Reason", "write-to-primary")
	c.Header("X-MAIA-Primary-Region", placement.PrimaryRegion)
	c.Redirect(http.StatusTemporaryRedirect, redirectURL)
	c.Abort()
}

// redirectToRegion redirects the request to a specific region.
func (r *RoutingMiddleware) redirectToRegion(c *gin.Context, region string) {
	baseURL, ok := r.regionToURL[region]
	if !ok {
		// Region not available, serve locally
		r.logger.Debug("preferred region not available, serving locally",
			zap.String("preferred_region", region))
		c.Next()
		return
	}

	redirectURL := r.buildRedirectURL(baseURL, c.Request)
	r.logger.Debug("redirecting to preferred region",
		zap.String("target_region", region),
		zap.String("redirect_url", redirectURL))

	c.Header("X-MAIA-Redirect-Reason", "preferred-region")
	c.Redirect(http.StatusTemporaryRedirect, redirectURL)
	c.Abort()
}

// buildRedirectURL constructs the full redirect URL.
func (r *RoutingMiddleware) buildRedirectURL(baseURL string, req *http.Request) string {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return baseURL + req.URL.RequestURI()
	}

	// Keep the original path and query
	parsed.Path = req.URL.Path
	parsed.RawQuery = req.URL.RawQuery

	return parsed.String()
}

// InvalidatePlacement invalidates the cached placement for a tenant.
func (r *RoutingMiddleware) InvalidatePlacement(tenantID string) {
	r.cache.Invalidate(tenantID)
}

// InvalidateAllPlacements invalidates all cached placements.
func (r *RoutingMiddleware) InvalidateAllPlacements() {
	r.cache.InvalidateAll()
}

// CacheStats returns the placement cache statistics.
func (r *RoutingMiddleware) CacheStats() CacheStats {
	return r.cache.Stats()
}

// RouteInfo contains routing information for a tenant.
type RouteInfo struct {
	TenantID      string `json:"tenant_id"`
	PrimaryRegion string `json:"primary_region"`
	CurrentRegion string `json:"current_region"`
	IsLocal       bool   `json:"is_local"`
	CanRead       bool   `json:"can_read"`
	CanWrite      bool   `json:"can_write"`
}

// GetRouteInfo returns routing information for a tenant.
func (r *RoutingMiddleware) GetRouteInfo(tenantID string) (*RouteInfo, error) {
	placement, err := r.cache.Get(nil, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get placement: %w", err)
	}

	isLocal := placement.PrimaryRegion == r.currentRegion
	canRead := isLocal
	if !canRead {
		for _, replica := range placement.Replicas {
			if replica == r.currentRegion {
				canRead = true
				break
			}
		}
	}

	return &RouteInfo{
		TenantID:      tenantID,
		PrimaryRegion: placement.PrimaryRegion,
		CurrentRegion: r.currentRegion,
		IsLocal:       isLocal,
		CanRead:       canRead,
		CanWrite:      isLocal,
	}, nil
}
