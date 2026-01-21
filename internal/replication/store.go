package replication

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ar4mirez/maia/internal/storage"
	"go.uber.org/zap"
)

// ReplicatedStore wraps a storage.Store to capture writes to the WAL.
// It implements the storage.Store interface and logs all write operations.
type ReplicatedStore struct {
	store    storage.Store
	wal      WAL
	tenantID string
	logger   *zap.Logger
}

// NewReplicatedStore creates a new store wrapper that captures writes to WAL.
func NewReplicatedStore(store storage.Store, wal WAL, tenantID string, logger *zap.Logger) *ReplicatedStore {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ReplicatedStore{
		store:    store,
		wal:      wal,
		tenantID: tenantID,
		logger:   logger,
	}
}

// CreateMemory creates a memory and logs the operation to WAL.
func (s *ReplicatedStore) CreateMemory(ctx context.Context, input *storage.CreateMemoryInput) (*storage.Memory, error) {
	// Create in the underlying store
	memory, err := s.store.CreateMemory(ctx, input)
	if err != nil {
		return nil, err
	}

	// Log to WAL
	if err := s.logMemoryOperation(ctx, OperationCreate, memory, nil); err != nil {
		s.logger.Error("failed to log memory create to WAL",
			zap.String("memory_id", memory.ID),
			zap.Error(err),
		)
		// Don't fail the operation if WAL logging fails
		// The write succeeded, we just might have replication issues
	}

	return memory, nil
}

// GetMemory retrieves a memory (read operation, no WAL logging).
func (s *ReplicatedStore) GetMemory(ctx context.Context, id string) (*storage.Memory, error) {
	return s.store.GetMemory(ctx, id)
}

// UpdateMemory updates a memory and logs the operation to WAL.
func (s *ReplicatedStore) UpdateMemory(ctx context.Context, id string, input *storage.UpdateMemoryInput) (*storage.Memory, error) {
	// Get current state for WAL
	previous, _ := s.store.GetMemory(ctx, id)

	// Update in the underlying store
	memory, err := s.store.UpdateMemory(ctx, id, input)
	if err != nil {
		return nil, err
	}

	// Log to WAL
	if err := s.logMemoryOperation(ctx, OperationUpdate, memory, previous); err != nil {
		s.logger.Error("failed to log memory update to WAL",
			zap.String("memory_id", memory.ID),
			zap.Error(err),
		)
	}

	return memory, nil
}

// DeleteMemory deletes a memory and logs the operation to WAL.
func (s *ReplicatedStore) DeleteMemory(ctx context.Context, id string) error {
	// Get current state for WAL
	previous, _ := s.store.GetMemory(ctx, id)

	// Delete from the underlying store
	if err := s.store.DeleteMemory(ctx, id); err != nil {
		return err
	}

	// Log to WAL
	if err := s.logMemoryOperation(ctx, OperationDelete, nil, previous); err != nil {
		s.logger.Error("failed to log memory delete to WAL",
			zap.String("memory_id", id),
			zap.Error(err),
		)
	}

	return nil
}

// ListMemories lists memories (read operation, no WAL logging).
func (s *ReplicatedStore) ListMemories(ctx context.Context, namespace string, opts *storage.ListOptions) ([]*storage.Memory, error) {
	return s.store.ListMemories(ctx, namespace, opts)
}

// SearchMemories searches memories (read operation, no WAL logging).
func (s *ReplicatedStore) SearchMemories(ctx context.Context, opts *storage.SearchOptions) ([]*storage.SearchResult, error) {
	return s.store.SearchMemories(ctx, opts)
}

// TouchMemory updates access time (logged as update for replication).
func (s *ReplicatedStore) TouchMemory(ctx context.Context, id string) error {
	if err := s.store.TouchMemory(ctx, id); err != nil {
		return err
	}

	// Get updated memory for WAL
	memory, err := s.store.GetMemory(ctx, id)
	if err != nil {
		return nil // Touch succeeded, just can't log
	}

	// Log to WAL (touch is a special update)
	if err := s.logMemoryOperation(ctx, OperationUpdate, memory, nil); err != nil {
		s.logger.Debug("failed to log memory touch to WAL",
			zap.String("memory_id", id),
			zap.Error(err),
		)
	}

	return nil
}

// CreateNamespace creates a namespace and logs the operation to WAL.
func (s *ReplicatedStore) CreateNamespace(ctx context.Context, input *storage.CreateNamespaceInput) (*storage.Namespace, error) {
	namespace, err := s.store.CreateNamespace(ctx, input)
	if err != nil {
		return nil, err
	}

	if err := s.logNamespaceOperation(ctx, OperationCreate, namespace, nil); err != nil {
		s.logger.Error("failed to log namespace create to WAL",
			zap.String("namespace_id", namespace.ID),
			zap.Error(err),
		)
	}

	return namespace, nil
}

// GetNamespace retrieves a namespace (read operation, no WAL logging).
func (s *ReplicatedStore) GetNamespace(ctx context.Context, id string) (*storage.Namespace, error) {
	return s.store.GetNamespace(ctx, id)
}

// GetNamespaceByName retrieves a namespace by name (read operation).
func (s *ReplicatedStore) GetNamespaceByName(ctx context.Context, name string) (*storage.Namespace, error) {
	return s.store.GetNamespaceByName(ctx, name)
}

// UpdateNamespace updates a namespace and logs the operation to WAL.
func (s *ReplicatedStore) UpdateNamespace(ctx context.Context, id string, config *storage.NamespaceConfig) (*storage.Namespace, error) {
	previous, _ := s.store.GetNamespace(ctx, id)

	namespace, err := s.store.UpdateNamespace(ctx, id, config)
	if err != nil {
		return nil, err
	}

	if err := s.logNamespaceOperation(ctx, OperationUpdate, namespace, previous); err != nil {
		s.logger.Error("failed to log namespace update to WAL",
			zap.String("namespace_id", namespace.ID),
			zap.Error(err),
		)
	}

	return namespace, nil
}

// DeleteNamespace deletes a namespace and logs the operation to WAL.
func (s *ReplicatedStore) DeleteNamespace(ctx context.Context, id string) error {
	previous, _ := s.store.GetNamespace(ctx, id)

	if err := s.store.DeleteNamespace(ctx, id); err != nil {
		return err
	}

	if err := s.logNamespaceOperation(ctx, OperationDelete, nil, previous); err != nil {
		s.logger.Error("failed to log namespace delete to WAL",
			zap.String("namespace_id", id),
			zap.Error(err),
		)
	}

	return nil
}

// ListNamespaces lists namespaces (read operation, no WAL logging).
func (s *ReplicatedStore) ListNamespaces(ctx context.Context, opts *storage.ListOptions) ([]*storage.Namespace, error) {
	return s.store.ListNamespaces(ctx, opts)
}

// BatchCreateMemories creates multiple memories and logs each to WAL.
func (s *ReplicatedStore) BatchCreateMemories(ctx context.Context, inputs []*storage.CreateMemoryInput) ([]*storage.Memory, error) {
	memories, err := s.store.BatchCreateMemories(ctx, inputs)
	if err != nil {
		return nil, err
	}

	// Log each memory to WAL
	for _, memory := range memories {
		if err := s.logMemoryOperation(ctx, OperationCreate, memory, nil); err != nil {
			s.logger.Error("failed to log batch memory create to WAL",
				zap.String("memory_id", memory.ID),
				zap.Error(err),
			)
		}
	}

	return memories, nil
}

// BatchDeleteMemories deletes multiple memories and logs each to WAL.
func (s *ReplicatedStore) BatchDeleteMemories(ctx context.Context, ids []string) error {
	// Get previous states for WAL
	var previousStates []*storage.Memory
	for _, id := range ids {
		if memory, err := s.store.GetMemory(ctx, id); err == nil {
			previousStates = append(previousStates, memory)
		}
	}

	if err := s.store.BatchDeleteMemories(ctx, ids); err != nil {
		return err
	}

	// Log each delete to WAL
	for _, memory := range previousStates {
		if err := s.logMemoryOperation(ctx, OperationDelete, nil, memory); err != nil {
			s.logger.Error("failed to log batch memory delete to WAL",
				zap.String("memory_id", memory.ID),
				zap.Error(err),
			)
		}
	}

	return nil
}

// Close closes the underlying store.
func (s *ReplicatedStore) Close() error {
	return s.store.Close()
}

// Stats returns storage statistics.
func (s *ReplicatedStore) Stats(ctx context.Context) (*storage.StoreStats, error) {
	return s.store.Stats(ctx)
}

// logMemoryOperation logs a memory operation to the WAL.
func (s *ReplicatedStore) logMemoryOperation(ctx context.Context, op Operation, memory, previous *storage.Memory) error {
	entry := &WALEntry{
		TenantID:     s.tenantID,
		Operation:    op,
		ResourceType: ResourceTypeMemory,
	}

	if memory != nil {
		entry.ResourceID = memory.ID
		entry.Namespace = memory.Namespace
		data, err := json.Marshal(memory)
		if err != nil {
			return fmt.Errorf("failed to marshal memory: %w", err)
		}
		entry.Data = data
	} else if previous != nil {
		entry.ResourceID = previous.ID
		entry.Namespace = previous.Namespace
	}

	if previous != nil {
		data, err := json.Marshal(previous)
		if err != nil {
			return fmt.Errorf("failed to marshal previous memory: %w", err)
		}
		entry.PreviousData = data
	}

	return s.wal.Append(ctx, entry)
}

// logNamespaceOperation logs a namespace operation to the WAL.
func (s *ReplicatedStore) logNamespaceOperation(ctx context.Context, op Operation, namespace, previous *storage.Namespace) error {
	entry := &WALEntry{
		TenantID:     s.tenantID,
		Operation:    op,
		ResourceType: ResourceTypeNamespace,
	}

	if namespace != nil {
		entry.ResourceID = namespace.ID
		entry.Namespace = namespace.Name
		data, err := json.Marshal(namespace)
		if err != nil {
			return fmt.Errorf("failed to marshal namespace: %w", err)
		}
		entry.Data = data
	} else if previous != nil {
		entry.ResourceID = previous.ID
		entry.Namespace = previous.Name
	}

	if previous != nil {
		data, err := json.Marshal(previous)
		if err != nil {
			return fmt.Errorf("failed to marshal previous namespace: %w", err)
		}
		entry.PreviousData = data
	}

	return s.wal.Append(ctx, entry)
}

// Underlying returns the underlying storage.Store.
func (s *ReplicatedStore) Underlying() storage.Store {
	return s.store
}

// WAL returns the WAL used by this store.
func (s *ReplicatedStore) WAL() WAL {
	return s.wal
}

// SetTenantID sets the tenant ID for WAL entries.
func (s *ReplicatedStore) SetTenantID(tenantID string) {
	s.tenantID = tenantID
}
