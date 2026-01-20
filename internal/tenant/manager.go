package tenant

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
)

// Key prefixes for tenant storage.
const (
	prefixTenant      = "tenant:"
	prefixTenantName  = "tenant_name:"
	prefixTenantUsage = "tenant_usage:"
)

// BadgerManager implements Manager using BadgerDB.
type BadgerManager struct {
	db *badger.DB
}

// NewBadgerManager creates a new BadgerDB-based tenant manager.
func NewBadgerManager(db *badger.DB) *BadgerManager {
	return &BadgerManager{db: db}
}

// Create creates a new tenant.
func (m *BadgerManager) Create(ctx context.Context, input *CreateTenantInput) (*Tenant, error) {
	if input == nil {
		return nil, &ErrInvalidInput{Field: "input", Message: "cannot be nil"}
	}
	if input.Name == "" {
		return nil, &ErrInvalidInput{Field: "name", Message: "cannot be empty"}
	}

	now := time.Now().UTC()
	plan := input.Plan
	if plan == "" {
		plan = PlanFree
	}

	// Use default quotas and config if not provided
	quotas := input.Quotas
	if quotas.MaxMemories == 0 && quotas.MaxStorageBytes == 0 {
		quotas = DefaultQuotas(plan)
	}

	config := input.Config
	if config.DefaultTokenBudget == 0 {
		config = DefaultConfig(plan)
	}

	tenant := &Tenant{
		ID:        uuid.New().String(),
		Name:      input.Name,
		Plan:      plan,
		Status:    StatusActive,
		Config:    config,
		Quotas:    quotas,
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  input.Metadata,
	}

	data, err := json.Marshal(tenant)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tenant: %w", err)
	}

	err = m.db.Update(func(txn *badger.Txn) error {
		// Check if name already exists
		_, err := txn.Get([]byte(prefixTenantName + tenant.Name))
		if err == nil {
			return &ErrAlreadyExists{Name: tenant.Name}
		}
		if err != badger.ErrKeyNotFound {
			return err
		}

		// Store the tenant
		if err := txn.Set([]byte(prefixTenant+tenant.ID), data); err != nil {
			return err
		}

		// Store name index
		if err := txn.Set([]byte(prefixTenantName+tenant.Name), []byte(tenant.ID)); err != nil {
			return err
		}

		// Initialize usage
		usage := &Usage{
			TenantID:    tenant.ID,
			LastUpdated: now,
		}
		usageData, err := json.Marshal(usage)
		if err != nil {
			return err
		}
		if err := txn.Set([]byte(prefixTenantUsage+tenant.ID), usageData); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return tenant, nil
}

// Get retrieves a tenant by ID.
func (m *BadgerManager) Get(ctx context.Context, id string) (*Tenant, error) {
	var tenant Tenant

	err := m.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(prefixTenant + id))
		if err == badger.ErrKeyNotFound {
			return &ErrNotFound{ID: id}
		}
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &tenant)
		})
	})

	if err != nil {
		return nil, err
	}

	return &tenant, nil
}

// GetByName retrieves a tenant by name.
func (m *BadgerManager) GetByName(ctx context.Context, name string) (*Tenant, error) {
	var tenantID string

	err := m.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(prefixTenantName + name))
		if err == badger.ErrKeyNotFound {
			return &ErrNotFound{Name: name}
		}
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			tenantID = string(val)
			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	return m.Get(ctx, tenantID)
}

// Update updates a tenant.
func (m *BadgerManager) Update(ctx context.Context, id string, input *UpdateTenantInput) (*Tenant, error) {
	var tenant *Tenant

	err := m.db.Update(func(txn *badger.Txn) error {
		// Get existing tenant
		item, err := txn.Get([]byte(prefixTenant + id))
		if err == badger.ErrKeyNotFound {
			return &ErrNotFound{ID: id}
		}
		if err != nil {
			return err
		}

		var existing Tenant
		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, &existing)
		}); err != nil {
			return err
		}

		// Handle name change
		if input.Name != nil && *input.Name != existing.Name {
			// Check if new name already exists
			_, err := txn.Get([]byte(prefixTenantName + *input.Name))
			if err == nil {
				return &ErrAlreadyExists{Name: *input.Name}
			}
			if err != badger.ErrKeyNotFound {
				return err
			}

			// Delete old name index
			if err := txn.Delete([]byte(prefixTenantName + existing.Name)); err != nil {
				return err
			}

			// Create new name index
			if err := txn.Set([]byte(prefixTenantName+*input.Name), []byte(id)); err != nil {
				return err
			}

			existing.Name = *input.Name
		}

		// Apply updates
		if input.Plan != nil {
			existing.Plan = *input.Plan
		}
		if input.Status != nil {
			existing.Status = *input.Status
		}
		if input.Config != nil {
			existing.Config = *input.Config
		}
		if input.Quotas != nil {
			existing.Quotas = *input.Quotas
		}
		if input.Metadata != nil {
			existing.Metadata = input.Metadata
		}
		existing.UpdatedAt = time.Now().UTC()

		data, err := json.Marshal(&existing)
		if err != nil {
			return err
		}

		if err := txn.Set([]byte(prefixTenant+id), data); err != nil {
			return err
		}

		tenant = &existing
		return nil
	})

	if err != nil {
		return nil, err
	}

	return tenant, nil
}

// Delete deletes a tenant.
func (m *BadgerManager) Delete(ctx context.Context, id string) error {
	return m.db.Update(func(txn *badger.Txn) error {
		// Get the tenant first
		item, err := txn.Get([]byte(prefixTenant + id))
		if err == badger.ErrKeyNotFound {
			return &ErrNotFound{ID: id}
		}
		if err != nil {
			return err
		}

		var tenant Tenant
		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, &tenant)
		}); err != nil {
			return err
		}

		// Delete tenant
		if err := txn.Delete([]byte(prefixTenant + id)); err != nil {
			return err
		}

		// Delete name index
		if err := txn.Delete([]byte(prefixTenantName + tenant.Name)); err != nil && err != badger.ErrKeyNotFound {
			return err
		}

		// Delete usage
		if err := txn.Delete([]byte(prefixTenantUsage + id)); err != nil && err != badger.ErrKeyNotFound {
			return err
		}

		return nil
	})
}

// List lists tenants with optional filtering.
func (m *BadgerManager) List(ctx context.Context, opts *ListTenantsOptions) ([]*Tenant, error) {
	if opts == nil {
		opts = &ListTenantsOptions{Limit: 100}
	}
	if opts.Limit <= 0 {
		opts.Limit = 100
	}

	var tenants []*Tenant
	prefix := []byte(prefixTenant)

	err := m.db.View(func(txn *badger.Txn) error {
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

			var tenant Tenant
			if err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &tenant)
			}); err != nil {
				continue
			}

			// Apply filters
			if opts.Status != "" && tenant.Status != opts.Status {
				continue
			}
			if opts.Plan != "" && tenant.Plan != opts.Plan {
				continue
			}

			tenants = append(tenants, &tenant)
			count++
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return tenants, nil
}

// Suspend suspends a tenant.
func (m *BadgerManager) Suspend(ctx context.Context, id string) error {
	status := StatusSuspended
	_, err := m.Update(ctx, id, &UpdateTenantInput{Status: &status})
	return err
}

// Activate activates a suspended tenant.
func (m *BadgerManager) Activate(ctx context.Context, id string) error {
	status := StatusActive
	_, err := m.Update(ctx, id, &UpdateTenantInput{Status: &status})
	return err
}

// GetUsage retrieves a tenant's current usage.
func (m *BadgerManager) GetUsage(ctx context.Context, id string) (*Usage, error) {
	var usage Usage

	err := m.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(prefixTenantUsage + id))
		if err == badger.ErrKeyNotFound {
			return &ErrNotFound{ID: id}
		}
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &usage)
		})
	})

	if err != nil {
		return nil, err
	}

	return &usage, nil
}

// IncrementUsage increments usage counters for a tenant.
func (m *BadgerManager) IncrementUsage(ctx context.Context, id string, memories, storage int64) error {
	return m.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(prefixTenantUsage + id))
		if err == badger.ErrKeyNotFound {
			return &ErrNotFound{ID: id}
		}
		if err != nil {
			return err
		}

		var usage Usage
		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, &usage)
		}); err != nil {
			return err
		}

		usage.MemoryCount += memories
		usage.StorageBytes += storage
		usage.LastUpdated = time.Now().UTC()

		data, err := json.Marshal(&usage)
		if err != nil {
			return err
		}

		return txn.Set([]byte(prefixTenantUsage+id), data)
	})
}

// EnsureSystemTenant ensures the system tenant exists.
func (m *BadgerManager) EnsureSystemTenant(ctx context.Context) (*Tenant, error) {
	// Try to get existing system tenant
	tenant, err := m.Get(ctx, SystemTenantID)
	if err == nil {
		return tenant, nil
	}

	// Check if it's a "not found" error
	if _, ok := err.(*ErrNotFound); !ok {
		return nil, err
	}

	// Create system tenant with premium quotas
	now := time.Now().UTC()
	tenant = &Tenant{
		ID:     SystemTenantID,
		Name:   SystemTenantName,
		Plan:   PlanPremium,
		Status: StatusActive,
		Config: Config{
			EmbeddingModel:     "all-MiniLM-L6-v2",
			DefaultTokenBudget: 8000,
			MaxNamespaces:      0, // Unlimited
			RetentionDays:      0, // Forever
			DedicatedStorage:   false,
		},
		Quotas: Quotas{
			MaxMemories:       0, // Unlimited
			MaxStorageBytes:   0, // Unlimited
			RequestsPerMinute: 0, // Unlimited
			RequestsPerDay:    0, // Unlimited
		},
		CreatedAt: now,
		UpdatedAt: now,
		Metadata: map[string]interface{}{
			"description": "Default system tenant for backward compatibility",
		},
	}

	data, err := json.Marshal(tenant)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal system tenant: %w", err)
	}

	err = m.db.Update(func(txn *badger.Txn) error {
		// Check if already exists (race condition protection)
		_, err := txn.Get([]byte(prefixTenant + SystemTenantID))
		if err == nil {
			return nil // Already exists
		}
		if err != badger.ErrKeyNotFound {
			return err
		}

		// Store the tenant
		if err := txn.Set([]byte(prefixTenant+tenant.ID), data); err != nil {
			return err
		}

		// Store name index
		if err := txn.Set([]byte(prefixTenantName+tenant.Name), []byte(tenant.ID)); err != nil {
			return err
		}

		// Initialize usage
		usage := &Usage{
			TenantID:    tenant.ID,
			LastUpdated: now,
		}
		usageData, err := json.Marshal(usage)
		if err != nil {
			return err
		}
		return txn.Set([]byte(prefixTenantUsage+tenant.ID), usageData)
	})

	if err != nil {
		return nil, err
	}

	return tenant, nil
}
