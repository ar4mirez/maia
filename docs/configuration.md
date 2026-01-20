# MAIA Configuration

Complete reference for all MAIA configuration options.

---

## Configuration Sources

MAIA loads configuration from multiple sources in order of precedence:

1. **Command-line flags** (highest priority)
2. **Environment variables**
3. **Config file** (`config.yaml`)
4. **Default values** (lowest priority)

---

## Quick Start

### Minimal Configuration

```bash
# Start with defaults (development)
maia

# Or with minimal environment variables
MAIA_HTTP_PORT=8080 MAIA_DATA_DIR=./data maia
```

### Production Configuration

```bash
# Set essential production variables
export MAIA_HTTP_PORT=8080
export MAIA_DATA_DIR=/var/lib/maia
export MAIA_API_KEY=your-secure-api-key
export MAIA_LOG_LEVEL=info
export MAIA_ENABLE_TLS=true
export MAIA_TLS_CERT=/etc/maia/tls.crt
export MAIA_TLS_KEY=/etc/maia/tls.key

maia
```

---

## Configuration File

Create `config.yaml`:

```yaml
# Server Configuration
server:
  http_port: 8080
  grpc_port: 9090
  max_concurrent_requests: 100
  request_timeout: 30s
  cors_origins:
    - "*"
  shutdown_grace_period: 10s
  enable_tracing: false

# Storage Configuration
storage:
  data_dir: ./data
  sync_writes: false
  compaction_interval: 1h
  max_table_size: 67108864  # 64MB
  encryption_key: ""  # 32-byte key for at-rest encryption

# Embedding Configuration
embedding:
  model: local
  dimensions: 384
  batch_size: 32
  # For OpenAI provider
  openai_api_key: ""
  openai_model: text-embedding-3-small
  # For Voyage provider
  voyage_api_key: ""
  voyage_model: voyage-02

# Memory Configuration
memory:
  default_namespace: default
  default_token_budget: 4000
  max_memory_size: 10000
  consolidation_interval: 24h
  auto_create_namespace: true

# Retrieval Configuration
retrieval:
  vector_weight: 0.35
  text_weight: 0.25
  recency_weight: 0.20
  frequency_weight: 0.10
  graph_weight: 0.10
  max_results: 100
  min_score: 0.0

# Context Assembly Configuration
context:
  zone_critical: 0.15
  zone_middle: 0.65
  zone_recency: 0.20
  critical_threshold: 0.7
  recency_window: 24h

# Proxy Configuration
proxy:
  backend: https://api.openai.com
  backend_api_key: ""
  auto_remember: true
  auto_context: true
  context_position: system  # system, first_user, before_last
  token_budget: 4000
  timeout: 60s

# Security Configuration
security:
  api_key: ""
  enable_tls: false
  tls_cert: ""
  tls_key: ""
  rate_limit_rps: 100
  rate_limit_burst: 150
  authorization:
    enabled: false
    default_policy: allow  # allow or deny
    api_key_permissions: {}

# Logging Configuration
logging:
  level: info  # debug, info, warn, error
  format: json  # json or text
  output: stdout  # stdout, stderr, or file path

# Tracing Configuration
tracing:
  enabled: false
  service_name: maia
  environment: development
  exporter_type: otlp-http  # otlp-http, otlp-grpc, noop
  endpoint: localhost:4318
  sample_rate: 1.0
  insecure: true

# Multi-Tenancy Configuration
tenancy:
  enabled: false
  default_plan: free
  system_tenant_name: system
  enforce_quotas: true
```

Run with config file:

```bash
maia --config /etc/maia/config.yaml
```

---

## Environment Variables

All configuration options can be set via environment variables using the `MAIA_` prefix:

### Server Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `MAIA_HTTP_PORT` | `8080` | HTTP API port |
| `MAIA_GRPC_PORT` | `9090` | gRPC API port |
| `MAIA_MAX_CONCURRENT_REQUESTS` | `100` | Max concurrent requests |
| `MAIA_REQUEST_TIMEOUT` | `30s` | Request timeout duration |
| `MAIA_CORS_ORIGINS` | `*` | Comma-separated CORS origins |
| `MAIA_SHUTDOWN_GRACE_PERIOD` | `10s` | Graceful shutdown timeout |
| `MAIA_ENABLE_TRACING` | `false` | Enable OpenTelemetry tracing |

### Storage Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `MAIA_DATA_DIR` | `./data` | Data directory path |
| `MAIA_SYNC_WRITES` | `false` | Sync writes to disk (slower, safer) |
| `MAIA_COMPACTION_INTERVAL` | `1h` | BadgerDB compaction interval |
| `MAIA_MAX_TABLE_SIZE` | `67108864` | Max LSM table size (bytes) |
| `MAIA_ENCRYPTION_KEY` | `` | 32-byte encryption key |

### Embedding Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `MAIA_EMBEDDING_MODEL` | `local` | `local`, `openai`, `voyage`, `mock` |
| `MAIA_EMBEDDING_DIMENSIONS` | `384` | Embedding dimensions |
| `MAIA_EMBEDDING_BATCH_SIZE` | `32` | Batch size for embedding generation |
| `MAIA_OPENAI_API_KEY` | `` | OpenAI API key |
| `MAIA_OPENAI_MODEL` | `text-embedding-3-small` | OpenAI embedding model |
| `MAIA_VOYAGE_API_KEY` | `` | Voyage AI API key |
| `MAIA_VOYAGE_MODEL` | `voyage-02` | Voyage embedding model |

### Memory Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `MAIA_DEFAULT_NAMESPACE` | `default` | Default namespace for operations |
| `MAIA_DEFAULT_TOKEN_BUDGET` | `4000` | Default context token budget |
| `MAIA_MAX_MEMORY_SIZE` | `10000` | Max content size (bytes) |
| `MAIA_CONSOLIDATION_INTERVAL` | `24h` | Memory consolidation interval |
| `MAIA_AUTO_CREATE_NAMESPACE` | `true` | Auto-create missing namespaces |

### Retrieval Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `MAIA_VECTOR_WEIGHT` | `0.35` | Vector similarity weight |
| `MAIA_TEXT_WEIGHT` | `0.25` | Full-text search weight |
| `MAIA_RECENCY_WEIGHT` | `0.20` | Recency scoring weight |
| `MAIA_FREQUENCY_WEIGHT` | `0.10` | Access frequency weight |
| `MAIA_GRAPH_WEIGHT` | `0.10` | Graph connectivity weight |
| `MAIA_MAX_RESULTS` | `100` | Max search results |
| `MAIA_MIN_SCORE` | `0.0` | Minimum relevance score |

### Context Assembly Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `MAIA_ZONE_CRITICAL` | `0.15` | Critical zone allocation (15%) |
| `MAIA_ZONE_MIDDLE` | `0.65` | Middle zone allocation (65%) |
| `MAIA_ZONE_RECENCY` | `0.20` | Recency zone allocation (20%) |
| `MAIA_CRITICAL_THRESHOLD` | `0.7` | Score threshold for critical zone |
| `MAIA_RECENCY_WINDOW` | `24h` | Time window for recency scoring |

### Proxy Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `MAIA_PROXY_BACKEND` | `` | Backend LLM API endpoint |
| `MAIA_PROXY_BACKEND_API_KEY` | `` | Backend API key |
| `MAIA_PROXY_AUTO_REMEMBER` | `true` | Auto-extract memories from responses |
| `MAIA_PROXY_AUTO_CONTEXT` | `true` | Auto-inject context into requests |
| `MAIA_PROXY_CONTEXT_POSITION` | `system` | Context injection position |
| `MAIA_PROXY_TOKEN_BUDGET` | `4000` | Proxy context token budget |
| `MAIA_PROXY_TIMEOUT` | `60s` | Backend request timeout |

### Security Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `MAIA_API_KEY` | `` | API key for authentication |
| `MAIA_ENABLE_TLS` | `false` | Enable TLS/HTTPS |
| `MAIA_TLS_CERT` | `` | TLS certificate file path |
| `MAIA_TLS_KEY` | `` | TLS private key file path |
| `MAIA_RATE_LIMIT_RPS` | `100` | Rate limit requests per second |
| `MAIA_RATE_LIMIT_BURST` | `150` | Rate limit burst capacity |
| `MAIA_AUTH_ENABLED` | `false` | Enable namespace authorization |
| `MAIA_AUTH_DEFAULT_POLICY` | `allow` | Default auth policy |

### Logging Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `MAIA_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `MAIA_LOG_FORMAT` | `json` | `json` or `text` |
| `MAIA_LOG_OUTPUT` | `stdout` | Output destination |

### Tracing Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `MAIA_TRACING_ENABLED` | `false` | Enable OpenTelemetry tracing |
| `MAIA_TRACING_SERVICE_NAME` | `maia` | Service name for traces |
| `MAIA_TRACING_ENVIRONMENT` | `development` | Environment tag |
| `MAIA_TRACING_EXPORTER_TYPE` | `otlp-http` | Exporter type |
| `MAIA_TRACING_ENDPOINT` | `localhost:4318` | Collector endpoint |
| `MAIA_TRACING_SAMPLE_RATE` | `1.0` | Sampling rate (0.0-1.0) |
| `MAIA_TRACING_INSECURE` | `true` | Skip TLS verification |

### Multi-Tenancy Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `MAIA_TENANCY_ENABLED` | `false` | Enable multi-tenancy |
| `MAIA_TENANCY_DEFAULT_PLAN` | `free` | Default tenant plan |
| `MAIA_TENANCY_SYSTEM_TENANT` | `system` | System tenant name |
| `MAIA_TENANCY_ENFORCE_QUOTAS` | `true` | Enforce tenant quotas |

---

## Configuration Scenarios

### Development

```yaml
server:
  http_port: 8080

storage:
  data_dir: ./data

embedding:
  model: mock  # Fast, no external dependencies

logging:
  level: debug
  format: text

security:
  api_key: ""  # No authentication required
```

### Production (Single Node)

```yaml
server:
  http_port: 8080
  max_concurrent_requests: 200
  request_timeout: 30s

storage:
  data_dir: /var/lib/maia
  sync_writes: true
  encryption_key: ${MAIA_ENCRYPTION_KEY}

embedding:
  model: local
  batch_size: 64

security:
  api_key: ${MAIA_API_KEY}
  enable_tls: true
  tls_cert: /etc/maia/tls.crt
  tls_key: /etc/maia/tls.key
  rate_limit_rps: 100

logging:
  level: info
  format: json

tracing:
  enabled: true
  exporter_type: otlp-http
  endpoint: jaeger:4318
```

### High-Volume (Performance Optimized)

```yaml
server:
  max_concurrent_requests: 500
  request_timeout: 15s

storage:
  sync_writes: false
  max_table_size: 134217728  # 128MB
  compaction_interval: 30m

embedding:
  model: local
  batch_size: 128

retrieval:
  max_results: 50  # Limit results for speed

memory:
  default_token_budget: 2000  # Smaller context
```

### Multi-Tenant SaaS

```yaml
tenancy:
  enabled: true
  default_plan: free
  enforce_quotas: true

security:
  api_key: ""  # Per-tenant keys
  authorization:
    enabled: true
    default_policy: deny
    api_key_permissions:
      admin_key: ["*"]
      tenant_a_key: ["tenant-a/*"]
      tenant_b_key: ["tenant-b/*"]
```

### Claude Desktop / MCP Only

```yaml
server:
  http_port: 0  # Disable HTTP server

embedding:
  model: local

memory:
  default_namespace: claude
  default_token_budget: 8000  # Claude has larger context
```

### OpenAI Proxy Mode

```yaml
proxy:
  backend: https://api.openai.com
  backend_api_key: ${OPENAI_API_KEY}
  auto_remember: true
  auto_context: true
  context_position: system
  token_budget: 4000
  timeout: 120s

embedding:
  model: openai
  openai_api_key: ${OPENAI_API_KEY}
```

---

## Advanced Configuration

### Namespace-Level Overrides

Namespaces can override global settings:

```bash
# Create namespace with custom config
curl -X POST http://localhost:8080/v1/namespaces \
  -H "Content-Type: application/json" \
  -d '{
    "name": "high-priority",
    "config": {
      "token_budget": 10000,
      "custom_scoring": {
        "vector_weight": 0.5,
        "text_weight": 0.3,
        "recency_weight": 0.15,
        "frequency_weight": 0.05
      }
    }
  }'
```

### Authorization Mapping

Map API keys to namespace permissions:

```yaml
security:
  authorization:
    enabled: true
    default_policy: deny
    api_key_permissions:
      # Admin access to all namespaces
      "sk-admin-xxx": ["*"]

      # Team access (hierarchical)
      "sk-team-a-xxx": ["team-a", "team-a/*"]
      "sk-team-b-xxx": ["team-b", "team-b/*"]

      # Project-specific access
      "sk-project-xxx": ["team-a/project-alpha"]

      # Read-only (future feature)
      # "sk-readonly-xxx": ["public:read"]
```

### Custom Embedding Providers

For self-hosted embedding models:

```yaml
embedding:
  model: openai  # Use OpenAI-compatible endpoint
  openai_api_key: "not-needed"
  openai_base_url: http://localhost:11434/v1  # Ollama
  openai_model: nomic-embed-text
```

---

## Validation

MAIA validates configuration on startup. Common issues:

### Invalid Data Directory

```
Error: storage.data_dir: directory does not exist and cannot be created
```

**Fix**: Ensure the data directory path is valid and writable.

### Invalid Embedding Provider

```
Error: embedding.model: unknown provider "invalid"
```

**Fix**: Use `local`, `openai`, `voyage`, or `mock`.

### Missing API Keys

```
Error: embedding.openai_api_key: required when model is "openai"
```

**Fix**: Set the API key or switch to `local` embedding.

### Invalid Zone Allocation

```
Error: context zones must sum to 1.0, got 0.9
```

**Fix**: Ensure `zone_critical + zone_middle + zone_recency = 1.0`.

---

## Environment-Specific Files

MAIA supports environment-specific config files:

```bash
# Load base config + environment overlay
maia --config config.yaml --config config.production.yaml
```

Later files override earlier ones.

---

## Related Documentation

- [Getting Started](getting-started.md) - Installation and first steps
- [Deployment](deployment.md) - Production deployment guide
- [Multi-Tenancy](multi-tenancy.md) - Tenant configuration
