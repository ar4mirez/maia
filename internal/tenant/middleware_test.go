package tenant

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockManager implements Manager interface for testing.
type mockManager struct {
	tenants map[string]*Tenant
	usage   map[string]*Usage
}

func newMockManager() *mockManager {
	return &mockManager{
		tenants: make(map[string]*Tenant),
		usage:   make(map[string]*Usage),
	}
}

func (m *mockManager) Create(ctx context.Context, input *CreateTenantInput) (*Tenant, error) {
	return nil, nil
}

func (m *mockManager) Get(ctx context.Context, id string) (*Tenant, error) {
	if t, ok := m.tenants[id]; ok {
		return t, nil
	}
	return nil, &ErrNotFound{ID: id}
}

func (m *mockManager) GetByName(ctx context.Context, name string) (*Tenant, error) {
	return nil, nil
}

func (m *mockManager) Update(ctx context.Context, id string, input *UpdateTenantInput) (*Tenant, error) {
	return nil, nil
}

func (m *mockManager) Delete(ctx context.Context, id string) error {
	return nil
}

func (m *mockManager) List(ctx context.Context, opts *ListTenantsOptions) ([]*Tenant, error) {
	return nil, nil
}

func (m *mockManager) Suspend(ctx context.Context, id string) error {
	return nil
}

func (m *mockManager) Activate(ctx context.Context, id string) error {
	return nil
}

func (m *mockManager) GetUsage(ctx context.Context, id string) (*Usage, error) {
	if u, ok := m.usage[id]; ok {
		return u, nil
	}
	return nil, &ErrNotFound{ID: id}
}

func (m *mockManager) IncrementUsage(ctx context.Context, id string, memories, storage int64) error {
	return nil
}

func setupRouter(middleware gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware)
	r.GET("/test", func(c *gin.Context) {
		tenant := GetTenant(c)
		if tenant != nil {
			c.JSON(http.StatusOK, gin.H{"tenant_id": tenant.ID})
		} else {
			c.JSON(http.StatusOK, gin.H{"tenant_id": nil})
		}
	})
	r.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "created"})
	})
	return r
}

func TestMiddleware_Disabled(t *testing.T) {
	manager := newMockManager()
	middleware := Middleware(MiddlewareConfig{
		Manager: manager,
		Enabled: false,
	})

	router := setupRouter(middleware)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestMiddleware_SkipPaths(t *testing.T) {
	manager := newMockManager()
	middleware := Middleware(MiddlewareConfig{
		Manager:       manager,
		Enabled:       true,
		RequireTenant: true,
		SkipPaths:     []string{"/health", "/metrics"},
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware)
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestMiddleware_RequireTenant_Missing(t *testing.T) {
	manager := newMockManager()
	middleware := Middleware(MiddlewareConfig{
		Manager:       manager,
		Enabled:       true,
		RequireTenant: true,
	})

	router := setupRouter(middleware)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "tenant identification required")
}

func TestMiddleware_DefaultTenant(t *testing.T) {
	manager := newMockManager()
	manager.tenants["default-tenant"] = &Tenant{
		ID:     "default-tenant",
		Name:   "default",
		Status: StatusActive,
	}

	middleware := Middleware(MiddlewareConfig{
		Manager:         manager,
		Enabled:         true,
		DefaultTenantID: "default-tenant",
	})

	router := setupRouter(middleware)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "default-tenant")
}

func TestMiddleware_TenantFromHeader(t *testing.T) {
	manager := newMockManager()
	manager.tenants["my-tenant"] = &Tenant{
		ID:     "my-tenant",
		Name:   "My Tenant",
		Status: StatusActive,
	}

	middleware := Middleware(MiddlewareConfig{
		Manager: manager,
		Enabled: true,
	})

	router := setupRouter(middleware)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(HeaderTenantID, "my-tenant")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "my-tenant")
}

func TestMiddleware_InvalidTenant(t *testing.T) {
	manager := newMockManager()

	middleware := Middleware(MiddlewareConfig{
		Manager: manager,
		Enabled: true,
	})

	router := setupRouter(middleware)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(HeaderTenantID, "nonexistent")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "invalid tenant")
}

func TestMiddleware_SuspendedTenant(t *testing.T) {
	manager := newMockManager()
	manager.tenants["suspended-tenant"] = &Tenant{
		ID:     "suspended-tenant",
		Name:   "Suspended",
		Status: StatusSuspended,
	}

	middleware := Middleware(MiddlewareConfig{
		Manager: manager,
		Enabled: true,
	})

	router := setupRouter(middleware)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(HeaderTenantID, "suspended-tenant")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "suspended")
}

func TestMiddleware_PendingDeletionTenant(t *testing.T) {
	manager := newMockManager()
	manager.tenants["deleting-tenant"] = &Tenant{
		ID:     "deleting-tenant",
		Name:   "Deleting",
		Status: StatusPendingDeletion,
	}

	middleware := Middleware(MiddlewareConfig{
		Manager: manager,
		Enabled: true,
	})

	router := setupRouter(middleware)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(HeaderTenantID, "deleting-tenant")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "pending deletion")
}

func TestMiddleware_NoTenantNoRequirement(t *testing.T) {
	manager := newMockManager()

	middleware := Middleware(MiddlewareConfig{
		Manager:       manager,
		Enabled:       true,
		RequireTenant: false,
	})

	router := setupRouter(middleware)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// tenant_id should be null in response
	assert.Contains(t, w.Body.String(), "null")
}

func TestGetTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("tenant exists", func(t *testing.T) {
		tenant := &Tenant{ID: "test-id", Name: "test"}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(string(TenantKey), tenant)

		result := GetTenant(c)
		require.NotNil(t, result)
		assert.Equal(t, "test-id", result.ID)
	})

	t.Run("tenant not set", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		result := GetTenant(c)
		assert.Nil(t, result)
	})

	t.Run("wrong type", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(string(TenantKey), "not a tenant")

		result := GetTenant(c)
		assert.Nil(t, result)
	})
}

func TestGetTenantID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("tenant ID exists", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(string(TenantIDKey), "test-id")

		result := GetTenantID(c)
		assert.Equal(t, "test-id", result)
	})

	t.Run("tenant ID not set", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		result := GetTenantID(c)
		assert.Empty(t, result)
	})

	t.Run("wrong type", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(string(TenantIDKey), 123)

		result := GetTenantID(c)
		assert.Empty(t, result)
	})
}

func TestQuotaMiddleware_NoTenant(t *testing.T) {
	manager := newMockManager()
	middleware := QuotaMiddleware(manager)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware)
	router.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "created"})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestQuotaMiddleware_ReadOperation(t *testing.T) {
	manager := newMockManager()
	tenant := &Tenant{
		ID:     "test-tenant",
		Quotas: Quotas{MaxMemories: 100},
	}
	manager.tenants[tenant.ID] = tenant
	manager.usage[tenant.ID] = &Usage{MemoryCount: 200} // Over quota

	middleware := QuotaMiddleware(manager)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(TenantKey), tenant)
		c.Next()
	})
	router.Use(middleware)
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// GET should not check quota
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestQuotaMiddleware_UnlimitedQuota(t *testing.T) {
	manager := newMockManager()
	tenant := &Tenant{
		ID:     "premium-tenant",
		Quotas: Quotas{MaxMemories: 0}, // Unlimited
	}
	manager.tenants[tenant.ID] = tenant

	middleware := QuotaMiddleware(manager)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(TenantKey), tenant)
		c.Next()
	})
	router.Use(middleware)
	router.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "created"})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestQuotaMiddleware_MemoryQuotaExceeded(t *testing.T) {
	manager := newMockManager()
	tenant := &Tenant{
		ID:     "test-tenant",
		Quotas: Quotas{MaxMemories: 100},
	}
	manager.tenants[tenant.ID] = tenant
	manager.usage[tenant.ID] = &Usage{MemoryCount: 100} // At limit

	middleware := QuotaMiddleware(manager)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(TenantKey), tenant)
		c.Next()
	})
	router.Use(middleware)
	router.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "created"})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Contains(t, w.Body.String(), "memory quota exceeded")
}

func TestQuotaMiddleware_StorageQuotaExceeded(t *testing.T) {
	manager := newMockManager()
	tenant := &Tenant{
		ID: "test-tenant",
		Quotas: Quotas{
			MaxMemories:     1000,
			MaxStorageBytes: 1024 * 1024, // 1MB
		},
	}
	manager.tenants[tenant.ID] = tenant
	manager.usage[tenant.ID] = &Usage{
		MemoryCount:  50,
		StorageBytes: 1024 * 1024, // At limit
	}

	middleware := QuotaMiddleware(manager)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(TenantKey), tenant)
		c.Next()
	})
	router.Use(middleware)
	router.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "created"})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Contains(t, w.Body.String(), "storage quota exceeded")
}

func TestQuotaMiddleware_UsageNotFound(t *testing.T) {
	manager := newMockManager()
	tenant := &Tenant{
		ID:     "test-tenant",
		Quotas: Quotas{MaxMemories: 100},
	}
	// Don't set usage - will return error

	middleware := QuotaMiddleware(manager)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(TenantKey), tenant)
		c.Next()
	})
	router.Use(middleware)
	router.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "created"})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "failed to check quota")
}

// Tests for API Key Scope functionality

func TestAPIKey_HasScope(t *testing.T) {
	tests := []struct {
		name     string
		scopes   []string
		required string
		expected bool
	}{
		{
			name:     "empty scopes allows all",
			scopes:   nil,
			required: ScopeRead,
			expected: true,
		},
		{
			name:     "empty scopes slice allows all",
			scopes:   []string{},
			required: ScopeRead,
			expected: true,
		},
		{
			name:     "wildcard allows all",
			scopes:   []string{ScopeAll},
			required: ScopeAdmin,
			expected: true,
		},
		{
			name:     "direct match",
			scopes:   []string{ScopeRead, ScopeWrite},
			required: ScopeRead,
			expected: true,
		},
		{
			name:     "no match",
			scopes:   []string{ScopeRead},
			required: ScopeWrite,
			expected: false,
		},
		{
			name:     "admin scope does not grant read",
			scopes:   []string{ScopeAdmin},
			required: ScopeRead,
			expected: false,
		},
		{
			name:     "multiple scopes one matches",
			scopes:   []string{ScopeRead, ScopeWrite, ScopeDelete},
			required: ScopeDelete,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiKey := &APIKey{Scopes: tt.scopes}
			result := apiKey.HasScope(tt.required)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAPIKey_HasAnyScope(t *testing.T) {
	tests := []struct {
		name     string
		scopes   []string
		required []string
		expected bool
	}{
		{
			name:     "empty scopes allows all",
			scopes:   nil,
			required: []string{ScopeRead, ScopeWrite},
			expected: true,
		},
		{
			name:     "matches first required",
			scopes:   []string{ScopeRead},
			required: []string{ScopeRead, ScopeWrite},
			expected: true,
		},
		{
			name:     "matches second required",
			scopes:   []string{ScopeWrite},
			required: []string{ScopeRead, ScopeWrite},
			expected: true,
		},
		{
			name:     "matches none",
			scopes:   []string{ScopeDelete},
			required: []string{ScopeRead, ScopeWrite},
			expected: false,
		},
		{
			name:     "wildcard matches any",
			scopes:   []string{ScopeAll},
			required: []string{ScopeAdmin, ScopeInference},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiKey := &APIKey{Scopes: tt.scopes}
			result := apiKey.HasAnyScope(tt.required...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateScopes(t *testing.T) {
	tests := []struct {
		name    string
		scopes  []string
		wantErr bool
	}{
		{
			name:    "empty scopes valid",
			scopes:  []string{},
			wantErr: false,
		},
		{
			name:    "single valid scope",
			scopes:  []string{ScopeRead},
			wantErr: false,
		},
		{
			name:    "multiple valid scopes",
			scopes:  []string{ScopeRead, ScopeWrite, ScopeDelete},
			wantErr: false,
		},
		{
			name:    "wildcard valid",
			scopes:  []string{ScopeAll},
			wantErr: false,
		},
		{
			name:    "invalid scope",
			scopes:  []string{"invalid_scope"},
			wantErr: true,
		},
		{
			name:    "mixed valid and invalid",
			scopes:  []string{ScopeRead, "invalid"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateScopes(tt.scopes)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid scope")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGetAPIKeyFromContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("api key exists", func(t *testing.T) {
		apiKey := &APIKey{Key: "test-key", TenantID: "tenant-1"}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(string(APIKeyKey), apiKey)

		result := GetAPIKeyFromContext(c)
		require.NotNil(t, result)
		assert.Equal(t, "test-key", result.Key)
	})

	t.Run("api key not set", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		result := GetAPIKeyFromContext(c)
		assert.Nil(t, result)
	})

	t.Run("wrong type", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(string(APIKeyKey), "not an api key")

		result := GetAPIKeyFromContext(c)
		assert.Nil(t, result)
	})
}

func TestScopeMiddleware_Disabled(t *testing.T) {
	middleware := ScopeMiddleware(ScopeMiddlewareConfig{
		Enabled: false,
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Set an API key with limited scopes
	router.Use(func(c *gin.Context) {
		c.Set(string(APIKeyKey), &APIKey{Scopes: []string{ScopeRead}})
		c.Next()
	})
	router.Use(middleware)
	router.POST("/v1/memories", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "created"})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/memories", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should allow even though write scope is missing (middleware disabled)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestScopeMiddleware_SkipPaths(t *testing.T) {
	middleware := ScopeMiddleware(ScopeMiddlewareConfig{
		Enabled:     true,
		RouteScopes: DefaultRouteScopes(),
		SkipPaths:   []string{"/health", "/metrics"},
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Set an API key with no scopes (would fail scope check)
	router.Use(func(c *gin.Context) {
		c.Set(string(APIKeyKey), &APIKey{Scopes: []string{"none"}})
		c.Next()
	})
	router.Use(middleware)
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestScopeMiddleware_NoAPIKey(t *testing.T) {
	middleware := ScopeMiddleware(ScopeMiddlewareConfig{
		Enabled:     true,
		RouteScopes: DefaultRouteScopes(),
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware)
	router.POST("/v1/memories", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "created"})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/memories", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should allow - no API key means scope checking is skipped
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestScopeMiddleware_HasRequiredScope(t *testing.T) {
	middleware := ScopeMiddleware(ScopeMiddlewareConfig{
		Enabled:     true,
		RouteScopes: DefaultRouteScopes(),
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Set an API key with write scope
	router.Use(func(c *gin.Context) {
		c.Set(string(APIKeyKey), &APIKey{Scopes: []string{ScopeWrite}})
		c.Next()
	})
	router.Use(middleware)
	router.POST("/v1/memories", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "created"})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/memories", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestScopeMiddleware_InsufficientScope(t *testing.T) {
	middleware := ScopeMiddleware(ScopeMiddlewareConfig{
		Enabled:     true,
		RouteScopes: DefaultRouteScopes(),
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Set an API key with only read scope
	router.Use(func(c *gin.Context) {
		c.Set(string(APIKeyKey), &APIKey{Scopes: []string{ScopeRead}})
		c.Next()
	})
	router.Use(middleware)
	router.POST("/v1/memories", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "created"})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/memories", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "insufficient scope")
	assert.Contains(t, w.Body.String(), "INSUFFICIENT_SCOPE")
}

func TestScopeMiddleware_WildcardScope(t *testing.T) {
	middleware := ScopeMiddleware(ScopeMiddlewareConfig{
		Enabled:     true,
		RouteScopes: DefaultRouteScopes(),
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Set an API key with wildcard scope
	router.Use(func(c *gin.Context) {
		c.Set(string(APIKeyKey), &APIKey{Scopes: []string{ScopeAll}})
		c.Next()
	})
	router.Use(middleware)
	router.POST("/admin/tenants", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "created"})
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/tenants", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Wildcard should grant access to admin routes
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestScopeMiddleware_AdminScope(t *testing.T) {
	middleware := ScopeMiddleware(ScopeMiddlewareConfig{
		Enabled:     true,
		RouteScopes: DefaultRouteScopes(),
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Set an API key with admin scope
	router.Use(func(c *gin.Context) {
		c.Set(string(APIKeyKey), &APIKey{Scopes: []string{ScopeAdmin}})
		c.Next()
	})
	router.Use(middleware)
	router.POST("/admin/tenants", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "created"})
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/tenants", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestScopeMiddleware_UnmappedRoute(t *testing.T) {
	middleware := ScopeMiddleware(ScopeMiddlewareConfig{
		Enabled:     true,
		RouteScopes: DefaultRouteScopes(),
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Set an API key with limited scopes
	router.Use(func(c *gin.Context) {
		c.Set(string(APIKeyKey), &APIKey{Scopes: []string{ScopeRead}})
		c.Next()
	})
	router.Use(middleware)
	router.GET("/custom/unmapped", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/custom/unmapped", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Unmapped routes should be allowed
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestFindRequiredScopes(t *testing.T) {
	routeScopes := DefaultRouteScopes()

	tests := []struct {
		name     string
		method   string
		path     string
		expected []string
	}{
		{
			name:     "exact match POST memories",
			method:   "POST",
			path:     "/v1/memories",
			expected: []string{ScopeWrite, ScopeAll},
		},
		{
			name:     "exact match GET memories",
			method:   "GET",
			path:     "/v1/memories",
			expected: []string{ScopeRead, ScopeSearch, ScopeAll},
		},
		{
			name:     "prefix match GET memory by ID",
			method:   "GET",
			path:     "/v1/memories/abc123",
			expected: []string{ScopeRead, ScopeSearch, ScopeAll},
		},
		{
			name:     "admin route",
			method:   "POST",
			path:     "/admin/tenants",
			expected: []string{ScopeAdmin, ScopeAll},
		},
		{
			name:     "unmapped route",
			method:   "GET",
			path:     "/unknown/path",
			expected: nil,
		},
		{
			name:     "context route",
			method:   "POST",
			path:     "/v1/context",
			expected: []string{ScopeContext, ScopeRead, ScopeAll},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findRequiredScopes(routeScopes, tt.method, tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRequireScope_HasScope(t *testing.T) {
	middleware := RequireScope(ScopeWrite)

	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.Use(func(c *gin.Context) {
		c.Set(string(APIKeyKey), &APIKey{Scopes: []string{ScopeWrite}})
		c.Next()
	})
	router.Use(middleware)
	router.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireScope_InsufficientScope(t *testing.T) {
	middleware := RequireScope(ScopeWrite)

	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.Use(func(c *gin.Context) {
		c.Set(string(APIKeyKey), &APIKey{Scopes: []string{ScopeRead}})
		c.Next()
	})
	router.Use(middleware)
	router.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "insufficient scope")
}

func TestRequireScope_NoAPIKey(t *testing.T) {
	middleware := RequireScope(ScopeWrite)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware)
	router.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// No API key - should allow (scope checking only applies to API key authenticated requests)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestDefaultRouteScopes(t *testing.T) {
	scopes := DefaultRouteScopes()

	// Verify key routes are mapped
	assert.NotEmpty(t, scopes["POST /v1/memories"])
	assert.NotEmpty(t, scopes["GET /v1/memories"])
	assert.NotEmpty(t, scopes["POST /v1/context"])
	assert.NotEmpty(t, scopes["POST /admin/tenants"])
	assert.NotEmpty(t, scopes["GET /v1/stats"])

	// Verify all routes have ScopeAll as one option
	for route, routeScopes := range scopes {
		found := false
		for _, s := range routeScopes {
			if s == ScopeAll {
				found = true
				break
			}
		}
		assert.True(t, found, "route %s should include ScopeAll", route)
	}
}
