package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ar4mirez/maia/internal/config"
	"github.com/ar4mirez/maia/internal/storage"
	storebadger "github.com/ar4mirez/maia/internal/storage/badger"
	"github.com/ar4mirez/maia/internal/tenant"
)

func setupAdminTestServer(t *testing.T) (*Server, *tenant.BadgerManager, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dir, err := os.MkdirTemp("", "maia-admin-test-*")
	require.NoError(t, err)

	store, err := storebadger.NewWithPath(dir)
	require.NoError(t, err)

	// Open a separate BadgerDB for tenant management
	tenantDir, err := os.MkdirTemp("", "maia-tenant-test-*")
	require.NoError(t, err)

	tenantDB, err := badger.Open(badger.DefaultOptions(tenantDir).WithLogger(nil))
	require.NoError(t, err)

	tenantManager := tenant.NewBadgerManager(tenantDB)

	cfg := &config.Config{
		Server: config.ServerConfig{
			HTTPPort:       8080,
			RequestTimeout: 30000000000,
		},
		Log: config.LogConfig{
			Level: "info",
		},
	}

	logger := zap.NewNop()
	server := NewWithDeps(cfg, store, logger, &ServerDeps{
		TenantManager: tenantManager,
	})

	cleanup := func() {
		store.Close()
		tenantDB.Close()
		os.RemoveAll(dir)
		os.RemoveAll(tenantDir)
	}

	return server, tenantManager, cleanup
}

func TestAdminHandler_CreateTenant(t *testing.T) {
	server, _, cleanup := setupAdminTestServer(t)
	defer cleanup()

	tests := []struct {
		name       string
		body       CreateTenantRequest
		wantStatus int
	}{
		{
			name: "valid tenant",
			body: CreateTenantRequest{
				Name: "test-tenant",
				Plan: tenant.PlanStandard,
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "tenant with custom config",
			body: CreateTenantRequest{
				Name: "custom-tenant",
				Plan: tenant.PlanPremium,
				Config: tenant.Config{
					DefaultTokenBudget: 10000,
					MaxNamespaces:      100,
				},
			},
			wantStatus: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/admin/tenants", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)

			if tt.wantStatus == http.StatusCreated {
				var resp tenant.Tenant
				err := json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.Equal(t, tt.body.Name, resp.Name)
				assert.NotEmpty(t, resp.ID)
			}
		})
	}
}

func TestAdminHandler_CreateTenant_InvalidInput(t *testing.T) {
	server, _, cleanup := setupAdminTestServer(t)
	defer cleanup()

	// Missing name (required)
	body := []byte(`{"plan": "standard"}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/tenants", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminHandler_CreateTenant_Duplicate(t *testing.T) {
	server, _, cleanup := setupAdminTestServer(t)
	defer cleanup()

	body := []byte(`{"name": "duplicate-tenant"}`)

	// Create first tenant
	req := httptest.NewRequest(http.MethodPost, "/admin/tenants", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	// Try to create duplicate
	req = httptest.NewRequest(http.MethodPost, "/admin/tenants", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestAdminHandler_GetTenant(t *testing.T) {
	server, mgr, cleanup := setupAdminTestServer(t)
	defer cleanup()

	// Create a tenant first
	created, err := mgr.Create(context.Background(), &tenant.CreateTenantInput{
		Name: "get-test-tenant",
		Plan: tenant.PlanStandard,
	})
	require.NoError(t, err)

	// Get the tenant
	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/"+created.ID, nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp tenant.Tenant
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, created.ID, resp.ID)
	assert.Equal(t, created.Name, resp.Name)
}

func TestAdminHandler_GetTenant_NotFound(t *testing.T) {
	server, _, cleanup := setupAdminTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/non-existent-id", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminHandler_UpdateTenant(t *testing.T) {
	server, mgr, cleanup := setupAdminTestServer(t)
	defer cleanup()

	// Create a tenant first
	created, err := mgr.Create(context.Background(), &tenant.CreateTenantInput{
		Name: "update-test-tenant",
		Plan: tenant.PlanFree,
	})
	require.NoError(t, err)

	// Update the tenant
	newName := "updated-tenant-name"
	newPlan := tenant.PlanStandard
	body, _ := json.Marshal(UpdateTenantRequest{
		Name: &newName,
		Plan: &newPlan,
	})

	req := httptest.NewRequest(http.MethodPut, "/admin/tenants/"+created.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp tenant.Tenant
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, newName, resp.Name)
	assert.Equal(t, newPlan, resp.Plan)
}

func TestAdminHandler_UpdateTenant_NotFound(t *testing.T) {
	server, _, cleanup := setupAdminTestServer(t)
	defer cleanup()

	body := []byte(`{"name": "new-name"}`)
	req := httptest.NewRequest(http.MethodPut, "/admin/tenants/non-existent-id", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminHandler_DeleteTenant(t *testing.T) {
	server, mgr, cleanup := setupAdminTestServer(t)
	defer cleanup()

	// Create a tenant first
	created, err := mgr.Create(context.Background(), &tenant.CreateTenantInput{
		Name: "delete-test-tenant",
	})
	require.NoError(t, err)

	// Delete the tenant
	req := httptest.NewRequest(http.MethodDelete, "/admin/tenants/"+created.ID, nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify it's deleted
	_, err = mgr.Get(context.Background(), created.ID)
	assert.Error(t, err)
}

func TestAdminHandler_DeleteTenant_NotFound(t *testing.T) {
	server, _, cleanup := setupAdminTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodDelete, "/admin/tenants/non-existent-id", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminHandler_DeleteTenant_SystemTenant(t *testing.T) {
	server, mgr, cleanup := setupAdminTestServer(t)
	defer cleanup()

	// Ensure system tenant exists
	_, err := mgr.EnsureSystemTenant(context.Background())
	require.NoError(t, err)

	// Try to delete system tenant
	req := httptest.NewRequest(http.MethodDelete, "/admin/tenants/"+tenant.SystemTenantID, nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestAdminHandler_ListTenants(t *testing.T) {
	server, mgr, cleanup := setupAdminTestServer(t)
	defer cleanup()

	// Create some tenants
	for i := 0; i < 3; i++ {
		_, err := mgr.Create(context.Background(), &tenant.CreateTenantInput{
			Name: "list-tenant-" + string(rune('A'+i)),
			Plan: tenant.PlanStandard,
		})
		require.NoError(t, err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp ListResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 3, resp.Count)
}

func TestAdminHandler_ListTenants_WithFilters(t *testing.T) {
	server, mgr, cleanup := setupAdminTestServer(t)
	defer cleanup()

	// Create tenants with different plans
	_, err := mgr.Create(context.Background(), &tenant.CreateTenantInput{
		Name: "free-tenant",
		Plan: tenant.PlanFree,
	})
	require.NoError(t, err)

	_, err = mgr.Create(context.Background(), &tenant.CreateTenantInput{
		Name: "standard-tenant",
		Plan: tenant.PlanStandard,
	})
	require.NoError(t, err)

	// Filter by plan
	req := httptest.NewRequest(http.MethodGet, "/admin/tenants?plan=free", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp ListResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 1, resp.Count)
}

func TestAdminHandler_GetTenantUsage(t *testing.T) {
	server, mgr, cleanup := setupAdminTestServer(t)
	defer cleanup()

	// Create a tenant
	created, err := mgr.Create(context.Background(), &tenant.CreateTenantInput{
		Name: "usage-test-tenant",
	})
	require.NoError(t, err)

	// Get usage
	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/"+created.ID+"/usage", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp TenantUsageResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, created.ID, resp.Tenant.ID)
	assert.NotNil(t, resp.Usage)
}

func TestAdminHandler_GetTenantUsage_NotFound(t *testing.T) {
	server, _, cleanup := setupAdminTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/non-existent-id/usage", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminHandler_SuspendTenant(t *testing.T) {
	server, mgr, cleanup := setupAdminTestServer(t)
	defer cleanup()

	// Create a tenant
	created, err := mgr.Create(context.Background(), &tenant.CreateTenantInput{
		Name: "suspend-test-tenant",
	})
	require.NoError(t, err)
	assert.Equal(t, tenant.StatusActive, created.Status)

	// Suspend the tenant
	req := httptest.NewRequest(http.MethodPost, "/admin/tenants/"+created.ID+"/suspend", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp tenant.Tenant
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, tenant.StatusSuspended, resp.Status)
}

func TestAdminHandler_SuspendTenant_SystemTenant(t *testing.T) {
	server, mgr, cleanup := setupAdminTestServer(t)
	defer cleanup()

	// Ensure system tenant exists
	_, err := mgr.EnsureSystemTenant(context.Background())
	require.NoError(t, err)

	// Try to suspend system tenant
	req := httptest.NewRequest(http.MethodPost, "/admin/tenants/"+tenant.SystemTenantID+"/suspend", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestAdminHandler_SuspendTenant_NotFound(t *testing.T) {
	server, _, cleanup := setupAdminTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/admin/tenants/non-existent-id/suspend", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminHandler_ActivateTenant(t *testing.T) {
	server, mgr, cleanup := setupAdminTestServer(t)
	defer cleanup()

	// Create and suspend a tenant
	created, err := mgr.Create(context.Background(), &tenant.CreateTenantInput{
		Name: "activate-test-tenant",
	})
	require.NoError(t, err)

	err = mgr.Suspend(context.Background(), created.ID)
	require.NoError(t, err)

	// Verify it's suspended
	suspended, err := mgr.Get(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, tenant.StatusSuspended, suspended.Status)

	// Activate the tenant
	req := httptest.NewRequest(http.MethodPost, "/admin/tenants/"+created.ID+"/activate", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp tenant.Tenant
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, tenant.StatusActive, resp.Status)
}

func TestAdminHandler_ActivateTenant_NotFound(t *testing.T) {
	server, _, cleanup := setupAdminTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/admin/tenants/non-existent-id/activate", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// Test that admin routes are not registered when tenant manager is nil
func TestServer_AdminRoutes_NotRegisteredWithoutTenantManager(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dir, err := os.MkdirTemp("", "maia-no-tenant-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	store, err := storebadger.NewWithPath(dir)
	require.NoError(t, err)
	defer store.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{
			HTTPPort:       8080,
			RequestTimeout: 30000000000,
		},
		Log: config.LogConfig{
			Level: "info",
		},
	}

	logger := zap.NewNop()
	server := New(cfg, store, logger) // No tenant manager

	// Admin routes should return 404
	req := httptest.NewRequest(http.MethodGet, "/admin/tenants", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// Compile-time interface check
var _ storage.Store = (*storebadger.Store)(nil)
