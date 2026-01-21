package tenant

import (
	"context"
	"os"
	"testing"

	badgerdb "github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ar4mirez/maia/internal/storage"
	"github.com/ar4mirez/maia/internal/storage/badger"
)

func setupTestStoreWithManager(t *testing.T) (*TenantAwareStore, Manager, func()) {
	t.Helper()

	tmpDir := t.TempDir()

	// Create underlying store
	store, err := badger.NewWithPath(tmpDir + "/data")
	require.NoError(t, err)

	// Create tenant manager with BadgerDB
	opts := badgerdb.DefaultOptions(tmpDir + "/tenants")
	opts.Logger = nil
	db, err := badgerdb.Open(opts)
	require.NoError(t, err)

	manager := NewBadgerManager(db)

	// Create some test tenants
	_, err = manager.Create(context.Background(), &CreateTenantInput{
		Name: "tenant1",
		Plan: PlanStandard,
	})
	require.NoError(t, err)

	_, err = manager.Create(context.Background(), &CreateTenantInput{
		Name: "tenant2",
		Plan: PlanStandard,
	})
	require.NoError(t, err)

	// Create tenant-aware store
	tenantStore := NewTenantAwareStore(store, manager)

	cleanup := func() {
		tenantStore.Close()
		db.Close()
	}

	return tenantStore, manager, cleanup
}

func TestTenantAwareStore_CreateMemory(t *testing.T) {
	store, manager, cleanup := setupTestStoreWithManager(t)
	defer cleanup()

	ctx := context.Background()

	// Get tenant1
	tenant1, err := manager.GetByName(ctx, "tenant1")
	require.NoError(t, err)

	// Create memory for tenant1
	mem, err := store.CreateMemory(ctx, tenant1.ID, &storage.CreateMemoryInput{
		Namespace:  "default",
		Content:    "Tenant 1 memory",
		Type:       storage.MemoryTypeSemantic,
		Confidence: 1.0,
		Source:     storage.MemorySourceUser,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, mem.ID)
	assert.Equal(t, "default", mem.Namespace) // Should be unprefixed
	assert.Equal(t, "Tenant 1 memory", mem.Content)
}

func TestTenantAwareStore_TenantIsolation(t *testing.T) {
	store, manager, cleanup := setupTestStoreWithManager(t)
	defer cleanup()

	ctx := context.Background()

	// Get tenants
	tenant1, err := manager.GetByName(ctx, "tenant1")
	require.NoError(t, err)
	tenant2, err := manager.GetByName(ctx, "tenant2")
	require.NoError(t, err)

	// Create memory for tenant1
	mem1, err := store.CreateMemory(ctx, tenant1.ID, &storage.CreateMemoryInput{
		Namespace:  "default",
		Content:    "Tenant 1 secret data",
		Type:       storage.MemoryTypeSemantic,
		Confidence: 1.0,
		Source:     storage.MemorySourceUser,
	})
	require.NoError(t, err)

	// Create memory for tenant2
	mem2, err := store.CreateMemory(ctx, tenant2.ID, &storage.CreateMemoryInput{
		Namespace:  "default",
		Content:    "Tenant 2 secret data",
		Type:       storage.MemoryTypeSemantic,
		Confidence: 1.0,
		Source:     storage.MemorySourceUser,
	})
	require.NoError(t, err)

	// Tenant1 should be able to access their own memory
	retrieved, err := store.GetMemory(ctx, tenant1.ID, mem1.ID)
	require.NoError(t, err)
	assert.Equal(t, "Tenant 1 secret data", retrieved.Content)

	// Tenant1 should NOT be able to access tenant2's memory
	_, err = store.GetMemory(ctx, tenant1.ID, mem2.ID)
	assert.Error(t, err)
	var notFoundErr *storage.ErrNotFound
	assert.ErrorAs(t, err, &notFoundErr)

	// Tenant2 should be able to access their own memory
	retrieved2, err := store.GetMemory(ctx, tenant2.ID, mem2.ID)
	require.NoError(t, err)
	assert.Equal(t, "Tenant 2 secret data", retrieved2.Content)
}

func TestTenantAwareStore_ListMemories_Isolation(t *testing.T) {
	store, manager, cleanup := setupTestStoreWithManager(t)
	defer cleanup()

	ctx := context.Background()

	// Get tenants
	tenant1, err := manager.GetByName(ctx, "tenant1")
	require.NoError(t, err)
	tenant2, err := manager.GetByName(ctx, "tenant2")
	require.NoError(t, err)

	// Create memories for tenant1
	for i := 0; i < 3; i++ {
		_, err := store.CreateMemory(ctx, tenant1.ID, &storage.CreateMemoryInput{
			Namespace:  "default",
			Content:    "Tenant 1 memory",
			Type:       storage.MemoryTypeSemantic,
			Confidence: 1.0,
			Source:     storage.MemorySourceUser,
		})
		require.NoError(t, err)
	}

	// Create memories for tenant2
	for i := 0; i < 5; i++ {
		_, err := store.CreateMemory(ctx, tenant2.ID, &storage.CreateMemoryInput{
			Namespace:  "default",
			Content:    "Tenant 2 memory",
			Type:       storage.MemoryTypeSemantic,
			Confidence: 1.0,
			Source:     storage.MemorySourceUser,
		})
		require.NoError(t, err)
	}

	// List memories for tenant1 - should only see 3
	memories1, err := store.ListMemories(ctx, tenant1.ID, "default", nil)
	require.NoError(t, err)
	assert.Len(t, memories1, 3)

	// List memories for tenant2 - should only see 5
	memories2, err := store.ListMemories(ctx, tenant2.ID, "default", nil)
	require.NoError(t, err)
	assert.Len(t, memories2, 5)
}

func TestTenantAwareStore_UpdateMemory_Isolation(t *testing.T) {
	store, manager, cleanup := setupTestStoreWithManager(t)
	defer cleanup()

	ctx := context.Background()

	// Get tenants
	tenant1, err := manager.GetByName(ctx, "tenant1")
	require.NoError(t, err)
	tenant2, err := manager.GetByName(ctx, "tenant2")
	require.NoError(t, err)

	// Create memory for tenant1
	mem, err := store.CreateMemory(ctx, tenant1.ID, &storage.CreateMemoryInput{
		Namespace:  "default",
		Content:    "Original content",
		Type:       storage.MemoryTypeSemantic,
		Confidence: 1.0,
		Source:     storage.MemorySourceUser,
	})
	require.NoError(t, err)

	// Tenant1 can update their own memory
	newContent := "Updated content"
	updated, err := store.UpdateMemory(ctx, tenant1.ID, mem.ID, &storage.UpdateMemoryInput{
		Content: &newContent,
	})
	require.NoError(t, err)
	assert.Equal(t, "Updated content", updated.Content)

	// Tenant2 should NOT be able to update tenant1's memory
	_, err = store.UpdateMemory(ctx, tenant2.ID, mem.ID, &storage.UpdateMemoryInput{
		Content: &newContent,
	})
	assert.Error(t, err)
}

func TestTenantAwareStore_DeleteMemory_Isolation(t *testing.T) {
	store, manager, cleanup := setupTestStoreWithManager(t)
	defer cleanup()

	ctx := context.Background()

	// Get tenants
	tenant1, err := manager.GetByName(ctx, "tenant1")
	require.NoError(t, err)
	tenant2, err := manager.GetByName(ctx, "tenant2")
	require.NoError(t, err)

	// Create memory for tenant1
	mem, err := store.CreateMemory(ctx, tenant1.ID, &storage.CreateMemoryInput{
		Namespace:  "default",
		Content:    "Memory to delete",
		Type:       storage.MemoryTypeSemantic,
		Confidence: 1.0,
		Source:     storage.MemorySourceUser,
	})
	require.NoError(t, err)

	// Tenant2 should NOT be able to delete tenant1's memory
	err = store.DeleteMemory(ctx, tenant2.ID, mem.ID)
	assert.Error(t, err)

	// Memory should still exist
	_, err = store.GetMemory(ctx, tenant1.ID, mem.ID)
	assert.NoError(t, err)

	// Tenant1 can delete their own memory
	err = store.DeleteMemory(ctx, tenant1.ID, mem.ID)
	assert.NoError(t, err)

	// Memory should be gone
	_, err = store.GetMemory(ctx, tenant1.ID, mem.ID)
	assert.Error(t, err)
}

func TestTenantAwareStore_Namespace_Operations(t *testing.T) {
	store, manager, cleanup := setupTestStoreWithManager(t)
	defer cleanup()

	ctx := context.Background()

	// Get tenant
	tenant1, err := manager.GetByName(ctx, "tenant1")
	require.NoError(t, err)

	// Create namespace
	ns, err := store.CreateNamespace(ctx, tenant1.ID, &storage.CreateNamespaceInput{
		Name: "my-namespace",
		Config: storage.NamespaceConfig{
			TokenBudget: 4000,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "my-namespace", ns.Name) // Should be unprefixed

	// Get namespace by ID
	retrieved, err := store.GetNamespace(ctx, tenant1.ID, ns.ID)
	require.NoError(t, err)
	assert.Equal(t, "my-namespace", retrieved.Name)

	// Get namespace by name
	retrievedByName, err := store.GetNamespaceByName(ctx, tenant1.ID, "my-namespace")
	require.NoError(t, err)
	assert.Equal(t, ns.ID, retrievedByName.ID)

	// Update namespace
	updated, err := store.UpdateNamespace(ctx, tenant1.ID, ns.ID, &storage.NamespaceConfig{
		TokenBudget: 8000,
	})
	require.NoError(t, err)
	assert.Equal(t, 8000, updated.Config.TokenBudget)

	// List namespaces
	namespaces, err := store.ListNamespaces(ctx, tenant1.ID, nil)
	require.NoError(t, err)
	assert.Len(t, namespaces, 1)

	// Delete namespace
	err = store.DeleteNamespace(ctx, tenant1.ID, ns.ID)
	assert.NoError(t, err)
}

func TestTenantAwareStore_Namespace_Isolation(t *testing.T) {
	store, manager, cleanup := setupTestStoreWithManager(t)
	defer cleanup()

	ctx := context.Background()

	// Get tenants
	tenant1, err := manager.GetByName(ctx, "tenant1")
	require.NoError(t, err)
	tenant2, err := manager.GetByName(ctx, "tenant2")
	require.NoError(t, err)

	// Create namespace for tenant1
	ns1, err := store.CreateNamespace(ctx, tenant1.ID, &storage.CreateNamespaceInput{
		Name: "shared-name",
		Config: storage.NamespaceConfig{
			TokenBudget: 4000,
		},
	})
	require.NoError(t, err)

	// Create namespace with same name for tenant2 (should work - different tenant)
	ns2, err := store.CreateNamespace(ctx, tenant2.ID, &storage.CreateNamespaceInput{
		Name: "shared-name",
		Config: storage.NamespaceConfig{
			TokenBudget: 8000,
		},
	})
	require.NoError(t, err)
	assert.NotEqual(t, ns1.ID, ns2.ID)

	// Tenant2 should NOT be able to access tenant1's namespace
	_, err = store.GetNamespace(ctx, tenant2.ID, ns1.ID)
	assert.Error(t, err)

	// Tenant2 should NOT be able to delete tenant1's namespace
	err = store.DeleteNamespace(ctx, tenant2.ID, ns1.ID)
	assert.Error(t, err)
}

func TestTenantAwareStore_BatchOperations(t *testing.T) {
	store, manager, cleanup := setupTestStoreWithManager(t)
	defer cleanup()

	ctx := context.Background()

	// Get tenant
	tenant1, err := manager.GetByName(ctx, "tenant1")
	require.NoError(t, err)

	// Batch create memories
	inputs := []*storage.CreateMemoryInput{
		{
			Namespace:  "default",
			Content:    "Memory 1",
			Type:       storage.MemoryTypeSemantic,
			Confidence: 1.0,
			Source:     storage.MemorySourceUser,
		},
		{
			Namespace:  "default",
			Content:    "Memory 2",
			Type:       storage.MemoryTypeSemantic,
			Confidence: 1.0,
			Source:     storage.MemorySourceUser,
		},
		{
			Namespace:  "default",
			Content:    "Memory 3",
			Type:       storage.MemoryTypeSemantic,
			Confidence: 1.0,
			Source:     storage.MemorySourceUser,
		},
	}

	memories, err := store.BatchCreateMemories(ctx, tenant1.ID, inputs)
	require.NoError(t, err)
	assert.Len(t, memories, 3)

	// Verify all have unprefixed namespaces
	for _, mem := range memories {
		assert.Equal(t, "default", mem.Namespace)
	}

	// Batch delete memories
	ids := make([]string, len(memories))
	for i, mem := range memories {
		ids[i] = mem.ID
	}

	err = store.BatchDeleteMemories(ctx, tenant1.ID, ids)
	require.NoError(t, err)

	// Verify all deleted
	for _, id := range ids {
		_, err := store.GetMemory(ctx, tenant1.ID, id)
		assert.Error(t, err)
	}
}

func TestTenantAwareStore_BatchDeleteMemories_Isolation(t *testing.T) {
	store, manager, cleanup := setupTestStoreWithManager(t)
	defer cleanup()

	ctx := context.Background()

	// Get tenants
	tenant1, err := manager.GetByName(ctx, "tenant1")
	require.NoError(t, err)
	tenant2, err := manager.GetByName(ctx, "tenant2")
	require.NoError(t, err)

	// Create memory for tenant1
	mem1, err := store.CreateMemory(ctx, tenant1.ID, &storage.CreateMemoryInput{
		Namespace:  "default",
		Content:    "Tenant 1 memory",
		Type:       storage.MemoryTypeSemantic,
		Confidence: 1.0,
		Source:     storage.MemorySourceUser,
	})
	require.NoError(t, err)

	// Tenant2 tries to batch delete tenant1's memory
	err = store.BatchDeleteMemories(ctx, tenant2.ID, []string{mem1.ID})
	require.NoError(t, err) // Should succeed but not delete anything

	// Memory should still exist
	_, err = store.GetMemory(ctx, tenant1.ID, mem1.ID)
	assert.NoError(t, err)
}

func TestTenantAwareStore_SystemTenant(t *testing.T) {
	store, _, cleanup := setupTestStoreWithManager(t)
	defer cleanup()

	ctx := context.Background()

	// System tenant should have no prefix
	mem, err := store.CreateMemory(ctx, SystemTenantID, &storage.CreateMemoryInput{
		Namespace:  "default",
		Content:    "System memory",
		Type:       storage.MemoryTypeSemantic,
		Confidence: 1.0,
		Source:     storage.MemorySourceUser,
	})
	require.NoError(t, err)
	assert.Equal(t, "default", mem.Namespace)

	// Empty tenant ID should also work like system tenant
	mem2, err := store.CreateMemory(ctx, "", &storage.CreateMemoryInput{
		Namespace:  "default",
		Content:    "No tenant memory",
		Type:       storage.MemoryTypeSemantic,
		Confidence: 1.0,
		Source:     storage.MemorySourceUser,
	})
	require.NoError(t, err)
	assert.Equal(t, "default", mem2.Namespace)
}

func TestTenantAwareStore_TouchMemory_Isolation(t *testing.T) {
	store, manager, cleanup := setupTestStoreWithManager(t)
	defer cleanup()

	ctx := context.Background()

	// Get tenants
	tenant1, err := manager.GetByName(ctx, "tenant1")
	require.NoError(t, err)
	tenant2, err := manager.GetByName(ctx, "tenant2")
	require.NoError(t, err)

	// Create memory for tenant1
	mem, err := store.CreateMemory(ctx, tenant1.ID, &storage.CreateMemoryInput{
		Namespace:  "default",
		Content:    "Tenant 1 memory",
		Type:       storage.MemoryTypeSemantic,
		Confidence: 1.0,
		Source:     storage.MemorySourceUser,
	})
	require.NoError(t, err)

	// Tenant1 can touch their own memory
	err = store.TouchMemory(ctx, tenant1.ID, mem.ID)
	assert.NoError(t, err)

	// Tenant2 should NOT be able to touch tenant1's memory
	err = store.TouchMemory(ctx, tenant2.ID, mem.ID)
	assert.Error(t, err)
}

func TestTenantAwareStore_Stats(t *testing.T) {
	store, manager, cleanup := setupTestStoreWithManager(t)
	defer cleanup()

	ctx := context.Background()

	// Get tenant
	tenant1, err := manager.GetByName(ctx, "tenant1")
	require.NoError(t, err)

	// Create some memories
	for i := 0; i < 5; i++ {
		_, err := store.CreateMemory(ctx, tenant1.ID, &storage.CreateMemoryInput{
			Namespace:  "default",
			Content:    "Memory content",
			Type:       storage.MemoryTypeSemantic,
			Confidence: 1.0,
			Source:     storage.MemorySourceUser,
		})
		require.NoError(t, err)
	}

	// Get stats for tenant
	stats, err := store.Stats(ctx, tenant1.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(5), stats.TotalMemories)
}

func TestPrefixUnprefixNamespace(t *testing.T) {
	tests := []struct {
		name      string
		tenantID  string
		namespace string
		expected  string
	}{
		{
			name:      "regular tenant",
			tenantID:  "tenant123",
			namespace: "default",
			expected:  "tenant123::default",
		},
		{
			name:      "system tenant",
			tenantID:  SystemTenantID,
			namespace: "default",
			expected:  "default",
		},
		{
			name:      "empty tenant ID",
			tenantID:  "",
			namespace: "default",
			expected:  "default",
		},
		{
			name:      "nested namespace",
			tenantID:  "tenant123",
			namespace: "org/project/user",
			expected:  "tenant123::org/project/user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefixed := prefixNamespace(tt.tenantID, tt.namespace)
			assert.Equal(t, tt.expected, prefixed)

			// Unprefix should return original
			unprefixed := unprefixNamespace(tt.tenantID, prefixed)
			assert.Equal(t, tt.namespace, unprefixed)
		})
	}
}

func TestTenantAwareStore_RegisterDedicatedStore(t *testing.T) {
	store, _, cleanup := setupTestStoreWithManager(t)
	defer cleanup()

	// Create a dedicated store
	dedicatedDir := t.TempDir()
	dedicatedStore, err := badger.NewWithPath(dedicatedDir)
	require.NoError(t, err)

	// Register dedicated store
	store.RegisterDedicatedStore("premium-tenant", dedicatedStore)

	// Verify it's registered
	assert.NotNil(t, store.dedicatedStores["premium-tenant"])
	assert.True(t, store.HasDedicatedStore("premium-tenant"))
	assert.False(t, store.HasDedicatedStore("non-existent"))
	assert.Equal(t, 1, store.DedicatedStoreCount())
}

func TestTenantAwareStore_UnregisterDedicatedStore(t *testing.T) {
	store, _, cleanup := setupTestStoreWithManager(t)
	defer cleanup()

	// Create a dedicated store
	dedicatedDir := t.TempDir()
	dedicatedStore, err := badger.NewWithPath(dedicatedDir)
	require.NoError(t, err)

	// Register dedicated store
	store.RegisterDedicatedStore("premium-tenant", dedicatedStore)
	assert.True(t, store.HasDedicatedStore("premium-tenant"))

	// Unregister it
	err = store.UnregisterDedicatedStore("premium-tenant")
	require.NoError(t, err)

	// Verify it's gone
	assert.False(t, store.HasDedicatedStore("premium-tenant"))
	assert.Equal(t, 0, store.DedicatedStoreCount())

	// Unregistering non-existent should not error
	err = store.UnregisterDedicatedStore("non-existent")
	require.NoError(t, err)
}

func TestTenantAwareStore_DedicatedStorageLazyInit(t *testing.T) {
	tmpDir := t.TempDir()

	// Create underlying store
	store, err := badger.NewWithPath(tmpDir + "/data")
	require.NoError(t, err)

	// Create tenant manager with BadgerDB
	opts := badgerdb.DefaultOptions(tmpDir + "/tenants")
	opts.Logger = nil
	db, err := badgerdb.Open(opts)
	require.NoError(t, err)

	manager := NewBadgerManager(db)

	// Create a premium tenant with dedicated storage
	premiumTenant, err := manager.Create(context.Background(), &CreateTenantInput{
		Name: "premium-tenant",
		Plan: PlanPremium,
		Config: Config{
			DedicatedStorage: true,
		},
	})
	require.NoError(t, err)

	// Create tenant-aware store with dedicated storage config
	dedicatedDir := tmpDir + "/dedicated"
	tenantStore := NewTenantAwareStoreWithDedicated(store, manager, &DedicatedStorageConfig{
		BaseDir:    dedicatedDir,
		SyncWrites: false,
	})

	defer func() {
		tenantStore.Close()
		db.Close()
	}()

	ctx := context.Background()

	// Initially no dedicated stores
	assert.Equal(t, 0, tenantStore.DedicatedStoreCount())

	// Create memory for premium tenant - should lazily initialize dedicated store
	mem, err := tenantStore.CreateMemory(ctx, premiumTenant.ID, &storage.CreateMemoryInput{
		Namespace:  "default",
		Content:    "Premium tenant memory",
		Type:       storage.MemoryTypeSemantic,
		Confidence: 1.0,
		Source:     storage.MemorySourceUser,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, mem.ID)

	// Dedicated store should now be initialized
	assert.True(t, tenantStore.HasDedicatedStore(premiumTenant.ID))
	assert.Equal(t, 1, tenantStore.DedicatedStoreCount())

	// Verify the dedicated directory was created
	_, err = os.Stat(dedicatedDir + "/" + premiumTenant.ID)
	assert.NoError(t, err)

	// Retrieve the memory - should use dedicated store
	retrieved, err := tenantStore.GetMemory(ctx, premiumTenant.ID, mem.ID)
	require.NoError(t, err)
	assert.Equal(t, "Premium tenant memory", retrieved.Content)
}

func TestTenantAwareStore_DedicatedStorageIsolation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create underlying store
	store, err := badger.NewWithPath(tmpDir + "/data")
	require.NoError(t, err)

	// Create tenant manager with BadgerDB
	opts := badgerdb.DefaultOptions(tmpDir + "/tenants")
	opts.Logger = nil
	db, err := badgerdb.Open(opts)
	require.NoError(t, err)

	manager := NewBadgerManager(db)

	// Create a premium tenant with dedicated storage
	premiumTenant, err := manager.Create(context.Background(), &CreateTenantInput{
		Name: "premium-tenant",
		Plan: PlanPremium,
		Config: Config{
			DedicatedStorage: true,
		},
	})
	require.NoError(t, err)

	// Create a standard tenant (uses shared store)
	standardTenant, err := manager.Create(context.Background(), &CreateTenantInput{
		Name: "standard-tenant",
		Plan: PlanStandard,
	})
	require.NoError(t, err)

	// Create tenant-aware store with dedicated storage config
	dedicatedDir := tmpDir + "/dedicated"
	tenantStore := NewTenantAwareStoreWithDedicated(store, manager, &DedicatedStorageConfig{
		BaseDir:    dedicatedDir,
		SyncWrites: false,
	})

	defer func() {
		tenantStore.Close()
		db.Close()
	}()

	ctx := context.Background()

	// Create memory for premium tenant (uses dedicated store)
	premiumMem, err := tenantStore.CreateMemory(ctx, premiumTenant.ID, &storage.CreateMemoryInput{
		Namespace:  "default",
		Content:    "Premium secret",
		Type:       storage.MemoryTypeSemantic,
		Confidence: 1.0,
		Source:     storage.MemorySourceUser,
	})
	require.NoError(t, err)

	// Create memory for standard tenant (uses shared store)
	standardMem, err := tenantStore.CreateMemory(ctx, standardTenant.ID, &storage.CreateMemoryInput{
		Namespace:  "default",
		Content:    "Standard data",
		Type:       storage.MemoryTypeSemantic,
		Confidence: 1.0,
		Source:     storage.MemorySourceUser,
	})
	require.NoError(t, err)

	// Premium can access their own memory
	retrieved, err := tenantStore.GetMemory(ctx, premiumTenant.ID, premiumMem.ID)
	require.NoError(t, err)
	assert.Equal(t, "Premium secret", retrieved.Content)

	// Standard can access their own memory
	retrieved, err = tenantStore.GetMemory(ctx, standardTenant.ID, standardMem.ID)
	require.NoError(t, err)
	assert.Equal(t, "Standard data", retrieved.Content)

	// Cross-tenant access should fail (different stores entirely)
	_, err = tenantStore.GetMemory(ctx, standardTenant.ID, premiumMem.ID)
	assert.Error(t, err)

	_, err = tenantStore.GetMemory(ctx, premiumTenant.ID, standardMem.ID)
	assert.Error(t, err)
}

func TestTenantAwareStore_SetDedicatedStorageConfig(t *testing.T) {
	store, _, cleanup := setupTestStoreWithManager(t)
	defer cleanup()

	// Initially no dedicated config
	store.mu.RLock()
	assert.Nil(t, store.dedicatedConfig)
	store.mu.RUnlock()

	// Set config
	config := &DedicatedStorageConfig{
		BaseDir:    "/tmp/dedicated",
		SyncWrites: true,
	}
	store.SetDedicatedStorageConfig(config)

	// Verify config is set
	store.mu.RLock()
	assert.NotNil(t, store.dedicatedConfig)
	assert.Equal(t, "/tmp/dedicated", store.dedicatedConfig.BaseDir)
	assert.True(t, store.dedicatedConfig.SyncWrites)
	store.mu.RUnlock()
}

func TestTenantAwareStore_DedicatedStorageWithoutConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Create underlying store
	store, err := badger.NewWithPath(tmpDir + "/data")
	require.NoError(t, err)

	// Create tenant manager with BadgerDB
	opts := badgerdb.DefaultOptions(tmpDir + "/tenants")
	opts.Logger = nil
	db, err := badgerdb.Open(opts)
	require.NoError(t, err)

	manager := NewBadgerManager(db)

	// Create a premium tenant with dedicated storage
	premiumTenant, err := manager.Create(context.Background(), &CreateTenantInput{
		Name: "premium-tenant",
		Plan: PlanPremium,
		Config: Config{
			DedicatedStorage: true,
		},
	})
	require.NoError(t, err)

	// Create tenant-aware store WITHOUT dedicated storage config
	tenantStore := NewTenantAwareStore(store, manager)

	defer func() {
		tenantStore.Close()
		db.Close()
	}()

	ctx := context.Background()

	// Create memory for premium tenant - should fall back to shared store
	mem, err := tenantStore.CreateMemory(ctx, premiumTenant.ID, &storage.CreateMemoryInput{
		Namespace:  "default",
		Content:    "Premium tenant memory",
		Type:       storage.MemoryTypeSemantic,
		Confidence: 1.0,
		Source:     storage.MemorySourceUser,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, mem.ID)

	// No dedicated store should be initialized (falls back to shared)
	assert.False(t, tenantStore.HasDedicatedStore(premiumTenant.ID))
	assert.Equal(t, 0, tenantStore.DedicatedStoreCount())

	// Memory should still be retrievable via shared store
	retrieved, err := tenantStore.GetMemory(ctx, premiumTenant.ID, mem.ID)
	require.NoError(t, err)
	assert.Equal(t, "Premium tenant memory", retrieved.Content)
}

func TestTenantAwareStore_UnderlyingStore(t *testing.T) {
	store, _, cleanup := setupTestStoreWithManager(t)
	defer cleanup()

	underlying := store.UnderlyingStore()
	assert.NotNil(t, underlying)
}
