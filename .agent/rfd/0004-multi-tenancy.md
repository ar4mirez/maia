# RFD 0004: Multi-Tenancy Architecture

## Status

**State**: Discussion
**Created**: 2026-01-19
**Last Updated**: 2026-01-19
**Author**: MAIA Team

## Summary

This RFD proposes a multi-tenancy architecture for MAIA to support isolated deployments for multiple organizations or customers from a single instance. This enables MAIA to be deployed as a shared service while maintaining strict data isolation and resource management between tenants.

## Problem Statement

Currently, MAIA is designed as a single-tenant system where one deployment serves one organization. As MAIA adoption grows, there's a need to:

1. **Reduce operational overhead**: Running separate MAIA instances for each customer is expensive
2. **Enable SaaS deployment**: Allow MAIA to be offered as a managed service
3. **Maintain data isolation**: Ensure strict separation between tenant data
4. **Fair resource allocation**: Prevent one tenant from monopolizing shared resources
5. **Tenant-specific configuration**: Allow each tenant to customize their MAIA instance

## Requirements

### Functional Requirements

1. **Tenant Isolation**
   - Complete data isolation at storage level
   - No cross-tenant data leakage in queries
   - Separate namespace hierarchies per tenant

2. **Authentication & Authorization**
   - Per-tenant API keys
   - Tenant-scoped permissions
   - Admin vs regular user roles

3. **Resource Management**
   - Per-tenant storage quotas
   - Per-tenant rate limits
   - Fair CPU/memory scheduling

4. **Configuration**
   - Per-tenant embedding model configuration
   - Per-tenant context assembly preferences
   - Per-tenant retention policies

### Non-Functional Requirements

1. **Performance**: < 10% overhead vs single-tenant
2. **Scalability**: Support 1000+ tenants per instance
3. **Availability**: Tenant failures should not affect others
4. **Observability**: Per-tenant metrics and logs

## Design Options

### Option A: Database-Level Isolation (Separate BadgerDB per Tenant)

Each tenant gets their own BadgerDB instance.

```
/data/
├── tenant-a/
│   └── badger/
├── tenant-b/
│   └── badger/
└── tenant-c/
    └── badger/
```

**Pros:**
- Complete data isolation
- Simple to implement
- Easy backup/restore per tenant
- No query complexity

**Cons:**
- Memory overhead (each BadgerDB has base memory cost)
- More file handles needed
- Harder to scale beyond hundreds of tenants
- Resource scheduling complexity

### Option B: Prefix-Based Isolation (Single BadgerDB with Tenant Prefixes)

Single BadgerDB with tenant ID prefixed to all keys.

```
Key structure:
  tenant:{tenant_id}:mem:{memory_id}
  tenant:{tenant_id}:ns:{namespace_id}
```

**Pros:**
- Single database to manage
- Efficient resource utilization
- Simpler operations
- Scales to thousands of tenants

**Cons:**
- Requires careful key design
- Risk of data leakage if prefixing is inconsistent
- Harder to backup/migrate single tenant
- No true isolation guarantees

### Option C: Hybrid Approach (Recommended)

Use prefix-based isolation with optional dedicated databases for premium tenants.

```go
type TenantStore interface {
    // Standard store operations with tenant context
    CreateMemory(ctx context.Context, tenantID string, input *CreateMemoryInput) (*Memory, error)
    GetMemory(ctx context.Context, tenantID string, id string) (*Memory, error)
    // ...
}

type MultiTenantStore struct {
    defaultStore *badger.Store      // Shared store for standard tenants
    dedicatedStores map[string]*badger.Store  // Premium tenant stores
}
```

**Pros:**
- Flexibility for different tenant tiers
- Efficient default path
- Premium isolation when needed
- Migration path from shared to dedicated

**Cons:**
- More complex implementation
- Need to manage store routing

## Proposed Architecture

### Tenant Management

```go
// Tenant represents a MAIA tenant
type Tenant struct {
    ID            string                 `json:"id"`
    Name          string                 `json:"name"`
    Plan          TenantPlan             `json:"plan"`  // free, standard, premium
    Config        TenantConfig           `json:"config"`
    Quotas        TenantQuotas           `json:"quotas"`
    Status        TenantStatus           `json:"status"`
    CreatedAt     time.Time              `json:"created_at"`
    Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

type TenantPlan string

const (
    TenantPlanFree     TenantPlan = "free"
    TenantPlanStandard TenantPlan = "standard"
    TenantPlanPremium  TenantPlan = "premium"
)

type TenantConfig struct {
    EmbeddingModel     string        `json:"embedding_model"`
    DefaultTokenBudget int           `json:"default_token_budget"`
    MaxNamespaces      int           `json:"max_namespaces"`
    RetentionDays      int           `json:"retention_days"`
    AllowedOrigins     []string      `json:"allowed_origins,omitempty"`
    DedicatedStorage   bool          `json:"dedicated_storage"`
}

type TenantQuotas struct {
    MaxMemories       int64  `json:"max_memories"`
    MaxStorageBytes   int64  `json:"max_storage_bytes"`
    RequestsPerMinute int    `json:"requests_per_minute"`
    RequestsPerDay    int64  `json:"requests_per_day"`
}
```

### Storage Layer Changes

```go
// TenantAwareStore wraps storage with tenant isolation
type TenantAwareStore struct {
    tenants  TenantManager
    shared   *badger.Store
    dedicated map[string]*badger.Store
    mu       sync.RWMutex
}

func (s *TenantAwareStore) getStore(tenantID string) (*badger.Store, error) {
    tenant, err := s.tenants.Get(tenantID)
    if err != nil {
        return nil, err
    }

    if tenant.Config.DedicatedStorage {
        s.mu.RLock()
        store, ok := s.dedicated[tenantID]
        s.mu.RUnlock()
        if ok {
            return store, nil
        }
        // Initialize dedicated store lazily
        return s.initDedicatedStore(tenantID)
    }

    return s.shared, nil
}

func (s *TenantAwareStore) buildKey(tenantID, prefix, id string) []byte {
    return []byte(fmt.Sprintf("t:%s:%s:%s", tenantID, prefix, id))
}
```

### API Layer Changes

```go
// Middleware to extract and validate tenant
func TenantMiddleware(tenants TenantManager) gin.HandlerFunc {
    return func(c *gin.Context) {
        // Extract tenant from API key or header
        tenantID := extractTenantID(c)
        if tenantID == "" {
            c.AbortWithStatusJSON(401, gin.H{"error": "tenant identification required"})
            return
        }

        tenant, err := tenants.Get(tenantID)
        if err != nil {
            c.AbortWithStatusJSON(401, gin.H{"error": "invalid tenant"})
            return
        }

        // Check tenant status
        if tenant.Status != TenantStatusActive {
            c.AbortWithStatusJSON(403, gin.H{"error": "tenant suspended"})
            return
        }

        // Set tenant in context
        c.Set("tenant", tenant)
        c.Set("tenant_id", tenantID)
        c.Next()
    }
}
```

### Quota Enforcement

```go
// QuotaEnforcer tracks and enforces tenant quotas
type QuotaEnforcer struct {
    store   QuotaStore
    metrics QuotaMetrics
}

func (e *QuotaEnforcer) CheckMemoryQuota(ctx context.Context, tenantID string) error {
    tenant, err := e.store.GetTenant(ctx, tenantID)
    if err != nil {
        return err
    }

    current, err := e.store.CountMemories(ctx, tenantID)
    if err != nil {
        return err
    }

    if current >= tenant.Quotas.MaxMemories {
        e.metrics.QuotaExceeded(tenantID, "memories")
        return ErrQuotaExceeded{
            Tenant:   tenantID,
            Resource: "memories",
            Limit:    tenant.Quotas.MaxMemories,
            Current:  current,
        }
    }

    return nil
}
```

### Metrics Per Tenant

```go
// Per-tenant Prometheus metrics
var (
    tenantMemoriesTotal = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "maia_tenant_memories_total",
            Help: "Total memories per tenant",
        },
        []string{"tenant_id", "plan"},
    )

    tenantRequestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "maia_tenant_requests_total",
            Help: "Total requests per tenant",
        },
        []string{"tenant_id", "method", "status"},
    )

    tenantStorageBytes = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "maia_tenant_storage_bytes",
            Help: "Storage usage per tenant",
        },
        []string{"tenant_id"},
    )
)
```

## Migration Strategy

### Phase 1: Add Tenant Layer (Non-Breaking)
1. Add `TenantManager` interface
2. Add default "system" tenant for existing deployments
3. Add tenant middleware (optional, behind flag)
4. All existing functionality works unchanged

### Phase 2: Enable Multi-Tenancy
1. Enable tenant middleware by default
2. Add tenant management API
3. Add quota tracking
4. Add per-tenant metrics

### Phase 3: Advanced Features
1. Dedicated storage for premium tenants
2. Cross-tenant admin operations
3. Tenant migration tools
4. Billing integration hooks

## API Extensions

### Tenant Management (Admin API)

```
POST   /admin/tenants                  # Create tenant
GET    /admin/tenants                  # List tenants
GET    /admin/tenants/{id}             # Get tenant
PUT    /admin/tenants/{id}             # Update tenant
DELETE /admin/tenants/{id}             # Delete tenant
GET    /admin/tenants/{id}/usage       # Get usage stats
POST   /admin/tenants/{id}/suspend     # Suspend tenant
POST   /admin/tenants/{id}/activate    # Activate tenant
```

### Tenant-Scoped Headers

```
X-MAIA-Tenant-ID: {tenant_id}     # Required for multi-tenant mode
X-MAIA-On-Behalf-Of: {user_id}    # Optional user identification
```

## Security Considerations

1. **API Key Isolation**: Each tenant has unique API keys
2. **Data Encryption**: Optional per-tenant encryption keys
3. **Audit Logging**: Track all tenant operations
4. **Network Isolation**: Optional VPC peering for premium tenants
5. **Compliance**: GDPR data deletion support per tenant

## Performance Considerations

1. **Key Prefix Performance**: BadgerDB prefix scans are efficient
2. **Cache Partitioning**: Separate LRU caches per tenant
3. **Connection Pooling**: Shared connection pools with fair scheduling
4. **Index Separation**: Per-tenant vector indices for isolation

## Estimated Effort

| Component | Complexity | Estimate |
|-----------|------------|----------|
| Tenant Manager | Medium | 2-3 days |
| Storage Changes | High | 3-4 days |
| API Middleware | Low | 1 day |
| Quota Enforcement | Medium | 2 days |
| Metrics | Low | 1 day |
| Admin API | Medium | 2 days |
| Testing | High | 3-4 days |
| Documentation | Low | 1 day |
| **Total** | | **15-18 days** |

## Decision

**Recommended**: Option C (Hybrid Approach)

This provides the best balance of:
- Efficient resource utilization for standard tenants
- True isolation for premium tenants
- Clear migration path
- Operational flexibility

## Open Questions

1. Should tenant data be encrypted at rest by default?
2. What's the minimum viable tenant management UI?
3. Should we support tenant self-service provisioning?
4. How do we handle tenant data export (GDPR compliance)?

## References

- [BadgerDB Documentation](https://dgraph.io/docs/badger/)
- [Multi-Tenant Data Architecture](https://docs.microsoft.com/en-us/azure/architecture/guide/multitenant/considerations/tenancy-models)
- [MAIA Product Vision](.agent/product.md)
