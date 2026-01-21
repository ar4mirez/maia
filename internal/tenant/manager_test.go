package tenant

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestManager(t *testing.T) (*BadgerManager, func()) {
	t.Helper()

	dir, err := os.MkdirTemp("", "maia-tenant-test-*")
	require.NoError(t, err)

	opts := badger.DefaultOptions(dir)
	opts.Logger = nil
	db, err := badger.Open(opts)
	require.NoError(t, err)

	manager := NewBadgerManager(db)

	cleanup := func() {
		db.Close()
		os.RemoveAll(dir)
	}

	return manager, cleanup
}

func TestBadgerManager_Create(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	input := &CreateTenantInput{
		Name: "test-tenant",
		Plan: PlanStandard,
	}

	tenant, err := manager.Create(ctx, input)
	require.NoError(t, err)
	assert.NotEmpty(t, tenant.ID)
	assert.Equal(t, "test-tenant", tenant.Name)
	assert.Equal(t, PlanStandard, tenant.Plan)
	assert.Equal(t, StatusActive, tenant.Status)
	assert.False(t, tenant.CreatedAt.IsZero())
	assert.False(t, tenant.UpdatedAt.IsZero())

	// Verify default quotas were applied
	defaultQuotas := DefaultQuotas(PlanStandard)
	assert.Equal(t, defaultQuotas.MaxMemories, tenant.Quotas.MaxMemories)
}

func TestBadgerManager_Create_WithCustomQuotas(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	input := &CreateTenantInput{
		Name: "custom-tenant",
		Plan: PlanFree,
		Quotas: Quotas{
			MaxMemories:       5000,
			MaxStorageBytes:   500 * 1024 * 1024,
			RequestsPerMinute: 100,
			RequestsPerDay:    5000,
		},
	}

	tenant, err := manager.Create(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, int64(5000), tenant.Quotas.MaxMemories)
	assert.Equal(t, int64(500*1024*1024), tenant.Quotas.MaxStorageBytes)
}

func TestBadgerManager_Create_Validation(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name    string
		input   *CreateTenantInput
		wantErr bool
	}{
		{
			name:    "nil input",
			input:   nil,
			wantErr: true,
		},
		{
			name:    "empty name",
			input:   &CreateTenantInput{Name: ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := manager.Create(ctx, tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBadgerManager_Create_Duplicate(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	input := &CreateTenantInput{Name: "duplicate-tenant"}

	_, err := manager.Create(ctx, input)
	require.NoError(t, err)

	// Try to create again
	_, err = manager.Create(ctx, input)
	assert.Error(t, err)

	var alreadyExists *ErrAlreadyExists
	assert.ErrorAs(t, err, &alreadyExists)
}

func TestBadgerManager_Get(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	created, err := manager.Create(ctx, &CreateTenantInput{Name: "get-test"})
	require.NoError(t, err)

	retrieved, err := manager.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, retrieved.ID)
	assert.Equal(t, created.Name, retrieved.Name)
}

func TestBadgerManager_Get_NotFound(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	_, err := manager.Get(ctx, "nonexistent-id")
	assert.Error(t, err)

	var notFound *ErrNotFound
	assert.ErrorAs(t, err, &notFound)
}

func TestBadgerManager_GetByName(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	created, err := manager.Create(ctx, &CreateTenantInput{Name: "findme"})
	require.NoError(t, err)

	found, err := manager.GetByName(ctx, "findme")
	require.NoError(t, err)
	assert.Equal(t, created.ID, found.ID)
}

func TestBadgerManager_GetByName_NotFound(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	_, err := manager.GetByName(ctx, "nonexistent-name")
	assert.Error(t, err)

	var notFound *ErrNotFound
	assert.ErrorAs(t, err, &notFound)
}

func TestBadgerManager_Update(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	created, err := manager.Create(ctx, &CreateTenantInput{
		Name: "update-test",
		Plan: PlanFree,
	})
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	newPlan := PlanStandard
	newConfig := Config{
		DefaultTokenBudget: 5000,
		MaxNamespaces:      20,
	}

	updated, err := manager.Update(ctx, created.ID, &UpdateTenantInput{
		Plan:   &newPlan,
		Config: &newConfig,
	})
	require.NoError(t, err)
	assert.Equal(t, PlanStandard, updated.Plan)
	assert.Equal(t, 5000, updated.Config.DefaultTokenBudget)
	assert.True(t, updated.UpdatedAt.After(created.UpdatedAt))
}

func TestBadgerManager_Update_Name(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	created, err := manager.Create(ctx, &CreateTenantInput{Name: "old-name"})
	require.NoError(t, err)

	newName := "new-name"
	updated, err := manager.Update(ctx, created.ID, &UpdateTenantInput{Name: &newName})
	require.NoError(t, err)
	assert.Equal(t, "new-name", updated.Name)

	// Verify old name no longer works
	_, err = manager.GetByName(ctx, "old-name")
	assert.Error(t, err)

	// Verify new name works
	found, err := manager.GetByName(ctx, "new-name")
	require.NoError(t, err)
	assert.Equal(t, created.ID, found.ID)
}

func TestBadgerManager_Update_NameConflict(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	_, err := manager.Create(ctx, &CreateTenantInput{Name: "existing-name"})
	require.NoError(t, err)

	tenant2, err := manager.Create(ctx, &CreateTenantInput{Name: "other-name"})
	require.NoError(t, err)

	// Try to rename to existing name
	newName := "existing-name"
	_, err = manager.Update(ctx, tenant2.ID, &UpdateTenantInput{Name: &newName})
	assert.Error(t, err)

	var alreadyExists *ErrAlreadyExists
	assert.ErrorAs(t, err, &alreadyExists)
}

func TestBadgerManager_Update_NotFound(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	_, err := manager.Update(ctx, "nonexistent-id", &UpdateTenantInput{})
	assert.Error(t, err)

	var notFound *ErrNotFound
	assert.ErrorAs(t, err, &notFound)
}

func TestBadgerManager_Delete(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	created, err := manager.Create(ctx, &CreateTenantInput{Name: "to-delete"})
	require.NoError(t, err)

	err = manager.Delete(ctx, created.ID)
	require.NoError(t, err)

	// Verify it's gone
	_, err = manager.Get(ctx, created.ID)
	assert.Error(t, err)

	// Verify name index is gone
	_, err = manager.GetByName(ctx, "to-delete")
	assert.Error(t, err)

	// Verify usage is gone
	_, err = manager.GetUsage(ctx, created.ID)
	assert.Error(t, err)
}

func TestBadgerManager_Delete_NotFound(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	err := manager.Delete(ctx, "nonexistent-id")
	assert.Error(t, err)

	var notFound *ErrNotFound
	assert.ErrorAs(t, err, &notFound)
}

func TestBadgerManager_List(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	// Create several tenants
	for i := 0; i < 5; i++ {
		_, err := manager.Create(ctx, &CreateTenantInput{
			Name: "tenant-" + string(rune('A'+i)),
		})
		require.NoError(t, err)
	}

	tenants, err := manager.List(ctx, &ListTenantsOptions{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, tenants, 5)
}

func TestBadgerManager_List_Pagination(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	// Create 10 tenants
	for i := 0; i < 10; i++ {
		_, err := manager.Create(ctx, &CreateTenantInput{
			Name: "paginate-" + string(rune('A'+i)),
		})
		require.NoError(t, err)
	}

	page1, err := manager.List(ctx, &ListTenantsOptions{Limit: 5, Offset: 0})
	require.NoError(t, err)
	assert.Len(t, page1, 5)

	page2, err := manager.List(ctx, &ListTenantsOptions{Limit: 5, Offset: 5})
	require.NoError(t, err)
	assert.Len(t, page2, 5)

	// Verify no overlap
	for _, t1 := range page1 {
		for _, t2 := range page2 {
			assert.NotEqual(t, t1.ID, t2.ID)
		}
	}
}

func TestBadgerManager_List_FilterByStatus(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	// Create active tenant
	active, err := manager.Create(ctx, &CreateTenantInput{Name: "active-tenant"})
	require.NoError(t, err)

	// Create and suspend another tenant
	suspended, err := manager.Create(ctx, &CreateTenantInput{Name: "suspended-tenant"})
	require.NoError(t, err)
	err = manager.Suspend(ctx, suspended.ID)
	require.NoError(t, err)

	// List only active
	activeTenants, err := manager.List(ctx, &ListTenantsOptions{Status: StatusActive})
	require.NoError(t, err)
	assert.Len(t, activeTenants, 1)
	assert.Equal(t, active.ID, activeTenants[0].ID)

	// List only suspended
	suspendedTenants, err := manager.List(ctx, &ListTenantsOptions{Status: StatusSuspended})
	require.NoError(t, err)
	assert.Len(t, suspendedTenants, 1)
	assert.Equal(t, suspended.ID, suspendedTenants[0].ID)
}

func TestBadgerManager_List_FilterByPlan(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	_, err := manager.Create(ctx, &CreateTenantInput{Name: "free-tenant", Plan: PlanFree})
	require.NoError(t, err)

	_, err = manager.Create(ctx, &CreateTenantInput{Name: "standard-tenant", Plan: PlanStandard})
	require.NoError(t, err)

	freeTenants, err := manager.List(ctx, &ListTenantsOptions{Plan: PlanFree})
	require.NoError(t, err)
	assert.Len(t, freeTenants, 1)
	assert.Equal(t, PlanFree, freeTenants[0].Plan)
}

func TestBadgerManager_List_NilOptions(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	_, err := manager.Create(ctx, &CreateTenantInput{Name: "test-tenant"})
	require.NoError(t, err)

	tenants, err := manager.List(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, tenants, 1)
}

func TestBadgerManager_Suspend(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	created, err := manager.Create(ctx, &CreateTenantInput{Name: "suspend-test"})
	require.NoError(t, err)
	assert.Equal(t, StatusActive, created.Status)

	err = manager.Suspend(ctx, created.ID)
	require.NoError(t, err)

	suspended, err := manager.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusSuspended, suspended.Status)
}

func TestBadgerManager_Activate(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	created, err := manager.Create(ctx, &CreateTenantInput{Name: "activate-test"})
	require.NoError(t, err)

	// Suspend first
	err = manager.Suspend(ctx, created.ID)
	require.NoError(t, err)

	// Then activate
	err = manager.Activate(ctx, created.ID)
	require.NoError(t, err)

	activated, err := manager.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusActive, activated.Status)
}

func TestBadgerManager_GetUsage(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	created, err := manager.Create(ctx, &CreateTenantInput{Name: "usage-test"})
	require.NoError(t, err)

	usage, err := manager.GetUsage(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, usage.TenantID)
	assert.Equal(t, int64(0), usage.MemoryCount)
	assert.Equal(t, int64(0), usage.StorageBytes)
}

func TestBadgerManager_GetUsage_NotFound(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	_, err := manager.GetUsage(ctx, "nonexistent-id")
	assert.Error(t, err)

	var notFound *ErrNotFound
	assert.ErrorAs(t, err, &notFound)
}

func TestBadgerManager_IncrementUsage(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	created, err := manager.Create(ctx, &CreateTenantInput{Name: "increment-test"})
	require.NoError(t, err)

	// Increment usage
	err = manager.IncrementUsage(ctx, created.ID, 10, 1024)
	require.NoError(t, err)

	usage, err := manager.GetUsage(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(10), usage.MemoryCount)
	assert.Equal(t, int64(1024), usage.StorageBytes)

	// Increment again
	err = manager.IncrementUsage(ctx, created.ID, 5, 512)
	require.NoError(t, err)

	usage, err = manager.GetUsage(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(15), usage.MemoryCount)
	assert.Equal(t, int64(1536), usage.StorageBytes)
}

func TestBadgerManager_IncrementUsage_NotFound(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	err := manager.IncrementUsage(ctx, "nonexistent-id", 10, 1024)
	assert.Error(t, err)

	var notFound *ErrNotFound
	assert.ErrorAs(t, err, &notFound)
}

func TestBadgerManager_EnsureSystemTenant(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	// First call should create it
	tenant, err := manager.EnsureSystemTenant(ctx)
	require.NoError(t, err)
	assert.Equal(t, SystemTenantID, tenant.ID)
	assert.Equal(t, SystemTenantName, tenant.Name)
	assert.Equal(t, PlanPremium, tenant.Plan)
	assert.Equal(t, StatusActive, tenant.Status)

	// Second call should return existing
	tenant2, err := manager.EnsureSystemTenant(ctx)
	require.NoError(t, err)
	assert.Equal(t, tenant.ID, tenant2.ID)
}

func TestDefaultQuotas(t *testing.T) {
	tests := []struct {
		plan     Plan
		expected Quotas
	}{
		{
			plan: PlanFree,
			expected: Quotas{
				MaxMemories:       1000,
				MaxStorageBytes:   100 * 1024 * 1024,
				RequestsPerMinute: 60,
				RequestsPerDay:    1000,
			},
		},
		{
			plan: PlanStandard,
			expected: Quotas{
				MaxMemories:       100000,
				MaxStorageBytes:   10 * 1024 * 1024 * 1024,
				RequestsPerMinute: 600,
				RequestsPerDay:    100000,
			},
		},
		{
			plan: PlanPremium,
			expected: Quotas{
				MaxMemories:       0,
				MaxStorageBytes:   0,
				RequestsPerMinute: 6000,
				RequestsPerDay:    0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.plan), func(t *testing.T) {
			quotas := DefaultQuotas(tt.plan)
			assert.Equal(t, tt.expected, quotas)
		})
	}
}

func TestDefaultQuotas_UnknownPlan(t *testing.T) {
	quotas := DefaultQuotas("unknown")
	freeQuotas := DefaultQuotas(PlanFree)
	assert.Equal(t, freeQuotas, quotas)
}

func TestDefaultConfig(t *testing.T) {
	freeConfig := DefaultConfig(PlanFree)
	assert.Equal(t, "all-MiniLM-L6-v2", freeConfig.EmbeddingModel)
	assert.Equal(t, 2000, freeConfig.DefaultTokenBudget)
	assert.Equal(t, 5, freeConfig.MaxNamespaces)
	assert.False(t, freeConfig.DedicatedStorage)

	premiumConfig := DefaultConfig(PlanPremium)
	assert.True(t, premiumConfig.DedicatedStorage)
	assert.Equal(t, 0, premiumConfig.MaxNamespaces) // Unlimited
}

func TestDefaultConfig_UnknownPlan(t *testing.T) {
	config := DefaultConfig("unknown")
	freeConfig := DefaultConfig(PlanFree)
	assert.Equal(t, freeConfig, config)
}

// API Key Tests

func TestBadgerManager_CreateAPIKey(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	// Create a tenant first
	tenant, err := manager.Create(ctx, &CreateTenantInput{Name: "apikey-test"})
	require.NoError(t, err)

	input := &CreateAPIKeyInput{
		TenantID: tenant.ID,
		Name:     "test-key",
		Scopes:   []string{"read", "write"},
	}

	apiKey, rawKey, err := manager.CreateAPIKey(ctx, input)
	require.NoError(t, err)
	assert.NotEmpty(t, apiKey.Key)
	assert.NotEmpty(t, rawKey)
	assert.True(t, len(rawKey) > 10) // maia_ prefix + hex
	assert.Equal(t, tenant.ID, apiKey.TenantID)
	assert.Equal(t, "test-key", apiKey.Name)
	assert.Equal(t, []string{"read", "write"}, apiKey.Scopes)
	assert.False(t, apiKey.CreatedAt.IsZero())
}

func TestBadgerManager_CreateAPIKey_WithExpiration(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	tenant, err := manager.Create(ctx, &CreateTenantInput{Name: "expiry-test"})
	require.NoError(t, err)

	expiresAt := time.Now().Add(24 * time.Hour)
	input := &CreateAPIKeyInput{
		TenantID:  tenant.ID,
		Name:      "expiring-key",
		ExpiresAt: expiresAt,
	}

	apiKey, _, err := manager.CreateAPIKey(ctx, input)
	require.NoError(t, err)
	assert.False(t, apiKey.ExpiresAt.IsZero())
	assert.False(t, apiKey.IsExpired())
}

func TestBadgerManager_CreateAPIKey_Validation(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	tenant, err := manager.Create(ctx, &CreateTenantInput{Name: "validation-test"})
	require.NoError(t, err)

	tests := []struct {
		name    string
		input   *CreateAPIKeyInput
		wantErr bool
	}{
		{
			name:    "nil input",
			input:   nil,
			wantErr: true,
		},
		{
			name:    "empty tenant_id",
			input:   &CreateAPIKeyInput{TenantID: "", Name: "test"},
			wantErr: true,
		},
		{
			name:    "empty name",
			input:   &CreateAPIKeyInput{TenantID: tenant.ID, Name: ""},
			wantErr: true,
		},
		{
			name:    "nonexistent tenant",
			input:   &CreateAPIKeyInput{TenantID: "nonexistent", Name: "test"},
			wantErr: true,
		},
		{
			name:    "invalid scope",
			input:   &CreateAPIKeyInput{TenantID: tenant.ID, Name: "test", Scopes: []string{"invalid_scope"}},
			wantErr: true,
		},
		{
			name:    "valid scopes",
			input:   &CreateAPIKeyInput{TenantID: tenant.ID, Name: "test-valid", Scopes: []string{"read", "write"}},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := manager.CreateAPIKey(ctx, tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBadgerManager_GetAPIKey(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	tenant, err := manager.Create(ctx, &CreateTenantInput{Name: "getkey-test"})
	require.NoError(t, err)

	created, rawKey, err := manager.CreateAPIKey(ctx, &CreateAPIKeyInput{
		TenantID: tenant.ID,
		Name:     "retrieve-key",
	})
	require.NoError(t, err)

	// Retrieve by raw key
	retrieved, err := manager.GetAPIKey(ctx, rawKey)
	require.NoError(t, err)
	assert.Equal(t, created.Key, retrieved.Key)
	assert.Equal(t, created.TenantID, retrieved.TenantID)
	assert.Equal(t, created.Name, retrieved.Name)
}

func TestBadgerManager_GetAPIKey_NotFound(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	_, err := manager.GetAPIKey(ctx, "nonexistent-key")
	assert.Error(t, err)

	var notFound *ErrAPIKeyNotFound
	assert.ErrorAs(t, err, &notFound)
}

func TestBadgerManager_GetTenantByAPIKey(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	tenant, err := manager.Create(ctx, &CreateTenantInput{Name: "tenant-by-key"})
	require.NoError(t, err)

	_, rawKey, err := manager.CreateAPIKey(ctx, &CreateAPIKeyInput{
		TenantID: tenant.ID,
		Name:     "lookup-key",
	})
	require.NoError(t, err)

	// Get tenant by API key
	retrieved, err := manager.GetTenantByAPIKey(ctx, rawKey)
	require.NoError(t, err)
	assert.Equal(t, tenant.ID, retrieved.ID)
	assert.Equal(t, tenant.Name, retrieved.Name)
}

func TestBadgerManager_GetTenantByAPIKey_Expired(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	tenant, err := manager.Create(ctx, &CreateTenantInput{Name: "expired-key-tenant"})
	require.NoError(t, err)

	// Create an already-expired key
	expiresAt := time.Now().Add(-1 * time.Hour)
	_, rawKey, err := manager.CreateAPIKey(ctx, &CreateAPIKeyInput{
		TenantID:  tenant.ID,
		Name:      "expired-key",
		ExpiresAt: expiresAt,
	})
	require.NoError(t, err)

	// Should fail due to expired key
	_, err = manager.GetTenantByAPIKey(ctx, rawKey)
	assert.Error(t, err)

	var expired *ErrAPIKeyExpired
	assert.ErrorAs(t, err, &expired)
}

func TestBadgerManager_ListAPIKeys(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	tenant, err := manager.Create(ctx, &CreateTenantInput{Name: "list-keys-tenant"})
	require.NoError(t, err)

	// Create multiple API keys
	for i := 0; i < 3; i++ {
		_, _, err := manager.CreateAPIKey(ctx, &CreateAPIKeyInput{
			TenantID: tenant.ID,
			Name:     "key-" + string(rune('A'+i)),
		})
		require.NoError(t, err)
	}

	apiKeys, err := manager.ListAPIKeys(ctx, tenant.ID)
	require.NoError(t, err)
	assert.Len(t, apiKeys, 3)
}

func TestBadgerManager_ListAPIKeys_Empty(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	tenant, err := manager.Create(ctx, &CreateTenantInput{Name: "no-keys-tenant"})
	require.NoError(t, err)

	apiKeys, err := manager.ListAPIKeys(ctx, tenant.ID)
	require.NoError(t, err)
	assert.Empty(t, apiKeys)
}

func TestBadgerManager_ListAPIKeys_Validation(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	_, err := manager.ListAPIKeys(ctx, "")
	assert.Error(t, err)

	var invalidInput *ErrInvalidInput
	assert.ErrorAs(t, err, &invalidInput)
}

func TestBadgerManager_RevokeAPIKey(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	tenant, err := manager.Create(ctx, &CreateTenantInput{Name: "revoke-key-tenant"})
	require.NoError(t, err)

	_, rawKey, err := manager.CreateAPIKey(ctx, &CreateAPIKeyInput{
		TenantID: tenant.ID,
		Name:     "to-revoke",
	})
	require.NoError(t, err)

	// Revoke the key
	err = manager.RevokeAPIKey(ctx, rawKey)
	require.NoError(t, err)

	// Should no longer be retrievable
	_, err = manager.GetAPIKey(ctx, rawKey)
	assert.Error(t, err)

	// Should no longer appear in list
	apiKeys, err := manager.ListAPIKeys(ctx, tenant.ID)
	require.NoError(t, err)
	assert.Empty(t, apiKeys)
}

func TestBadgerManager_RevokeAPIKey_NotFound(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	err := manager.RevokeAPIKey(ctx, "nonexistent-key")
	assert.Error(t, err)

	var notFound *ErrAPIKeyNotFound
	assert.ErrorAs(t, err, &notFound)
}

func TestBadgerManager_UpdateAPIKeyLastUsed(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	tenant, err := manager.Create(ctx, &CreateTenantInput{Name: "last-used-tenant"})
	require.NoError(t, err)

	apiKey, rawKey, err := manager.CreateAPIKey(ctx, &CreateAPIKeyInput{
		TenantID: tenant.ID,
		Name:     "track-usage",
	})
	require.NoError(t, err)
	assert.True(t, apiKey.LastUsedAt.IsZero())

	// Update last used
	err = manager.UpdateAPIKeyLastUsed(ctx, rawKey)
	require.NoError(t, err)

	// Verify it was updated
	retrieved, err := manager.GetAPIKey(ctx, rawKey)
	require.NoError(t, err)
	assert.False(t, retrieved.LastUsedAt.IsZero())
}

func TestBadgerManager_UpdateAPIKeyLastUsed_NotFound(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	// Should not error for nonexistent key (silently ignore)
	err := manager.UpdateAPIKeyLastUsed(ctx, "nonexistent-key")
	assert.NoError(t, err)
}

func TestBadgerManager_APIKeyIsolation(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	// Create two tenants
	tenant1, err := manager.Create(ctx, &CreateTenantInput{Name: "tenant-1"})
	require.NoError(t, err)

	tenant2, err := manager.Create(ctx, &CreateTenantInput{Name: "tenant-2"})
	require.NoError(t, err)

	// Create API keys for each
	_, key1, err := manager.CreateAPIKey(ctx, &CreateAPIKeyInput{
		TenantID: tenant1.ID,
		Name:     "key-1",
	})
	require.NoError(t, err)

	_, key2, err := manager.CreateAPIKey(ctx, &CreateAPIKeyInput{
		TenantID: tenant2.ID,
		Name:     "key-2",
	})
	require.NoError(t, err)

	// Verify keys belong to correct tenants
	t1, err := manager.GetTenantByAPIKey(ctx, key1)
	require.NoError(t, err)
	assert.Equal(t, tenant1.ID, t1.ID)

	t2, err := manager.GetTenantByAPIKey(ctx, key2)
	require.NoError(t, err)
	assert.Equal(t, tenant2.ID, t2.ID)

	// Verify list isolation
	keys1, err := manager.ListAPIKeys(ctx, tenant1.ID)
	require.NoError(t, err)
	assert.Len(t, keys1, 1)

	keys2, err := manager.ListAPIKeys(ctx, tenant2.ID)
	require.NoError(t, err)
	assert.Len(t, keys2, 1)
}
