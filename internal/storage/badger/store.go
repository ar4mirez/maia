// Package badger provides a BadgerDB-based storage implementation for MAIA.
package badger

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"

	"github.com/ar4mirez/maia/internal/storage"
)

// Key prefixes for different data types.
const (
	prefixMemory    = "mem:"
	prefixNamespace = "ns:"
	prefixNSByName  = "nsn:" // namespace by name index
	prefixMemByNS   = "mns:" // memory by namespace index
)

// Store implements storage.Store using BadgerDB.
type Store struct {
	db     *badger.DB
	mu     sync.RWMutex
	closed bool
}

// Options holds configuration for the BadgerDB store.
type Options struct {
	DataDir    string
	SyncWrites bool
	Logger     badger.Logger
}

// New creates a new BadgerDB store.
func New(opts *Options) (*Store, error) {
	if opts == nil {
		return nil, fmt.Errorf("options cannot be nil")
	}
	if opts.DataDir == "" {
		return nil, fmt.Errorf("data directory is required")
	}

	// Create BadgerDB options
	dbOpts := badger.DefaultOptions(opts.DataDir)
	dbOpts.SyncWrites = opts.SyncWrites

	// Reduce memory usage for development
	dbOpts.ValueLogFileSize = 64 << 20 // 64MB
	dbOpts.MemTableSize = 16 << 20     // 16MB

	if opts.Logger != nil {
		dbOpts.Logger = opts.Logger
	} else {
		dbOpts.Logger = nil // Disable default logging
	}

	db, err := badger.Open(dbOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to open BadgerDB: %w", err)
	}

	return &Store{db: db}, nil
}

// NewWithPath creates a new BadgerDB store with just a path (convenience method).
func NewWithPath(dataDir string) (*Store, error) {
	return New(&Options{DataDir: dataDir})
}

// Close closes the BadgerDB store.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true
	return s.db.Close()
}

// CreateMemory creates a new memory.
func (s *Store) CreateMemory(ctx context.Context, input *storage.CreateMemoryInput) (*storage.Memory, error) {
	if err := validateCreateMemoryInput(input); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	mem := &storage.Memory{
		ID:          uuid.New().String(),
		Namespace:   input.Namespace,
		Content:     input.Content,
		Type:        input.Type,
		Embedding:   input.Embedding,
		Metadata:    input.Metadata,
		Tags:        input.Tags,
		CreatedAt:   now,
		UpdatedAt:   now,
		AccessedAt:  now,
		AccessCount: 0,
		Confidence:  input.Confidence,
		Source:      input.Source,
		Relations:   input.Relations,
	}

	// Set defaults
	if mem.Type == "" {
		mem.Type = storage.MemoryTypeSemantic
	}
	if mem.Source == "" {
		mem.Source = storage.MemorySourceUser
	}
	if mem.Confidence == 0 {
		mem.Confidence = 1.0
	}

	data, err := json.Marshal(mem)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal memory: %w", err)
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		// Store the memory
		if err := txn.Set([]byte(prefixMemory+mem.ID), data); err != nil {
			return err
		}

		// Store namespace index
		nsIndexKey := []byte(prefixMemByNS + mem.Namespace + ":" + mem.ID)
		if err := txn.Set(nsIndexKey, []byte(mem.ID)); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create memory: %w", err)
	}

	return mem, nil
}

// GetMemory retrieves a memory by ID.
func (s *Store) GetMemory(ctx context.Context, id string) (*storage.Memory, error) {
	var mem storage.Memory

	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(prefixMemory + id))
		if err == badger.ErrKeyNotFound {
			return &storage.ErrNotFound{Type: "memory", ID: id}
		}
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &mem)
		})
	})

	if err != nil {
		return nil, err
	}

	return &mem, nil
}

// UpdateMemory updates an existing memory.
func (s *Store) UpdateMemory(ctx context.Context, id string, input *storage.UpdateMemoryInput) (*storage.Memory, error) {
	var mem *storage.Memory

	err := s.db.Update(func(txn *badger.Txn) error {
		// Get existing memory
		item, err := txn.Get([]byte(prefixMemory + id))
		if err == badger.ErrKeyNotFound {
			return &storage.ErrNotFound{Type: "memory", ID: id}
		}
		if err != nil {
			return err
		}

		var existing storage.Memory
		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, &existing)
		}); err != nil {
			return err
		}

		// Apply updates
		if input.Content != nil {
			existing.Content = *input.Content
		}
		if input.Embedding != nil {
			existing.Embedding = input.Embedding
		}
		if input.Metadata != nil {
			existing.Metadata = input.Metadata
		}
		if input.Tags != nil {
			existing.Tags = input.Tags
		}
		if input.Confidence != nil {
			existing.Confidence = *input.Confidence
		}
		if input.Relations != nil {
			existing.Relations = input.Relations
		}
		existing.UpdatedAt = time.Now().UTC()

		data, err := json.Marshal(&existing)
		if err != nil {
			return err
		}

		if err := txn.Set([]byte(prefixMemory+id), data); err != nil {
			return err
		}

		mem = &existing
		return nil
	})

	if err != nil {
		return nil, err
	}

	return mem, nil
}

// DeleteMemory deletes a memory by ID.
func (s *Store) DeleteMemory(ctx context.Context, id string) error {
	return s.db.Update(func(txn *badger.Txn) error {
		// Get the memory first to find its namespace
		item, err := txn.Get([]byte(prefixMemory + id))
		if err == badger.ErrKeyNotFound {
			return &storage.ErrNotFound{Type: "memory", ID: id}
		}
		if err != nil {
			return err
		}

		var mem storage.Memory
		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, &mem)
		}); err != nil {
			return err
		}

		// Delete the memory
		if err := txn.Delete([]byte(prefixMemory + id)); err != nil {
			return err
		}

		// Delete namespace index
		nsIndexKey := []byte(prefixMemByNS + mem.Namespace + ":" + id)
		if err := txn.Delete(nsIndexKey); err != nil && err != badger.ErrKeyNotFound {
			return err
		}

		return nil
	})
}

// ListMemories lists memories in a namespace.
func (s *Store) ListMemories(ctx context.Context, namespace string, opts *storage.ListOptions) ([]*storage.Memory, error) {
	if opts == nil {
		opts = &storage.ListOptions{Limit: 100}
	}
	if opts.Limit <= 0 {
		opts.Limit = 100
	}

	var memories []*storage.Memory
	prefix := []byte(prefixMemByNS + namespace + ":")

	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		count := 0
		skipped := 0

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			// Handle offset
			if skipped < opts.Offset {
				skipped++
				continue
			}

			// Handle limit
			if count >= opts.Limit {
				break
			}

			// Get memory ID from index value
			item := it.Item()
			var memID string
			if err := item.Value(func(val []byte) error {
				memID = string(val)
				return nil
			}); err != nil {
				return err
			}

			// Get the actual memory
			memItem, err := txn.Get([]byte(prefixMemory + memID))
			if err == badger.ErrKeyNotFound {
				continue // Index orphan, skip
			}
			if err != nil {
				return err
			}

			var mem storage.Memory
			if err := memItem.Value(func(val []byte) error {
				return json.Unmarshal(val, &mem)
			}); err != nil {
				return err
			}

			memories = append(memories, &mem)
			count++
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return memories, nil
}

// SearchMemories searches memories with filters.
func (s *Store) SearchMemories(ctx context.Context, opts *storage.SearchOptions) ([]*storage.SearchResult, error) {
	if opts == nil {
		opts = &storage.SearchOptions{Limit: 100}
	}
	if opts.Limit <= 0 {
		opts.Limit = 100
	}

	var results []*storage.SearchResult

	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		// Determine prefix based on namespace filter
		var prefix []byte
		if opts.Namespace != "" {
			prefix = []byte(prefixMemByNS + opts.Namespace + ":")
		} else {
			prefix = []byte(prefixMemory)
		}

		count := 0
		skipped := 0

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			if count >= opts.Limit {
				break
			}

			item := it.Item()

			var mem storage.Memory
			var data []byte

			// Handle different prefix types
			if opts.Namespace != "" {
				// Get memory ID from index
				var memID string
				if err := item.Value(func(val []byte) error {
					memID = string(val)
					return nil
				}); err != nil {
					return err
				}

				memItem, err := txn.Get([]byte(prefixMemory + memID))
				if err == badger.ErrKeyNotFound {
					continue
				}
				if err != nil {
					return err
				}

				if err := memItem.Value(func(val []byte) error {
					data = append([]byte{}, val...)
					return nil
				}); err != nil {
					return err
				}
			} else {
				if err := item.Value(func(val []byte) error {
					data = append([]byte{}, val...)
					return nil
				}); err != nil {
					return err
				}
			}

			if err := json.Unmarshal(data, &mem); err != nil {
				continue // Skip invalid entries
			}

			// Apply filters
			if !matchesFilters(&mem, opts) {
				continue
			}

			// Handle offset
			if skipped < opts.Offset {
				skipped++
				continue
			}

			results = append(results, &storage.SearchResult{
				Memory: &mem,
				Score:  1.0, // Basic implementation, no scoring yet
			})
			count++
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return results, nil
}

// TouchMemory updates the access time and count for a memory.
func (s *Store) TouchMemory(ctx context.Context, id string) error {
	return s.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(prefixMemory + id))
		if err == badger.ErrKeyNotFound {
			return &storage.ErrNotFound{Type: "memory", ID: id}
		}
		if err != nil {
			return err
		}

		var mem storage.Memory
		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, &mem)
		}); err != nil {
			return err
		}

		mem.AccessedAt = time.Now().UTC()
		mem.AccessCount++

		data, err := json.Marshal(&mem)
		if err != nil {
			return err
		}

		return txn.Set([]byte(prefixMemory+id), data)
	})
}

// CreateNamespace creates a new namespace.
func (s *Store) CreateNamespace(ctx context.Context, input *storage.CreateNamespaceInput) (*storage.Namespace, error) {
	if input.Name == "" {
		return nil, &storage.ErrInvalidInput{Field: "name", Message: "cannot be empty"}
	}

	now := time.Now().UTC()
	ns := &storage.Namespace{
		ID:        uuid.New().String(),
		Name:      input.Name,
		Parent:    input.Parent,
		Template:  input.Template,
		Config:    input.Config,
		CreatedAt: now,
		UpdatedAt: now,
	}

	data, err := json.Marshal(ns)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal namespace: %w", err)
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		// Check if name already exists
		_, err := txn.Get([]byte(prefixNSByName + ns.Name))
		if err == nil {
			return &storage.ErrAlreadyExists{Type: "namespace", ID: ns.Name}
		}
		if err != badger.ErrKeyNotFound {
			return err
		}

		// Store the namespace
		if err := txn.Set([]byte(prefixNamespace+ns.ID), data); err != nil {
			return err
		}

		// Store name index
		if err := txn.Set([]byte(prefixNSByName+ns.Name), []byte(ns.ID)); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return ns, nil
}

// GetNamespace retrieves a namespace by ID.
func (s *Store) GetNamespace(ctx context.Context, id string) (*storage.Namespace, error) {
	var ns storage.Namespace

	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(prefixNamespace + id))
		if err == badger.ErrKeyNotFound {
			return &storage.ErrNotFound{Type: "namespace", ID: id}
		}
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &ns)
		})
	})

	if err != nil {
		return nil, err
	}

	return &ns, nil
}

// GetNamespaceByName retrieves a namespace by name.
func (s *Store) GetNamespaceByName(ctx context.Context, name string) (*storage.Namespace, error) {
	var nsID string

	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(prefixNSByName + name))
		if err == badger.ErrKeyNotFound {
			return &storage.ErrNotFound{Type: "namespace", ID: name}
		}
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			nsID = string(val)
			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	return s.GetNamespace(ctx, nsID)
}

// UpdateNamespace updates a namespace's configuration.
func (s *Store) UpdateNamespace(ctx context.Context, id string, config *storage.NamespaceConfig) (*storage.Namespace, error) {
	var ns *storage.Namespace

	err := s.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(prefixNamespace + id))
		if err == badger.ErrKeyNotFound {
			return &storage.ErrNotFound{Type: "namespace", ID: id}
		}
		if err != nil {
			return err
		}

		var existing storage.Namespace
		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, &existing)
		}); err != nil {
			return err
		}

		existing.Config = *config
		existing.UpdatedAt = time.Now().UTC()

		data, err := json.Marshal(&existing)
		if err != nil {
			return err
		}

		if err := txn.Set([]byte(prefixNamespace+id), data); err != nil {
			return err
		}

		ns = &existing
		return nil
	})

	if err != nil {
		return nil, err
	}

	return ns, nil
}

// DeleteNamespace deletes a namespace.
func (s *Store) DeleteNamespace(ctx context.Context, id string) error {
	return s.db.Update(func(txn *badger.Txn) error {
		// Get the namespace first
		item, err := txn.Get([]byte(prefixNamespace + id))
		if err == badger.ErrKeyNotFound {
			return &storage.ErrNotFound{Type: "namespace", ID: id}
		}
		if err != nil {
			return err
		}

		var ns storage.Namespace
		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, &ns)
		}); err != nil {
			return err
		}

		// Delete namespace
		if err := txn.Delete([]byte(prefixNamespace + id)); err != nil {
			return err
		}

		// Delete name index
		if err := txn.Delete([]byte(prefixNSByName + ns.Name)); err != nil && err != badger.ErrKeyNotFound {
			return err
		}

		return nil
	})
}

// ListNamespaces lists all namespaces.
func (s *Store) ListNamespaces(ctx context.Context, opts *storage.ListOptions) ([]*storage.Namespace, error) {
	if opts == nil {
		opts = &storage.ListOptions{Limit: 100}
	}
	if opts.Limit <= 0 {
		opts.Limit = 100
	}

	var namespaces []*storage.Namespace
	prefix := []byte(prefixNamespace)

	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		count := 0
		skipped := 0

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			if skipped < opts.Offset {
				skipped++
				continue
			}
			if count >= opts.Limit {
				break
			}

			var ns storage.Namespace
			if err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &ns)
			}); err != nil {
				continue
			}

			namespaces = append(namespaces, &ns)
			count++
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return namespaces, nil
}

// BatchCreateMemories creates multiple memories in a batch.
func (s *Store) BatchCreateMemories(ctx context.Context, inputs []*storage.CreateMemoryInput) ([]*storage.Memory, error) {
	memories := make([]*storage.Memory, 0, len(inputs))

	wb := s.db.NewWriteBatch()
	defer wb.Cancel()

	now := time.Now().UTC()

	for _, input := range inputs {
		if err := validateCreateMemoryInput(input); err != nil {
			return nil, err
		}

		mem := &storage.Memory{
			ID:          uuid.New().String(),
			Namespace:   input.Namespace,
			Content:     input.Content,
			Type:        input.Type,
			Embedding:   input.Embedding,
			Metadata:    input.Metadata,
			Tags:        input.Tags,
			CreatedAt:   now,
			UpdatedAt:   now,
			AccessedAt:  now,
			AccessCount: 0,
			Confidence:  input.Confidence,
			Source:      input.Source,
			Relations:   input.Relations,
		}

		if mem.Type == "" {
			mem.Type = storage.MemoryTypeSemantic
		}
		if mem.Source == "" {
			mem.Source = storage.MemorySourceUser
		}
		if mem.Confidence == 0 {
			mem.Confidence = 1.0
		}

		data, err := json.Marshal(mem)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal memory: %w", err)
		}

		if err := wb.Set([]byte(prefixMemory+mem.ID), data); err != nil {
			return nil, err
		}

		nsIndexKey := []byte(prefixMemByNS + mem.Namespace + ":" + mem.ID)
		if err := wb.Set(nsIndexKey, []byte(mem.ID)); err != nil {
			return nil, err
		}

		memories = append(memories, mem)
	}

	if err := wb.Flush(); err != nil {
		return nil, fmt.Errorf("failed to flush batch: %w", err)
	}

	return memories, nil
}

// BatchDeleteMemories deletes multiple memories.
func (s *Store) BatchDeleteMemories(ctx context.Context, ids []string) error {
	return s.db.Update(func(txn *badger.Txn) error {
		for _, id := range ids {
			// Get the memory first to find its namespace
			item, err := txn.Get([]byte(prefixMemory + id))
			if err == badger.ErrKeyNotFound {
				continue // Skip missing
			}
			if err != nil {
				return err
			}

			var mem storage.Memory
			if err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &mem)
			}); err != nil {
				continue
			}

			if err := txn.Delete([]byte(prefixMemory + id)); err != nil {
				return err
			}

			nsIndexKey := []byte(prefixMemByNS + mem.Namespace + ":" + id)
			_ = txn.Delete(nsIndexKey) // Ignore if not found
		}

		return nil
	})
}

// Stats returns storage statistics.
func (s *Store) Stats(ctx context.Context) (*storage.StoreStats, error) {
	stats := &storage.StoreStats{}

	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		// Count memories
		memPrefix := []byte(prefixMemory)
		for it.Seek(memPrefix); it.ValidForPrefix(memPrefix); it.Next() {
			stats.TotalMemories++
		}

		// Count namespaces
		nsPrefix := []byte(prefixNamespace)
		for it.Seek(nsPrefix); it.ValidForPrefix(nsPrefix); it.Next() {
			stats.TotalNamespaces++
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Get storage size
	lsm, vlog := s.db.Size()
	stats.StorageSizeBytes = lsm + vlog

	return stats, nil
}

// DataDir returns the data directory path.
func (s *Store) DataDir() string {
	return filepath.Dir(s.db.Opts().Dir)
}

// validateCreateMemoryInput validates memory creation input.
func validateCreateMemoryInput(input *storage.CreateMemoryInput) error {
	if input == nil {
		return &storage.ErrInvalidInput{Field: "input", Message: "cannot be nil"}
	}
	if input.Content == "" {
		return &storage.ErrInvalidInput{Field: "content", Message: "cannot be empty"}
	}
	if input.Namespace == "" {
		return &storage.ErrInvalidInput{Field: "namespace", Message: "cannot be empty"}
	}
	return nil
}

// matchesFilters checks if a memory matches the search filters.
func matchesFilters(mem *storage.Memory, opts *storage.SearchOptions) bool {
	// Filter by types
	if len(opts.Types) > 0 {
		found := false
		for _, t := range opts.Types {
			if mem.Type == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Filter by tags
	if len(opts.Tags) > 0 {
		for _, reqTag := range opts.Tags {
			found := false
			for _, memTag := range mem.Tags {
				if memTag == reqTag {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}

	// Filter by time range
	if opts.TimeRange != nil {
		if !opts.TimeRange.Start.IsZero() && mem.CreatedAt.Before(opts.TimeRange.Start) {
			return false
		}
		if !opts.TimeRange.End.IsZero() && mem.CreatedAt.After(opts.TimeRange.End) {
			return false
		}
	}

	return true
}
