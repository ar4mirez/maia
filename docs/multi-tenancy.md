# MAIA Multi-Tenancy Guide

MAIA supports multi-tenant deployments with tenant isolation, quota management, and per-tenant metrics.

---

## Overview

Multi-tenancy allows a single MAIA instance to serve multiple organizations or users with:

- **Isolation**: Each tenant's data is separated
- **Quotas**: Resource limits per tenant
- **Metrics**: Per-tenant usage tracking
- **Plans**: Different service tiers

---

## Enabling Multi-Tenancy

### Configuration

```yaml
# config.yaml
tenancy:
  enabled: true
  default_plan: free
  system_tenant_name: system
  enforce_quotas: true
```

### Environment Variables

```bash
export MAIA_TENANCY_ENABLED=true
export MAIA_TENANCY_DEFAULT_PLAN=free
export MAIA_TENANCY_ENFORCE_QUOTAS=true
```

---

## Tenant Plans

MAIA provides three built-in plans with default quotas:

| Plan | Memories | Storage | RPM | RPD |
|------|----------|---------|-----|-----|
| **Free** | 10,000 | 100 MB | 100 | 10,000 |
| **Standard** | 100,000 | 1 GB | 1,000 | 100,000 |
| **Premium** | 1,000,000 | 10 GB | 10,000 | 1,000,000 |

### Custom Quotas

Override defaults when creating tenants:

```bash
curl -X POST http://localhost:8080/admin/tenants \
  -H "Content-Type: application/json" \
  -H "X-API-Key: admin-key" \
  -d '{
    "name": "enterprise-customer",
    "plan": "premium",
    "quotas": {
      "max_memories": 5000000,
      "max_storage_bytes": 53687091200,
      "requests_per_minute": 50000,
      "requests_per_day": 5000000
    }
  }'
```

---

## Tenant Management

### Create Tenant

```bash
curl -X POST http://localhost:8080/admin/tenants \
  -H "Content-Type: application/json" \
  -H "X-API-Key: admin-key" \
  -d '{
    "name": "acme-corp",
    "display_name": "Acme Corporation",
    "plan": "standard",
    "config": {
      "default_namespace": "acme/default",
      "allowed_memory_types": ["semantic", "episodic"]
    }
  }'
```

**Response:**
```json
{
  "id": "tenant_01HQ5K6ABC",
  "name": "acme-corp",
  "display_name": "Acme Corporation",
  "plan": "standard",
  "status": "active",
  "quotas": {
    "max_memories": 100000,
    "max_storage_bytes": 1073741824,
    "requests_per_minute": 1000,
    "requests_per_day": 100000
  },
  "usage": {
    "memory_count": 0,
    "storage_bytes": 0,
    "requests_today": 0
  },
  "created_at": "2026-01-19T10:00:00Z"
}
```

### List Tenants

```bash
# All tenants
curl http://localhost:8080/admin/tenants \
  -H "X-API-Key: admin-key"

# Filter by status
curl "http://localhost:8080/admin/tenants?status=active" \
  -H "X-API-Key: admin-key"

# Filter by plan
curl "http://localhost:8080/admin/tenants?plan=premium" \
  -H "X-API-Key: admin-key"
```

### Get Tenant

```bash
curl http://localhost:8080/admin/tenants/tenant_01HQ5K6ABC \
  -H "X-API-Key: admin-key"
```

### Update Tenant

```bash
curl -X PUT http://localhost:8080/admin/tenants/tenant_01HQ5K6ABC \
  -H "Content-Type: application/json" \
  -H "X-API-Key: admin-key" \
  -d '{
    "plan": "premium",
    "quotas": {
      "max_memories": 500000
    }
  }'
```

### Delete Tenant

```bash
curl -X DELETE http://localhost:8080/admin/tenants/tenant_01HQ5K6ABC \
  -H "X-API-Key: admin-key"
```

> **Note**: Deleting a tenant removes all associated namespaces and memories.

### Suspend Tenant

```bash
curl -X POST http://localhost:8080/admin/tenants/tenant_01HQ5K6ABC/suspend \
  -H "Content-Type: application/json" \
  -H "X-API-Key: admin-key" \
  -d '{"reason": "Payment overdue"}'
```

Suspended tenants cannot perform any operations.

### Activate Tenant

```bash
curl -X POST http://localhost:8080/admin/tenants/tenant_01HQ5K6ABC/activate \
  -H "X-API-Key: admin-key"
```

---

## Tenant Usage

### Get Usage Statistics

```bash
curl http://localhost:8080/admin/tenants/tenant_01HQ5K6ABC/usage \
  -H "X-API-Key: admin-key"
```

**Response:**
```json
{
  "tenant_id": "tenant_01HQ5K6ABC",
  "memory_count": 45000,
  "storage_bytes": 47185920,
  "requests_today": 5420,
  "requests_this_minute": 12,
  "quota_memory_percent": 45.0,
  "quota_storage_percent": 4.4,
  "quota_rpm_percent": 1.2,
  "quota_rpd_percent": 5.4
}
```

### Usage Alerts

Monitor quota usage via metrics:

```promql
# Alert when memory usage exceeds 80%
maia_tenant_quota_usage_ratio{resource="memories"} > 0.8

# Alert when storage exceeds 90%
maia_tenant_quota_usage_ratio{resource="storage"} > 0.9

# Alert when RPM exceeds 70%
maia_tenant_quota_usage_ratio{resource="rpm"} > 0.7
```

---

## Tenant Isolation

### Namespace Isolation

Each tenant's namespaces are prefixed:

```
tenant-id/namespace-name
```

Example namespace structure:
```
acme-corp/default
acme-corp/project-alpha
acme-corp/project-beta

other-tenant/default
other-tenant/production
```

### API Key to Tenant Mapping

Configure which API keys belong to which tenants:

```yaml
security:
  authorization:
    enabled: true
    api_key_permissions:
      # Admin access to all tenants
      "sk-admin-xxx": ["*"]

      # Tenant-specific keys
      "sk-acme-xxx": ["acme-corp/*"]
      "sk-beta-xxx": ["beta-inc/*"]
```

### Request Flow

1. Client sends request with API key
2. MAIA validates API key
3. Tenant identified from API key mapping
4. Request scoped to tenant's namespaces
5. Quotas checked and enforced
6. Operation executed
7. Usage metrics updated

---

## Quota Enforcement

### Quota Types

| Quota | Enforcement | Error |
|-------|-------------|-------|
| `max_memories` | On memory create | 429 Quota Exceeded |
| `max_storage_bytes` | On memory create | 429 Quota Exceeded |
| `requests_per_minute` | Per request | 429 Rate Limited |
| `requests_per_day` | Per request | 429 Rate Limited |

### Quota Error Response

```json
{
  "error": {
    "code": "quota_exceeded",
    "message": "Memory quota exceeded",
    "details": {
      "tenant": "acme-corp",
      "quota": "max_memories",
      "limit": 100000,
      "current": 100000,
      "requested": 1
    }
  }
}
```

### Disabling Quotas

For testing or specific tenants:

```yaml
tenancy:
  enforce_quotas: false  # Global disable

# Or per-tenant
curl -X PUT http://localhost:8080/admin/tenants/test-tenant \
  -H "Content-Type: application/json" \
  -H "X-API-Key: admin-key" \
  -d '{
    "quotas": {
      "max_memories": 0,
      "max_storage_bytes": 0,
      "requests_per_minute": 0,
      "requests_per_day": 0
    }
  }'
```

> **Note**: Setting quota to `0` means unlimited.

---

## Per-Tenant Metrics

MAIA exposes Prometheus metrics per tenant:

### Available Metrics

```promql
# Total memories per tenant
maia_tenant_memories_total{tenant="acme-corp"}

# Storage usage per tenant
maia_tenant_storage_bytes{tenant="acme-corp"}

# Requests per tenant
maia_tenant_requests_total{tenant="acme-corp"}

# Quota usage ratio (0.0 - 1.0)
maia_tenant_quota_usage_ratio{tenant="acme-corp", resource="memories"}
maia_tenant_quota_usage_ratio{tenant="acme-corp", resource="storage"}
maia_tenant_quota_usage_ratio{tenant="acme-corp", resource="rpm"}
maia_tenant_quota_usage_ratio{tenant="acme-corp", resource="rpd"}

# Active tenants count
maia_tenants_active_total

# Tenant operations
maia_tenant_operations_total{operation="create"}
maia_tenant_operations_total{operation="suspend"}
```

### Grafana Dashboard

Example queries for a tenant dashboard:

```promql
# Memory usage trend
rate(maia_tenant_memories_total{tenant="$tenant"}[5m])

# Storage growth
increase(maia_tenant_storage_bytes{tenant="$tenant"}[1d])

# Request rate
rate(maia_tenant_requests_total{tenant="$tenant"}[1m])

# Quota usage heatmap
maia_tenant_quota_usage_ratio{tenant="$tenant"}
```

---

## System Tenant

The system tenant (`system`) is special:

- Cannot be deleted
- Cannot be suspended
- Unlimited quotas
- Used for backward compatibility

### Single-Tenant Mode

When multi-tenancy is disabled, all operations use the system tenant:

```yaml
tenancy:
  enabled: false  # All requests use system tenant
```

### Migration to Multi-Tenancy

1. Enable multi-tenancy:
   ```yaml
   tenancy:
     enabled: true
   ```

2. Existing data remains under system tenant

3. Create new tenants as needed

4. Optionally migrate data:
   ```bash
   # Export memories from system tenant
   maiactl -j memory list -n default > memories.json

   # Create new tenant
   curl -X POST http://localhost:8080/admin/tenants \
     -d '{"name": "new-tenant"}'

   # Import to new tenant namespace
   # (custom migration script needed)
   ```

---

## Best Practices

### Tenant Naming

Use clear, consistent naming:

```
organization-name    # e.g., acme-corp
user-uuid           # e.g., user-550e8400-e29b
project-name        # e.g., project-alpha
```

### Namespace Strategy

Structure namespaces hierarchically:

```
tenant/environment/project
├── acme-corp/prod/main
├── acme-corp/prod/analytics
├── acme-corp/staging/main
└── acme-corp/dev/main
```

### Quota Planning

| User Type | Recommended Plan | Notes |
|-----------|-----------------|-------|
| Free trial | Free | Auto-expire after 14 days |
| Individual | Free or Standard | Based on usage |
| Small team | Standard | 5-20 users |
| Enterprise | Premium | Custom quotas |

### Monitoring

Set up alerts for:

1. **Quota approaching limits** (> 80%)
2. **Sudden usage spikes**
3. **Suspended tenants with activity**
4. **High error rates per tenant**

### Security

1. **Rotate API keys** regularly
2. **Use separate keys** for different environments
3. **Audit tenant operations** via logs
4. **Implement IP allowlisting** for sensitive tenants

---

## API Reference

### Tenant Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/admin/tenants` | Create tenant |
| GET | `/admin/tenants` | List tenants |
| GET | `/admin/tenants/:id` | Get tenant |
| PUT | `/admin/tenants/:id` | Update tenant |
| DELETE | `/admin/tenants/:id` | Delete tenant |
| GET | `/admin/tenants/:id/usage` | Get usage |
| POST | `/admin/tenants/:id/suspend` | Suspend tenant |
| POST | `/admin/tenants/:id/activate` | Activate tenant |

### Tenant Object

```json
{
  "id": "tenant_01HQ5K6ABC",
  "name": "acme-corp",
  "display_name": "Acme Corporation",
  "plan": "standard",
  "status": "active",
  "config": {
    "default_namespace": "acme-corp/default",
    "allowed_memory_types": ["semantic", "episodic", "working"]
  },
  "quotas": {
    "max_memories": 100000,
    "max_storage_bytes": 1073741824,
    "requests_per_minute": 1000,
    "requests_per_day": 100000
  },
  "usage": {
    "memory_count": 45000,
    "storage_bytes": 47185920,
    "requests_today": 5420,
    "requests_this_minute": 12
  },
  "created_at": "2026-01-19T10:00:00Z",
  "updated_at": "2026-01-19T14:30:00Z"
}
```

### Status Values

| Status | Description |
|--------|-------------|
| `active` | Normal operation |
| `suspended` | Operations blocked |
| `pending_deletion` | Marked for deletion |

---

## Troubleshooting

### Tenant Not Found

```json
{
  "error": {
    "code": "not_found",
    "message": "Tenant not found"
  }
}
```

**Solution**: Verify tenant ID or name, check authorization.

### Quota Exceeded

```json
{
  "error": {
    "code": "quota_exceeded",
    "message": "Memory quota exceeded"
  }
}
```

**Solution**: Upgrade plan, delete unused memories, or increase quota.

### Unauthorized Access

```json
{
  "error": {
    "code": "forbidden",
    "message": "Access denied to namespace"
  }
}
```

**Solution**: Verify API key has access to the requested namespace.

### System Tenant Operations

```json
{
  "error": {
    "code": "bad_request",
    "message": "Cannot suspend system tenant"
  }
}
```

**Solution**: System tenant cannot be suspended or deleted.

---

## Related Documentation

- [Configuration](configuration.md) - Multi-tenancy configuration
- [API Reference](api-reference.md) - Admin API endpoints
- [Deployment](deployment.md) - Production deployment
- [Architecture](architecture.md) - System design
