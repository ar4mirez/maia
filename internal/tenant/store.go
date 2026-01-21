// Package tenant provides multi-tenancy support for MAIA.
package tenant

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/ar4mirez/maia/internal/storage"
)

// TenantSeparator is used to separate tenant ID from namespace.
const TenantSeparator = "::"

// TenantAwareStore wraps a storage.Store with tenant isolation.
// It prefixes all namespaces with the tenant ID to ensure data isolation.
type TenantAwareStore struct {
	store   storage.Store
	manager Manager

	// dedicatedStores holds per-tenant BadgerDB instances for premium tenants.
	dedicatedStores map[string]storage.Store
	mu              sync.RWMutex
}

// NewTenantAwareStore creates a new tenant-aware store wrapper.
func NewTenantAwareStore(store storage.Store, manager Manager) *TenantAwareStore {
	return &TenantAwareStore{
		store:           store,
		manager:         manager,
		dedicatedStores: make(map[string]storage.Store),
	}
}

// getStoreForTenant returns the appropriate store for the given tenant.
// Premium tenants with dedicated storage get their own store instance.
func (s *TenantAwareStore) getStoreForTenant(ctx context.Context, tenantID string) (storage.Store, error) {
	if tenantID == "" || tenantID == SystemTenantID {
		return s.store, nil
	}

	tenant, err := s.manager.Get(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}

	if !tenant.Config.DedicatedStorage {
		return s.store, nil
	}

	// Check for existing dedicated store
	s.mu.RLock()
	dedicatedStore, ok := s.dedicatedStores[tenantID]
	s.mu.RUnlock()
	if ok {
		return dedicatedStore, nil
	}

	// For now, return shared store even for premium tenants
	// Dedicated store initialization can be implemented later
	return s.store, nil
}

// RegisterDedicatedStore registers a dedicated store for a premium tenant.
func (s *TenantAwareStore) RegisterDedicatedStore(tenantID string, store storage.Store) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dedicatedStores[tenantID] = store
}

// prefixNamespace adds the tenant ID prefix to a namespace.
func prefixNamespace(tenantID, namespace string) string {
	if tenantID == "" || tenantID == SystemTenantID {
		return namespace
	}
	return tenantID + TenantSeparator + namespace
}

// unprefixNamespace removes the tenant ID prefix from a namespace.
func unprefixNamespace(tenantID, namespace string) string {
	if tenantID == "" || tenantID == SystemTenantID {
		return namespace
	}
	prefix := tenantID + TenantSeparator
	if strings.HasPrefix(namespace, prefix) {
		return strings.TrimPrefix(namespace, prefix)
	}
	return namespace
}

// validateTenantOwnership checks if a memory or namespace belongs to the tenant.
func (s *TenantAwareStore) validateTenantOwnership(tenantID, namespace string) bool {
	if tenantID == "" || tenantID == SystemTenantID {
		return true
	}
	prefix := tenantID + TenantSeparator
	return strings.HasPrefix(namespace, prefix)
}

// CreateMemory creates a new memory with tenant isolation.
func (s *TenantAwareStore) CreateMemory(ctx context.Context, tenantID string, input *storage.CreateMemoryInput) (*storage.Memory, error) {
	store, err := s.getStoreForTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// Create a copy of input with prefixed namespace
	prefixedInput := &storage.CreateMemoryInput{
		Namespace:  prefixNamespace(tenantID, input.Namespace),
		Content:    input.Content,
		Type:       input.Type,
		Embedding:  input.Embedding,
		Metadata:   input.Metadata,
		Tags:       input.Tags,
		Confidence: input.Confidence,
		Source:     input.Source,
		Relations:  input.Relations,
	}

	mem, err := store.CreateMemory(ctx, prefixedInput)
	if err != nil {
		return nil, err
	}

	// Update usage tracking
	if s.manager != nil && tenantID != "" && tenantID != SystemTenantID {
		// Estimate storage size (content + metadata overhead)
		storageSize := int64(len(mem.Content) + 500) // 500 bytes overhead estimate
		// Ignore errors from usage tracking - it's best-effort
		_ = s.manager.IncrementUsage(ctx, tenantID, 1, storageSize)
	}

	// Return with unprefixed namespace for client visibility
	mem.Namespace = unprefixNamespace(tenantID, mem.Namespace)
	return mem, nil
}

// GetMemory retrieves a memory with tenant validation.
func (s *TenantAwareStore) GetMemory(ctx context.Context, tenantID, id string) (*storage.Memory, error) {
	store, err := s.getStoreForTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	mem, err := store.GetMemory(ctx, id)
	if err != nil {
		return nil, err
	}

	// Validate ownership
	if !s.validateTenantOwnership(tenantID, mem.Namespace) {
		return nil, &storage.ErrNotFound{Type: "memory", ID: id}
	}

	// Return with unprefixed namespace
	mem.Namespace = unprefixNamespace(tenantID, mem.Namespace)
	return mem, nil
}

// UpdateMemory updates a memory with tenant validation.
func (s *TenantAwareStore) UpdateMemory(ctx context.Context, tenantID, id string, input *storage.UpdateMemoryInput) (*storage.Memory, error) {
	store, err := s.getStoreForTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// First, verify ownership
	existing, err := store.GetMemory(ctx, id)
	if err != nil {
		return nil, err
	}

	if !s.validateTenantOwnership(tenantID, existing.Namespace) {
		return nil, &storage.ErrNotFound{Type: "memory", ID: id}
	}

	mem, err := store.UpdateMemory(ctx, id, input)
	if err != nil {
		return nil, err
	}

	// Return with unprefixed namespace
	mem.Namespace = unprefixNamespace(tenantID, mem.Namespace)
	return mem, nil
}

// DeleteMemory deletes a memory with tenant validation.
func (s *TenantAwareStore) DeleteMemory(ctx context.Context, tenantID, id string) error {
	store, err := s.getStoreForTenant(ctx, tenantID)
	if err != nil {
		return err
	}

	// First, verify ownership
	existing, err := store.GetMemory(ctx, id)
	if err != nil {
		return err
	}

	if !s.validateTenantOwnership(tenantID, existing.Namespace) {
		return &storage.ErrNotFound{Type: "memory", ID: id}
	}

	if err := store.DeleteMemory(ctx, id); err != nil {
		return err
	}

	// Update usage tracking (decrement)
	if s.manager != nil && tenantID != "" && tenantID != SystemTenantID {
		storageSize := int64(len(existing.Content) + 500)
		// Ignore errors from usage tracking - it's best-effort
		_ = s.manager.IncrementUsage(ctx, tenantID, -1, -storageSize)
	}

	return nil
}

// ListMemories lists memories in a namespace with tenant isolation.
func (s *TenantAwareStore) ListMemories(ctx context.Context, tenantID, namespace string, opts *storage.ListOptions) ([]*storage.Memory, error) {
	store, err := s.getStoreForTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// Prefix the namespace
	prefixedNamespace := prefixNamespace(tenantID, namespace)

	memories, err := store.ListMemories(ctx, prefixedNamespace, opts)
	if err != nil {
		return nil, err
	}

	// Unprefix namespaces in results
	for _, mem := range memories {
		mem.Namespace = unprefixNamespace(tenantID, mem.Namespace)
	}

	return memories, nil
}

// SearchMemories searches memories with tenant isolation.
func (s *TenantAwareStore) SearchMemories(ctx context.Context, tenantID string, opts *storage.SearchOptions) ([]*storage.SearchResult, error) {
	store, err := s.getStoreForTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// Create a copy of options with prefixed namespace
	prefixedOpts := &storage.SearchOptions{
		Namespace: prefixNamespace(tenantID, opts.Namespace),
		Types:     opts.Types,
		Tags:      opts.Tags,
		TimeRange: opts.TimeRange,
		Filters:   opts.Filters,
		Limit:     opts.Limit,
		Offset:    opts.Offset,
		SortBy:    opts.SortBy,
		SortOrder: opts.SortOrder,
	}

	results, err := store.SearchMemories(ctx, prefixedOpts)
	if err != nil {
		return nil, err
	}

	// Unprefix namespaces in results
	for _, result := range results {
		if result.Memory != nil {
			result.Memory.Namespace = unprefixNamespace(tenantID, result.Memory.Namespace)
		}
	}

	return results, nil
}

// TouchMemory updates access time with tenant validation.
func (s *TenantAwareStore) TouchMemory(ctx context.Context, tenantID, id string) error {
	store, err := s.getStoreForTenant(ctx, tenantID)
	if err != nil {
		return err
	}

	// First, verify ownership
	existing, err := store.GetMemory(ctx, id)
	if err != nil {
		return err
	}

	if !s.validateTenantOwnership(tenantID, existing.Namespace) {
		return &storage.ErrNotFound{Type: "memory", ID: id}
	}

	return store.TouchMemory(ctx, id)
}

// CreateNamespace creates a namespace with tenant isolation.
func (s *TenantAwareStore) CreateNamespace(ctx context.Context, tenantID string, input *storage.CreateNamespaceInput) (*storage.Namespace, error) {
	store, err := s.getStoreForTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// Create a copy of input with prefixed name
	prefixedInput := &storage.CreateNamespaceInput{
		Name:     prefixNamespace(tenantID, input.Name),
		Parent:   input.Parent,
		Template: input.Template,
		Config:   input.Config,
	}

	// Also prefix the parent if provided
	if prefixedInput.Parent != "" {
		prefixedInput.Parent = prefixNamespace(tenantID, prefixedInput.Parent)
	}

	ns, err := store.CreateNamespace(ctx, prefixedInput)
	if err != nil {
		return nil, err
	}

	// Return with unprefixed name
	ns.Name = unprefixNamespace(tenantID, ns.Name)
	if ns.Parent != "" {
		ns.Parent = unprefixNamespace(tenantID, ns.Parent)
	}

	return ns, nil
}

// GetNamespace retrieves a namespace with tenant validation.
func (s *TenantAwareStore) GetNamespace(ctx context.Context, tenantID, id string) (*storage.Namespace, error) {
	store, err := s.getStoreForTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	ns, err := store.GetNamespace(ctx, id)
	if err != nil {
		return nil, err
	}

	// Validate ownership
	if !s.validateTenantOwnership(tenantID, ns.Name) {
		return nil, &storage.ErrNotFound{Type: "namespace", ID: id}
	}

	// Return with unprefixed name
	ns.Name = unprefixNamespace(tenantID, ns.Name)
	if ns.Parent != "" {
		ns.Parent = unprefixNamespace(tenantID, ns.Parent)
	}

	return ns, nil
}

// GetNamespaceByName retrieves a namespace by name with tenant isolation.
func (s *TenantAwareStore) GetNamespaceByName(ctx context.Context, tenantID, name string) (*storage.Namespace, error) {
	store, err := s.getStoreForTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	prefixedName := prefixNamespace(tenantID, name)
	ns, err := store.GetNamespaceByName(ctx, prefixedName)
	if err != nil {
		return nil, err
	}

	// Return with unprefixed name
	ns.Name = unprefixNamespace(tenantID, ns.Name)
	if ns.Parent != "" {
		ns.Parent = unprefixNamespace(tenantID, ns.Parent)
	}

	return ns, nil
}

// UpdateNamespace updates a namespace with tenant validation.
func (s *TenantAwareStore) UpdateNamespace(ctx context.Context, tenantID, id string, config *storage.NamespaceConfig) (*storage.Namespace, error) {
	store, err := s.getStoreForTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// First, verify ownership
	existing, err := store.GetNamespace(ctx, id)
	if err != nil {
		return nil, err
	}

	if !s.validateTenantOwnership(tenantID, existing.Name) {
		return nil, &storage.ErrNotFound{Type: "namespace", ID: id}
	}

	ns, err := store.UpdateNamespace(ctx, id, config)
	if err != nil {
		return nil, err
	}

	// Return with unprefixed name
	ns.Name = unprefixNamespace(tenantID, ns.Name)
	if ns.Parent != "" {
		ns.Parent = unprefixNamespace(tenantID, ns.Parent)
	}

	return ns, nil
}

// DeleteNamespace deletes a namespace with tenant validation.
func (s *TenantAwareStore) DeleteNamespace(ctx context.Context, tenantID, id string) error {
	store, err := s.getStoreForTenant(ctx, tenantID)
	if err != nil {
		return err
	}

	// First, verify ownership
	existing, err := store.GetNamespace(ctx, id)
	if err != nil {
		return err
	}

	if !s.validateTenantOwnership(tenantID, existing.Name) {
		return &storage.ErrNotFound{Type: "namespace", ID: id}
	}

	return store.DeleteNamespace(ctx, id)
}

// ListNamespaces lists namespaces for a tenant.
func (s *TenantAwareStore) ListNamespaces(ctx context.Context, tenantID string, opts *storage.ListOptions) ([]*storage.Namespace, error) {
	store, err := s.getStoreForTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// Get all namespaces
	allNamespaces, err := store.ListNamespaces(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Filter to only those belonging to the tenant
	var result []*storage.Namespace
	for _, ns := range allNamespaces {
		if s.validateTenantOwnership(tenantID, ns.Name) {
			// Unprefix the name
			ns.Name = unprefixNamespace(tenantID, ns.Name)
			if ns.Parent != "" {
				ns.Parent = unprefixNamespace(tenantID, ns.Parent)
			}
			result = append(result, ns)
		}
	}

	return result, nil
}

// BatchCreateMemories creates multiple memories with tenant isolation.
func (s *TenantAwareStore) BatchCreateMemories(ctx context.Context, tenantID string, inputs []*storage.CreateMemoryInput) ([]*storage.Memory, error) {
	store, err := s.getStoreForTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// Prefix all namespaces
	prefixedInputs := make([]*storage.CreateMemoryInput, len(inputs))
	for i, input := range inputs {
		prefixedInputs[i] = &storage.CreateMemoryInput{
			Namespace:  prefixNamespace(tenantID, input.Namespace),
			Content:    input.Content,
			Type:       input.Type,
			Embedding:  input.Embedding,
			Metadata:   input.Metadata,
			Tags:       input.Tags,
			Confidence: input.Confidence,
			Source:     input.Source,
			Relations:  input.Relations,
		}
	}

	memories, err := store.BatchCreateMemories(ctx, prefixedInputs)
	if err != nil {
		return nil, err
	}

	// Unprefix namespaces and track usage
	var totalStorage int64
	for _, mem := range memories {
		mem.Namespace = unprefixNamespace(tenantID, mem.Namespace)
		totalStorage += int64(len(mem.Content) + 500)
	}

	// Update usage tracking
	if s.manager != nil && tenantID != "" && tenantID != SystemTenantID {
		// Ignore errors from usage tracking - it's best-effort
		_ = s.manager.IncrementUsage(ctx, tenantID, int64(len(memories)), totalStorage)
	}

	return memories, nil
}

// BatchDeleteMemories deletes multiple memories with tenant validation.
func (s *TenantAwareStore) BatchDeleteMemories(ctx context.Context, tenantID string, ids []string) error {
	store, err := s.getStoreForTenant(ctx, tenantID)
	if err != nil {
		return err
	}

	// Verify ownership of all memories before deleting
	var validIDs []string
	var totalStorage int64
	for _, id := range ids {
		mem, err := store.GetMemory(ctx, id)
		if err != nil {
			continue // Skip not found
		}
		if s.validateTenantOwnership(tenantID, mem.Namespace) {
			validIDs = append(validIDs, id)
			totalStorage += int64(len(mem.Content) + 500)
		}
	}

	if len(validIDs) == 0 {
		return nil
	}

	if err := store.BatchDeleteMemories(ctx, validIDs); err != nil {
		return err
	}

	// Update usage tracking (decrement)
	if s.manager != nil && tenantID != "" && tenantID != SystemTenantID {
		// Ignore errors from usage tracking - it's best-effort
		_ = s.manager.IncrementUsage(ctx, tenantID, -int64(len(validIDs)), -totalStorage)
	}

	return nil
}

// Close closes the store and all dedicated stores.
func (s *TenantAwareStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var lastErr error
	for _, dedicatedStore := range s.dedicatedStores {
		if err := dedicatedStore.Close(); err != nil {
			lastErr = err
		}
	}

	if err := s.store.Close(); err != nil {
		lastErr = err
	}

	return lastErr
}

// Stats returns statistics for a tenant's data.
func (s *TenantAwareStore) Stats(ctx context.Context, tenantID string) (*storage.StoreStats, error) {
	store, err := s.getStoreForTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// For system tenant, return overall stats
	if tenantID == "" || tenantID == SystemTenantID {
		return store.Stats(ctx)
	}

	// For specific tenants, we need to calculate stats from usage tracking
	if s.manager != nil {
		usage, err := s.manager.GetUsage(ctx, tenantID)
		if err != nil {
			return nil, err
		}
		return &storage.StoreStats{
			TotalMemories:    usage.MemoryCount,
			TotalNamespaces:  usage.NamespaceCount,
			StorageSizeBytes: usage.StorageBytes,
		}, nil
	}

	// Fallback to overall stats
	return store.Stats(ctx)
}

// UnderlyingStore returns the underlying store for advanced operations.
// Use with caution as it bypasses tenant isolation.
func (s *TenantAwareStore) UnderlyingStore() storage.Store {
	return s.store
}
