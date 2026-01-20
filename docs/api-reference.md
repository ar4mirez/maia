# MAIA API Reference

Complete REST API documentation for MAIA.

**Base URL**: `http://localhost:8080` (default)

---

## Authentication

MAIA supports API key authentication via three methods:

```bash
# Method 1: X-API-Key header
curl -H "X-API-Key: your-api-key" http://localhost:8080/v1/memories

# Method 2: Bearer token
curl -H "Authorization: Bearer your-api-key" http://localhost:8080/v1/memories

# Method 3: Query parameter
curl "http://localhost:8080/v1/memories?api_key=your-api-key"
```

> **Note**: Authentication is optional unless `security.api_key` is configured.

---

## Health & Status

### Health Check

```
GET /health
```

Liveness probe for container orchestration.

**Response** `200 OK`:
```json
{
  "status": "ok"
}
```

### Readiness Check

```
GET /ready
```

Readiness probe including dependency checks.

**Response** `200 OK`:
```json
{
  "status": "ready",
  "checks": {
    "storage": "ok"
  }
}
```

### Metrics

```
GET /metrics
```

Prometheus-compatible metrics endpoint.

**Response**: Prometheus text format

---

## Memory Operations

### Create Memory

```
POST /v1/memories
```

Store a new memory.

**Request Body**:
```json
{
  "namespace": "default",
  "content": "User prefers dark mode and compact layouts",
  "type": "semantic",
  "metadata": {
    "source": "user-settings",
    "confidence": 0.95
  },
  "tags": ["preferences", "ui"],
  "confidence": 0.9,
  "source": "user"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `namespace` | string | Yes | Target namespace |
| `content` | string | Yes | Memory content (max 10KB) |
| `type` | string | No | `semantic`, `episodic`, or `working` (default: `semantic`) |
| `metadata` | object | No | Custom key-value metadata |
| `tags` | array | No | Tags for filtering |
| `confidence` | float | No | Confidence score 0.0-1.0 (default: 1.0) |
| `source` | string | No | `user`, `extracted`, `inferred`, `imported` (default: `user`) |

**Response** `201 Created`:
```json
{
  "id": "mem_01HQ5K3...",
  "namespace": "default",
  "content": "User prefers dark mode and compact layouts",
  "type": "semantic",
  "embedding": [0.123, -0.456, ...],
  "metadata": {
    "source": "user-settings",
    "confidence": 0.95
  },
  "tags": ["preferences", "ui"],
  "confidence": 0.9,
  "source": "user",
  "created_at": "2026-01-19T10:00:00Z",
  "updated_at": "2026-01-19T10:00:00Z",
  "accessed_at": "2026-01-19T10:00:00Z",
  "access_count": 0
}
```

**Errors**:
- `400 Bad Request`: Invalid input (missing content, invalid type)
- `404 Not Found`: Namespace not found
- `500 Internal Server Error`: Storage failure

---

### Get Memory

```
GET /v1/memories/:id
```

Retrieve a memory by ID.

**Path Parameters**:
| Parameter | Description |
|-----------|-------------|
| `id` | Memory ID |

**Response** `200 OK`:
```json
{
  "id": "mem_01HQ5K3...",
  "namespace": "default",
  "content": "User prefers dark mode and compact layouts",
  "type": "semantic",
  "embedding": [0.123, -0.456, ...],
  "metadata": {},
  "tags": ["preferences"],
  "confidence": 0.9,
  "source": "user",
  "created_at": "2026-01-19T10:00:00Z",
  "updated_at": "2026-01-19T10:00:00Z",
  "accessed_at": "2026-01-19T10:05:00Z",
  "access_count": 3
}
```

**Errors**:
- `404 Not Found`: Memory not found

---

### Update Memory

```
PUT /v1/memories/:id
```

Update an existing memory.

**Path Parameters**:
| Parameter | Description |
|-----------|-------------|
| `id` | Memory ID |

**Request Body**:
```json
{
  "content": "User prefers dark mode, compact layouts, and monospace fonts",
  "metadata": {
    "updated_by": "user-profile-sync"
  },
  "tags": ["preferences", "ui", "fonts"],
  "confidence": 0.95
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `content` | string | No | New content (regenerates embedding) |
| `metadata` | object | No | Metadata updates (merged with existing) |
| `tags` | array | No | Replace tags |
| `confidence` | float | No | New confidence score |

**Response** `200 OK`: Updated memory object

**Errors**:
- `400 Bad Request`: Invalid input
- `404 Not Found`: Memory not found

---

### Delete Memory

```
DELETE /v1/memories/:id
```

Delete a memory.

**Path Parameters**:
| Parameter | Description |
|-----------|-------------|
| `id` | Memory ID |

**Response** `204 No Content`

**Errors**:
- `404 Not Found`: Memory not found

---

### Search Memories

```
POST /v1/memories/search
```

Search memories with filters.

**Request Body**:
```json
{
  "query": "user preferences display settings",
  "namespace": "default",
  "types": ["semantic", "episodic"],
  "tags": ["preferences"],
  "source": "user",
  "min_confidence": 0.5,
  "time_range": {
    "start": "2026-01-01T00:00:00Z",
    "end": "2026-01-31T23:59:59Z"
  },
  "limit": 20,
  "offset": 0
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `query` | string | No | Search query (uses vector + fulltext) |
| `namespace` | string | No | Filter by namespace |
| `types` | array | No | Filter by memory types |
| `tags` | array | No | Filter by tags (AND logic) |
| `source` | string | No | Filter by source |
| `min_confidence` | float | No | Minimum confidence threshold |
| `time_range.start` | string | No | Start of time range (ISO 8601) |
| `time_range.end` | string | No | End of time range (ISO 8601) |
| `limit` | int | No | Max results (default: 100, max: 1000) |
| `offset` | int | No | Pagination offset |

**Response** `200 OK`:
```json
{
  "results": [
    {
      "memory": {
        "id": "mem_01HQ5K3...",
        "namespace": "default",
        "content": "User prefers dark mode...",
        "type": "semantic",
        "tags": ["preferences"],
        "confidence": 0.9,
        "created_at": "2026-01-19T10:00:00Z"
      },
      "score": 0.87
    }
  ],
  "total": 15,
  "limit": 20,
  "offset": 0
}
```

---

## Namespace Operations

### Create Namespace

```
POST /v1/namespaces
```

Create a new namespace.

**Request Body**:
```json
{
  "name": "project-alpha",
  "parent": "team-a",
  "template": "team-a",
  "config": {
    "token_budget": 8000,
    "max_memories": 50000,
    "retention_days": 90,
    "allowed_types": ["semantic", "episodic", "working"],
    "inherit_from_parent": true,
    "custom_scoring": {
      "vector_weight": 0.4,
      "text_weight": 0.3
    }
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique namespace name |
| `parent` | string | No | Parent namespace (hierarchical) |
| `template` | string | No | Copy config from existing namespace |
| `config.token_budget` | int | No | Default token budget (default: 4000) |
| `config.max_memories` | int | No | Maximum memories allowed |
| `config.retention_days` | int | No | Auto-delete after N days (0 = never) |
| `config.allowed_types` | array | No | Restrict memory types |
| `config.inherit_from_parent` | bool | No | Inherit parent config |
| `config.custom_scoring` | object | No | Override retrieval weights |

**Response** `201 Created`:
```json
{
  "id": "ns_01HQ5K4...",
  "name": "project-alpha",
  "parent": "team-a",
  "config": {
    "token_budget": 8000,
    "max_memories": 50000,
    "retention_days": 90
  },
  "stats": {
    "memory_count": 0,
    "total_tokens": 0
  },
  "created_at": "2026-01-19T10:00:00Z",
  "updated_at": "2026-01-19T10:00:00Z"
}
```

**Errors**:
- `400 Bad Request`: Invalid name or config
- `409 Conflict`: Namespace already exists

---

### List Namespaces

```
GET /v1/namespaces
```

List all namespaces.

**Query Parameters**:
| Parameter | Description |
|-----------|-------------|
| `limit` | Max results (default: 100) |
| `offset` | Pagination offset |
| `parent` | Filter by parent namespace |

**Response** `200 OK`:
```json
{
  "namespaces": [
    {
      "id": "ns_01HQ5K4...",
      "name": "default",
      "config": {
        "token_budget": 4000
      },
      "stats": {
        "memory_count": 150,
        "total_tokens": 45000
      },
      "created_at": "2026-01-19T10:00:00Z"
    }
  ],
  "total": 5,
  "limit": 100,
  "offset": 0
}
```

---

### Get Namespace

```
GET /v1/namespaces/:id
```

Get namespace details.

**Path Parameters**:
| Parameter | Description |
|-----------|-------------|
| `id` | Namespace ID or name |

**Response** `200 OK`: Namespace object

**Errors**:
- `404 Not Found`: Namespace not found

---

### Update Namespace

```
PUT /v1/namespaces/:id
```

Update namespace configuration.

**Path Parameters**:
| Parameter | Description |
|-----------|-------------|
| `id` | Namespace ID or name |

**Request Body**:
```json
{
  "config": {
    "token_budget": 10000,
    "retention_days": 180
  }
}
```

**Response** `200 OK`: Updated namespace object

---

### Delete Namespace

```
DELETE /v1/namespaces/:id
```

Delete a namespace and all its memories.

**Path Parameters**:
| Parameter | Description |
|-----------|-------------|
| `id` | Namespace ID or name |

**Query Parameters**:
| Parameter | Description |
|-----------|-------------|
| `force` | Skip confirmation for non-empty namespaces |

**Response** `204 No Content`

**Errors**:
- `400 Bad Request`: Namespace not empty (without `force`)
- `404 Not Found`: Namespace not found

---

### List Namespace Memories

```
GET /v1/namespaces/:id/memories
```

List all memories in a namespace.

**Path Parameters**:
| Parameter | Description |
|-----------|-------------|
| `id` | Namespace ID or name |

**Query Parameters**:
| Parameter | Description |
|-----------|-------------|
| `limit` | Max results (default: 100) |
| `offset` | Pagination offset |
| `type` | Filter by memory type |

**Response** `200 OK`:
```json
{
  "memories": [...],
  "total": 150,
  "limit": 100,
  "offset": 0
}
```

---

## Context Assembly

### Get Context

```
POST /v1/context
```

Assemble position-aware context from relevant memories.

**Request Body**:
```json
{
  "query": "What should I know about this user?",
  "namespace": "default",
  "token_budget": 2000,
  "system_prompt": "You are a helpful assistant.",
  "include_scores": true,
  "min_score": 0.3,
  "memory_types": ["semantic", "episodic"],
  "zone_allocation": {
    "critical": 0.15,
    "middle": 0.65,
    "recency": 0.20
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `query` | string | Yes | Query to retrieve relevant context |
| `namespace` | string | No | Target namespace (default: "default") |
| `token_budget` | int | No | Token limit (default: 4000) |
| `system_prompt` | string | No | System prompt to include |
| `include_scores` | bool | No | Include relevance scores |
| `min_score` | float | No | Minimum relevance threshold |
| `memory_types` | array | No | Filter memory types |
| `zone_allocation` | object | No | Custom zone percentages |

**Response** `200 OK`:
```json
{
  "content": "## Relevant Context\n\nUser prefers dark mode and compact layouts...\n\nYesterday we discussed migrating the monolith...",
  "memories": [
    {
      "id": "mem_01HQ5K3...",
      "content": "User prefers dark mode and compact layouts",
      "type": "semantic",
      "score": 0.92,
      "position": "critical",
      "token_count": 45,
      "truncated": false
    },
    {
      "id": "mem_02HQ5K4...",
      "content": "Yesterday we discussed migrating the monolith...",
      "type": "episodic",
      "score": 0.71,
      "position": "middle",
      "token_count": 52,
      "truncated": false
    },
    {
      "id": "mem_03HQ5K5...",
      "content": "Currently debugging a race condition...",
      "type": "working",
      "score": 0.45,
      "position": "recency",
      "token_count": 38,
      "truncated": false
    }
  ],
  "token_count": 135,
  "token_budget": 2000,
  "truncated": false,
  "zone_stats": {
    "critical_used": 45,
    "critical_budget": 300,
    "middle_used": 52,
    "middle_budget": 1300,
    "recency_used": 38,
    "recency_budget": 400
  },
  "query_analysis": {
    "intent": "exploratory",
    "context_type": "semantic",
    "temporal_scope": "all"
  },
  "query_time": "45.2ms"
}
```

**Position Values**:
| Position | Description | Budget |
|----------|-------------|--------|
| `critical` | High-relevance facts (score ≥ 0.7) | 15% |
| `middle` | Supporting context | 65% |
| `recency` | Working memory & recent items | 20% |

---

## Statistics

### Get Statistics

```
GET /v1/stats
```

Get server and storage statistics.

**Response** `200 OK`:
```json
{
  "storage": {
    "memory_count": 15000,
    "namespace_count": 12,
    "total_size_bytes": 15728640,
    "index_size_bytes": 5242880
  },
  "server": {
    "uptime_seconds": 86400,
    "requests_total": 150000,
    "requests_per_second": 1.74
  },
  "performance": {
    "avg_write_latency_ms": 1.2,
    "avg_read_latency_ms": 0.5,
    "avg_search_latency_ms": 5.3,
    "avg_context_latency_ms": 45.2
  }
}
```

---

## Admin API (Multi-Tenancy)

Admin endpoints require appropriate authorization.

### Create Tenant

```
POST /admin/tenants
```

Create a new tenant.

**Request Body**:
```json
{
  "name": "acme-corp",
  "display_name": "Acme Corporation",
  "plan": "standard",
  "config": {
    "custom_setting": "value"
  },
  "quotas": {
    "max_memories": 100000,
    "max_storage_bytes": 1073741824,
    "requests_per_minute": 1000,
    "requests_per_day": 100000
  }
}
```

**Response** `201 Created`:
```json
{
  "id": "tenant_01HQ5K6...",
  "name": "acme-corp",
  "display_name": "Acme Corporation",
  "plan": "standard",
  "status": "active",
  "quotas": {...},
  "usage": {
    "memory_count": 0,
    "storage_bytes": 0,
    "requests_today": 0
  },
  "created_at": "2026-01-19T10:00:00Z"
}
```

### List Tenants

```
GET /admin/tenants
```

**Query Parameters**:
| Parameter | Description |
|-----------|-------------|
| `status` | Filter by status (`active`, `suspended`) |
| `plan` | Filter by plan (`free`, `standard`, `premium`) |
| `limit` | Max results |
| `offset` | Pagination offset |

### Get Tenant

```
GET /admin/tenants/:id
```

### Update Tenant

```
PUT /admin/tenants/:id
```

### Delete Tenant

```
DELETE /admin/tenants/:id
```

### Get Tenant Usage

```
GET /admin/tenants/:id/usage
```

**Response** `200 OK`:
```json
{
  "tenant_id": "tenant_01HQ5K6...",
  "memory_count": 5000,
  "storage_bytes": 52428800,
  "requests_today": 1500,
  "requests_this_minute": 25,
  "quota_memory_percent": 5.0,
  "quota_storage_percent": 4.9,
  "quota_rpm_percent": 2.5
}
```

### Suspend Tenant

```
POST /admin/tenants/:id/suspend
```

**Request Body**:
```json
{
  "reason": "Payment overdue"
}
```

### Activate Tenant

```
POST /admin/tenants/:id/activate
```

---

## OpenAI-Compatible Proxy

### Chat Completions

```
POST /proxy/v1/chat/completions
```

OpenAI-compatible chat completions with automatic context injection.

**Headers**:
| Header | Description | Default |
|--------|-------------|---------|
| `X-MAIA-Namespace` | Target namespace | `default` |
| `X-MAIA-Skip-Memory` | Skip context injection | `false` |
| `X-MAIA-Skip-Extract` | Skip memory extraction | `false` |
| `X-MAIA-Token-Budget` | Context token budget | `4000` |

**Request Body**: Standard OpenAI chat completions format

```json
{
  "model": "gpt-4",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "What are my preferences?"}
  ],
  "stream": true,
  "temperature": 0.7
}
```

**Response**: Standard OpenAI chat completions response (streaming or non-streaming)

**Context Injection Positions**:
- `system`: Append to system message
- `first_user`: Prepend to first user message
- `before_last`: Insert before last user message

---

## Error Responses

All errors follow this format:

```json
{
  "error": {
    "code": "not_found",
    "message": "Memory not found",
    "details": {
      "memory_id": "mem_invalid"
    }
  }
}
```

**Common Error Codes**:

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `bad_request` | 400 | Invalid request body or parameters |
| `unauthorized` | 401 | Missing or invalid API key |
| `forbidden` | 403 | Insufficient permissions |
| `not_found` | 404 | Resource not found |
| `conflict` | 409 | Resource already exists |
| `rate_limited` | 429 | Rate limit exceeded |
| `internal_error` | 500 | Server error |

---

## Rate Limiting

When rate limited, responses include:

```
HTTP/1.1 429 Too Many Requests
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1706097600
Retry-After: 60
```

```json
{
  "error": {
    "code": "rate_limited",
    "message": "Rate limit exceeded",
    "details": {
      "limit": 100,
      "reset_at": "2026-01-19T10:00:00Z"
    }
  }
}
```

---

## Pagination

List endpoints support pagination:

```
GET /v1/memories?limit=50&offset=100
```

**Response includes**:
```json
{
  "items": [...],
  "total": 500,
  "limit": 50,
  "offset": 100,
  "has_more": true
}
```

---

## Related Documentation

- [Getting Started](getting-started.md) - Quick start guide
- [SDKs](sdks.md) - Client library documentation
- [Configuration](configuration.md) - Server configuration
