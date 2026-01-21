package replication

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupRoutingTest(t *testing.T, currentRegion string) (*RoutingMiddleware, *mockManager) {
	manager := newMockManager()

	// Override region
	regionManager := &regionMockManager{
		mockManager: manager,
		region:      currentRegion,
	}

	cache := NewPlacementCache(&PlacementCacheConfig{
		Manager: regionManager,
	})

	routing := NewRoutingMiddleware(&RoutingConfig{
		Manager: regionManager,
		Cache:   cache,
		RegionToURL: map[string]string{
			"us-west-1":    "https://us-west.maia.example.com",
			"eu-central-1": "https://eu-central.maia.example.com",
			"ap-tokyo-1":   "https://ap-tokyo.maia.example.com",
		},
		Logger: zap.NewNop(),
	})

	return routing, manager
}

type regionMockManager struct {
	*mockManager
	region string
}

func (m *regionMockManager) Region() string {
	return m.region
}

func TestRoutingMiddleware_WriteToLocalPrimary(t *testing.T) {
	routing, manager := setupRoutingTest(t, "us-west-1")

	manager.setPlacement(&TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-west-1",
		Mode:          PlacementReplicated,
		Replicas:      []string{"eu-central-1"},
	})

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("tenant_id", "tenant-1")
		c.Next()
	})
	router.Use(routing.RouteRequest())
	router.POST("/v1/memories", func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{"status": "created"})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/memories", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestRoutingMiddleware_WriteRedirectToPrimary(t *testing.T) {
	routing, manager := setupRoutingTest(t, "eu-central-1")

	manager.setPlacement(&TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-west-1",
		Mode:          PlacementReplicated,
		Replicas:      []string{"eu-central-1"},
	})

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("tenant_id", "tenant-1")
		c.Next()
	})
	router.Use(routing.RouteRequest())
	router.POST("/v1/memories", func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{"status": "created"})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/memories", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTemporaryRedirect, w.Code)
	assert.Equal(t, "https://us-west.maia.example.com/v1/memories", w.Header().Get("Location"))
	assert.Equal(t, "write-to-primary", w.Header().Get("X-MAIA-Redirect-Reason"))
	assert.Equal(t, "us-west-1", w.Header().Get("X-MAIA-Primary-Region"))
}

func TestRoutingMiddleware_ReadFromReplica(t *testing.T) {
	routing, manager := setupRoutingTest(t, "eu-central-1")

	manager.setPlacement(&TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-west-1",
		Mode:          PlacementReplicated,
		Replicas:      []string{"eu-central-1"},
	})

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("tenant_id", "tenant-1")
		c.Next()
	})
	router.Use(routing.RouteRequest())
	router.GET("/v1/memories", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"memories": []string{}})
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/memories", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Read should be served locally from replica
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRoutingMiddleware_ReadWithPreferredRegion(t *testing.T) {
	routing, manager := setupRoutingTest(t, "us-west-1")

	manager.setPlacement(&TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-west-1",
		Mode:          PlacementReplicated,
		Replicas:      []string{"eu-central-1"},
	})

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("tenant_id", "tenant-1")
		c.Next()
	})
	router.Use(routing.RouteRequest())
	router.GET("/v1/memories", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"memories": []string{}})
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/memories", nil)
	req.Header.Set("X-MAIA-Preferred-Region", "eu-central-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should redirect to preferred region
	assert.Equal(t, http.StatusTemporaryRedirect, w.Code)
	assert.Equal(t, "https://eu-central.maia.example.com/v1/memories", w.Header().Get("Location"))
	assert.Equal(t, "preferred-region", w.Header().Get("X-MAIA-Redirect-Reason"))
}

func TestRoutingMiddleware_ReadWithUnavailablePreferredRegion(t *testing.T) {
	routing, manager := setupRoutingTest(t, "us-west-1")

	manager.setPlacement(&TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-west-1",
		Mode:          PlacementSingle, // No replicas
	})

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("tenant_id", "tenant-1")
		c.Next()
	})
	router.Use(routing.RouteRequest())
	router.GET("/v1/memories", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"memories": []string{}})
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/memories", nil)
	req.Header.Set("X-MAIA-Preferred-Region", "eu-central-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Preferred region not in placement, serve locally
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRoutingMiddleware_SkipPaths(t *testing.T) {
	routing, _ := setupRoutingTest(t, "us-west-1")

	testCases := []struct {
		path     string
		expected bool
	}{
		{"/health", true},
		{"/ready", true},
		{"/metrics", true},
		{"/admin/tenants", true},
		{"/v1/replication/entries", true},
		{"/v1/memories", false},
		{"/v1/namespaces", false},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			result := routing.shouldSkipRouting(tc.path)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestRoutingMiddleware_NoTenantContext(t *testing.T) {
	routing, _ := setupRoutingTest(t, "us-west-1")

	router := gin.New()
	// No tenant context middleware
	router.Use(routing.RouteRequest())
	router.POST("/v1/memories", func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{"status": "created"})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/memories", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should proceed without routing
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestRoutingMiddleware_TenantFromHeader(t *testing.T) {
	routing, manager := setupRoutingTest(t, "eu-central-1")

	manager.setPlacement(&TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-west-1",
		Mode:          PlacementReplicated,
	})

	router := gin.New()
	router.Use(routing.RouteRequest())
	router.POST("/v1/memories", func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{"status": "created"})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/memories", nil)
	req.Header.Set("X-MAIA-Tenant-ID", "tenant-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should redirect based on header
	assert.Equal(t, http.StatusTemporaryRedirect, w.Code)
}

func TestRoutingMiddleware_WriteMethodDetection(t *testing.T) {
	routing, _ := setupRoutingTest(t, "us-west-1")

	testCases := []struct {
		method   string
		isWrite  bool
	}{
		{http.MethodGet, false},
		{http.MethodHead, false},
		{http.MethodOptions, false},
		{http.MethodPost, true},
		{http.MethodPut, true},
		{http.MethodPatch, true},
		{http.MethodDelete, true},
	}

	for _, tc := range testCases {
		t.Run(tc.method, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(tc.method, "/v1/memories", nil)
			result := routing.isWriteRequest(c)
			assert.Equal(t, tc.isWrite, result)
		})
	}
}

func TestRoutingMiddleware_BuildRedirectURL(t *testing.T) {
	routing, _ := setupRoutingTest(t, "us-west-1")

	testCases := []struct {
		baseURL  string
		path     string
		query    string
		expected string
	}{
		{
			"https://example.com",
			"/v1/memories",
			"",
			"https://example.com/v1/memories",
		},
		{
			"https://example.com",
			"/v1/memories",
			"namespace=default",
			"https://example.com/v1/memories?namespace=default",
		},
		{
			"https://example.com:8080",
			"/v1/memories/123",
			"",
			"https://example.com:8080/v1/memories/123",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			url := tc.path
			if tc.query != "" {
				url += "?" + tc.query
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			result := routing.buildRedirectURL(tc.baseURL, req)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestRoutingMiddleware_InvalidatePlacement(t *testing.T) {
	routing, manager := setupRoutingTest(t, "us-west-1")

	manager.setPlacement(&TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-west-1",
		Mode:          PlacementSingle,
	})

	// Populate cache
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("tenant_id", "tenant-1")
		c.Next()
	})
	router.Use(routing.RouteRequest())
	router.GET("/v1/memories", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{})
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/memories", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	initialFetch := manager.getFetchCount()
	assert.Equal(t, 1, initialFetch)

	// Invalidate and fetch again
	routing.InvalidatePlacement("tenant-1")

	req = httptest.NewRequest(http.MethodGet, "/v1/memories", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 2, manager.getFetchCount())
}

func TestRoutingMiddleware_GetRouteInfo(t *testing.T) {
	routing, manager := setupRoutingTest(t, "us-west-1")

	manager.setPlacement(&TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-west-1",
		Mode:          PlacementReplicated,
		Replicas:      []string{"eu-central-1"},
	})

	info, err := routing.GetRouteInfo("tenant-1")
	require.NoError(t, err)

	assert.Equal(t, "tenant-1", info.TenantID)
	assert.Equal(t, "us-west-1", info.PrimaryRegion)
	assert.Equal(t, "us-west-1", info.CurrentRegion)
	assert.True(t, info.IsLocal)
	assert.True(t, info.CanRead)
	assert.True(t, info.CanWrite)
}

func TestRoutingMiddleware_GetRouteInfo_Replica(t *testing.T) {
	routing, manager := setupRoutingTest(t, "eu-central-1")

	manager.setPlacement(&TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-west-1",
		Mode:          PlacementReplicated,
		Replicas:      []string{"eu-central-1"},
	})

	info, err := routing.GetRouteInfo("tenant-1")
	require.NoError(t, err)

	assert.Equal(t, "tenant-1", info.TenantID)
	assert.Equal(t, "us-west-1", info.PrimaryRegion)
	assert.Equal(t, "eu-central-1", info.CurrentRegion)
	assert.False(t, info.IsLocal)
	assert.True(t, info.CanRead) // Can read from replica
	assert.False(t, info.CanWrite)
}

func TestRoutingMiddleware_CacheStats(t *testing.T) {
	routing, manager := setupRoutingTest(t, "us-west-1")

	manager.setPlacement(&TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-west-1",
		Mode:          PlacementSingle,
	})

	// Initial stats
	stats := routing.CacheStats()
	assert.Equal(t, int64(0), stats.Hits)
	assert.Equal(t, int64(0), stats.Misses)

	// Trigger cache population
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("tenant_id", "tenant-1")
		c.Next()
	})
	router.Use(routing.RouteRequest())
	router.GET("/v1/memories", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{})
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/memories", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	stats = routing.CacheStats()
	assert.Equal(t, int64(1), stats.Misses)
}

func TestRoutingMiddleware_RegionNotConfigured(t *testing.T) {
	routing, manager := setupRoutingTest(t, "eu-central-1")

	manager.setPlacement(&TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "ap-southeast-1", // Region not in config
		Mode:          PlacementSingle,
	})

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("tenant_id", "tenant-1")
		c.Next()
	})
	router.Use(routing.RouteRequest())
	router.POST("/v1/memories", func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/memories", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should return service unavailable since region is not configured
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}
