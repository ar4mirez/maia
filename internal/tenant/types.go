// Package tenant provides multi-tenancy support for MAIA.
package tenant

import (
	"context"
	"time"
)

// Plan represents a tenant's subscription tier.
type Plan string

const (
	// PlanFree is the free tier with limited resources.
	PlanFree Plan = "free"
	// PlanStandard is the standard paid tier.
	PlanStandard Plan = "standard"
	// PlanPremium is the premium tier with dedicated resources.
	PlanPremium Plan = "premium"
)

// Status represents a tenant's operational status.
type Status string

const (
	// StatusActive indicates the tenant is active and operational.
	StatusActive Status = "active"
	// StatusSuspended indicates the tenant is temporarily suspended.
	StatusSuspended Status = "suspended"
	// StatusPendingDeletion indicates the tenant is marked for deletion.
	StatusPendingDeletion Status = "pending_deletion"
)

// Tenant represents a MAIA tenant.
type Tenant struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Plan      Plan                   `json:"plan"`
	Status    Status                 `json:"status"`
	Config    Config                 `json:"config"`
	Quotas    Quotas                 `json:"quotas"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Config holds tenant-specific configuration.
type Config struct {
	// EmbeddingModel is the embedding model to use for this tenant.
	EmbeddingModel string `json:"embedding_model,omitempty"`
	// DefaultTokenBudget is the default token budget for context assembly.
	DefaultTokenBudget int `json:"default_token_budget,omitempty"`
	// MaxNamespaces is the maximum number of namespaces allowed.
	MaxNamespaces int `json:"max_namespaces,omitempty"`
	// RetentionDays is the number of days to retain memories (0 = forever).
	RetentionDays int `json:"retention_days,omitempty"`
	// AllowedOrigins for CORS if tenant-specific origins are needed.
	AllowedOrigins []string `json:"allowed_origins,omitempty"`
	// DedicatedStorage indicates if the tenant has dedicated storage.
	DedicatedStorage bool `json:"dedicated_storage,omitempty"`
}

// Quotas defines resource limits for a tenant.
type Quotas struct {
	// MaxMemories is the maximum number of memories allowed.
	MaxMemories int64 `json:"max_memories"`
	// MaxStorageBytes is the maximum storage allowed in bytes.
	MaxStorageBytes int64 `json:"max_storage_bytes"`
	// RequestsPerMinute is the rate limit per minute.
	RequestsPerMinute int `json:"requests_per_minute"`
	// RequestsPerDay is the daily request limit.
	RequestsPerDay int64 `json:"requests_per_day"`
}

// Usage tracks a tenant's current resource usage.
type Usage struct {
	TenantID       string    `json:"tenant_id"`
	MemoryCount    int64     `json:"memory_count"`
	NamespaceCount int64     `json:"namespace_count"`
	StorageBytes   int64     `json:"storage_bytes"`
	RequestsToday  int64     `json:"requests_today"`
	LastUpdated    time.Time `json:"last_updated"`
}

// CreateTenantInput holds input for creating a tenant.
type CreateTenantInput struct {
	Name     string                 `json:"name"`
	Plan     Plan                   `json:"plan,omitempty"`
	Config   Config                 `json:"config,omitempty"`
	Quotas   Quotas                 `json:"quotas,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// UpdateTenantInput holds input for updating a tenant.
type UpdateTenantInput struct {
	Name     *string                `json:"name,omitempty"`
	Plan     *Plan                  `json:"plan,omitempty"`
	Status   *Status                `json:"status,omitempty"`
	Config   *Config                `json:"config,omitempty"`
	Quotas   *Quotas                `json:"quotas,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ListTenantsOptions holds options for listing tenants.
type ListTenantsOptions struct {
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
	Status Status `json:"status,omitempty"`
	Plan   Plan   `json:"plan,omitempty"`
}

// Manager defines the interface for tenant management.
type Manager interface {
	// Create creates a new tenant.
	Create(ctx context.Context, input *CreateTenantInput) (*Tenant, error)

	// Get retrieves a tenant by ID.
	Get(ctx context.Context, id string) (*Tenant, error)

	// GetByName retrieves a tenant by name.
	GetByName(ctx context.Context, name string) (*Tenant, error)

	// Update updates a tenant.
	Update(ctx context.Context, id string, input *UpdateTenantInput) (*Tenant, error)

	// Delete deletes a tenant.
	Delete(ctx context.Context, id string) error

	// List lists tenants with optional filtering.
	List(ctx context.Context, opts *ListTenantsOptions) ([]*Tenant, error)

	// Suspend suspends a tenant.
	Suspend(ctx context.Context, id string) error

	// Activate activates a suspended tenant.
	Activate(ctx context.Context, id string) error

	// GetUsage retrieves a tenant's current usage.
	GetUsage(ctx context.Context, id string) (*Usage, error)

	// IncrementUsage increments usage counters for a tenant.
	IncrementUsage(ctx context.Context, id string, memories, storage int64) error
}

// DefaultQuotas returns the default quotas for a given plan.
func DefaultQuotas(plan Plan) Quotas {
	switch plan {
	case PlanFree:
		return Quotas{
			MaxMemories:       1000,
			MaxStorageBytes:   100 * 1024 * 1024, // 100MB
			RequestsPerMinute: 60,
			RequestsPerDay:    1000,
		}
	case PlanStandard:
		return Quotas{
			MaxMemories:       100000,
			MaxStorageBytes:   10 * 1024 * 1024 * 1024, // 10GB
			RequestsPerMinute: 600,
			RequestsPerDay:    100000,
		}
	case PlanPremium:
		return Quotas{
			MaxMemories:       0, // Unlimited
			MaxStorageBytes:   0, // Unlimited
			RequestsPerMinute: 6000,
			RequestsPerDay:    0, // Unlimited
		}
	default:
		return DefaultQuotas(PlanFree)
	}
}

// DefaultConfig returns the default configuration for a given plan.
func DefaultConfig(plan Plan) Config {
	switch plan {
	case PlanFree:
		return Config{
			EmbeddingModel:     "all-MiniLM-L6-v2",
			DefaultTokenBudget: 2000,
			MaxNamespaces:      5,
			RetentionDays:      30,
			DedicatedStorage:   false,
		}
	case PlanStandard:
		return Config{
			EmbeddingModel:     "all-MiniLM-L6-v2",
			DefaultTokenBudget: 4000,
			MaxNamespaces:      50,
			RetentionDays:      90,
			DedicatedStorage:   false,
		}
	case PlanPremium:
		return Config{
			EmbeddingModel:     "all-MiniLM-L6-v2",
			DefaultTokenBudget: 8000,
			MaxNamespaces:      0, // Unlimited
			RetentionDays:      0, // Forever
			DedicatedStorage:   true,
		}
	default:
		return DefaultConfig(PlanFree)
	}
}

// SystemTenantID is the ID of the default system tenant.
const SystemTenantID = "system"

// SystemTenantName is the name of the default system tenant.
const SystemTenantName = "system"
