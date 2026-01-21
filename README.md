# MAIA - Memory AI Architecture

An AI-native distributed memory system that acts as an intelligent interceptor between applications and LLMs. MAIA solves the fundamental problem of context window limitations by intelligently managing what information goes into that window.

## Key Features

- **Distributed-first architecture** - Designed for multi-agent, multi-instance deployments
- **Position-aware context assembly** - Solves the "context rot" problem (20-30% accuracy variance)
- **Sub-200ms latency target** - Production-grade performance
- **Flexible namespace model** - Per-user, per-org, or custom hierarchies
- **Multiple integration patterns** - MCP Server, OpenAI-compatible proxy, native SDKs

## Quick Start

### Prerequisites

- Go 1.23 or later

### Installation

#### Download Pre-built Binaries

```bash
# macOS (Apple Silicon)
curl -LO https://github.com/ar4mirez/maia/releases/latest/download/maia-darwin-arm64.tar.gz
tar -xzf maia-darwin-arm64.tar.gz
sudo mv maia maiactl maia-mcp maia-migrate /usr/local/bin/

# macOS (Intel)
curl -LO https://github.com/ar4mirez/maia/releases/latest/download/maia-darwin-amd64.tar.gz
tar -xzf maia-darwin-amd64.tar.gz
sudo mv maia maiactl maia-mcp maia-migrate /usr/local/bin/

# Linux (x86_64)
curl -LO https://github.com/ar4mirez/maia/releases/latest/download/maia-linux-amd64.tar.gz
tar -xzf maia-linux-amd64.tar.gz
sudo mv maia maiactl maia-mcp maia-migrate /usr/local/bin/

# Linux (ARM64)
curl -LO https://github.com/ar4mirez/maia/releases/latest/download/maia-linux-arm64.tar.gz
tar -xzf maia-linux-arm64.tar.gz
sudo mv maia maiactl maia-mcp maia-migrate /usr/local/bin/
```

#### Build from Source

```bash
# Clone the repository
git clone https://github.com/ar4mirez/maia.git
cd maia

# Build all binaries
make build

# Or run directly
go run ./cmd/maia
```

### Configuration

Copy the example environment file and configure:

```bash
cp .env.example .env
# Edit .env with your settings
```

Key environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `MAIA_DATA_DIR` | `./data` | Storage directory |
| `MAIA_HTTP_PORT` | `8080` | HTTP API port |
| `MAIA_GRPC_PORT` | `9090` | gRPC API port |
| `MAIA_LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `MAIA_DEFAULT_NAMESPACE` | `default` | Default namespace for operations |

### Running

```bash
# Development mode
make dev

# Or with the built binary
./build/maia
```

## API Usage

### Health Check

```bash
curl http://localhost:8080/health
```

### Create a Memory

```bash
curl -X POST http://localhost:8080/v1/memories \
  -H "Content-Type: application/json" \
  -d '{
    "namespace": "default",
    "content": "The user prefers dark mode",
    "type": "semantic",
    "tags": ["preference", "ui"]
  }'
```

### Search Memories

```bash
curl -X POST http://localhost:8080/v1/memories/search \
  -H "Content-Type: application/json" \
  -d '{
    "namespace": "default",
    "tags": ["preference"]
  }'
```

### Create a Namespace

```bash
curl -X POST http://localhost:8080/v1/namespaces \
  -H "Content-Type: application/json" \
  -d '{
    "name": "user:john",
    "config": {
      "token_budget": 4000
    }
  }'
```

### Get Context (Assembled)

```bash
curl -X POST http://localhost:8080/v1/context \
  -H "Content-Type: application/json" \
  -d '{
    "query": "What are the user preferences?",
    "namespace": "default",
    "token_budget": 4000
  }'
```

## CLI Tool (maiactl)

MAIA includes a CLI tool for managing memories and namespaces.

```bash
# Build the CLI
go build -o maiactl ./cmd/maiactl

# Or install
go install ./cmd/maiactl
```

### Memory Operations

```bash
# Create a memory
maiactl memory create -n default -c "User prefers dark mode" -t semantic

# List memories
maiactl memory list -n default

# Search memories
maiactl memory search -q "preferences" -n default

# Get a specific memory
maiactl memory get <memory-id>

# Delete a memory
maiactl memory delete <memory-id>
```

### Namespace Operations

```bash
# Create a namespace
maiactl namespace create my-project --token-budget 8000

# List namespaces
maiactl namespace list

# Get namespace details
maiactl namespace get my-project
```

### Context Assembly

```bash
# Get assembled context for a query
maiactl context "What are the user's preferences?" -n default -b 2000
```

### Server Statistics

```bash
maiactl stats
```

### Global Flags

| Flag | Environment | Description |
|------|-------------|-------------|
| `--server` | `MAIA_URL` | Server URL (default: `http://localhost:8080`) |
| `--json` | | Output in JSON format |

## Authentication

MAIA supports API key authentication for production deployments.

### Auth Configuration

```yaml
# config.yaml
auth:
  enabled: true
  api_keys:
    - key: "your-api-key-here"
      namespaces: ["default", "user:*"]  # Allowed namespaces (supports wildcards)
    - key: "admin-key"
      namespaces: ["*"]  # Access to all namespaces
```

### Using API Keys

API keys can be provided via:

1. **Header**: `X-API-Key: your-api-key`
2. **Bearer Token**: `Authorization: Bearer your-api-key`
3. **Query Parameter**: `?api_key=your-api-key`

```bash
# Using header
curl -H "X-API-Key: your-api-key" http://localhost:8080/v1/memories

# Using bearer token
curl -H "Authorization: Bearer your-api-key" http://localhost:8080/v1/memories
```

## MCP Server Integration

MAIA can run as an MCP (Model Context Protocol) server for integration with Claude, Cursor, and other MCP-compatible tools.

### Running the MCP Server

```bash
go run ./cmd/mcp-server
```

### Available Tools

| Tool | Description |
|------|-------------|
| `remember` | Store a new memory |
| `recall` | Retrieve context based on a query |
| `forget` | Delete a memory |
| `list_memories` | List all memories in a namespace |
| `get_context` | Get assembled context with position-aware optimization |

### Available Resources

| Resource | Description |
|----------|-------------|
| `namespaces` | List all available namespaces |
| `memories` | List memories (namespace parameter) |
| `stats` | Server statistics |

### Available Prompts

| Prompt | Description |
|--------|-------------|
| `inject_context` | Inject MAIA context into a conversation |
| `summarize_memories` | Summarize memories in a namespace |
| `explore_memories` | Explore and understand stored memories |

## OpenAI-Compatible Proxy

MAIA provides an OpenAI-compatible proxy that automatically injects relevant context and extracts memories from responses.

### Proxy Configuration

```yaml
proxy:
  enabled: true
  backend: "https://api.openai.com"  # Or any OpenAI-compatible API
  default_namespace: "default"
  context_position: "system"  # system, first_user, or before_last
```

### Usage

Point your OpenAI client to MAIA:

```bash
curl -X POST http://localhost:8080/proxy/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H "X-MAIA-Namespace: default" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "What do you know about me?"}]
  }'
```

### Proxy Headers

| Header | Description |
|--------|-------------|
| `X-MAIA-Namespace` | Target namespace for memory operations |
| `X-MAIA-Skip-Memory` | Skip memory retrieval for this request |
| `X-MAIA-Skip-Extract` | Skip memory extraction from response |
| `X-MAIA-Token-Budget` | Override token budget for context |

## SDKs

### Go SDK

```go
import "github.com/ar4mirez/maia/pkg/maia"

client := maia.New(maia.WithBaseURL("http://localhost:8080"))

// Store a memory
mem, _ := client.Remember(ctx, "default", "User prefers dark mode")

// Recall context
context, _ := client.Recall(ctx, "user preferences",
    maia.WithNamespace("default"),
    maia.WithTokenBudget(2000),
)

// Forget a memory
client.Forget(ctx, mem.ID)
```

### TypeScript SDK

```typescript
import { MAIAClient } from '@maia/sdk';

const client = new MAIAClient({ baseUrl: 'http://localhost:8080' });

// Store a memory
const memory = await client.remember('default', 'User prefers dark mode');

// Recall context
const context = await client.recall('user preferences', {
  namespace: 'default',
  tokenBudget: 2000,
});

// Forget a memory
await client.forget(memory.id);
```

### Python SDK

```python
from maia import MAIAClient, AsyncMAIAClient

# Sync client
client = MAIAClient(base_url="http://localhost:8080")
memory = client.remember("default", "User prefers dark mode")
context = client.recall("user preferences", namespace="default")
client.forget(memory.id)

# Async client
async with AsyncMAIAClient() as client:
    memory = await client.remember("default", "User prefers dark mode")
    context = await client.recall("user preferences")
```

## Deployment

### Docker

```bash
# Build the image
docker build -t maia .

# Run
docker run -p 8080:8080 -v maia-data:/data ghcr.io/ar4mirez/maia:latest
```

### Docker Compose

```bash
# Basic deployment
docker-compose up -d

# With monitoring (Prometheus, Grafana, Jaeger)
docker-compose --profile monitoring up -d

# View logs
docker-compose logs -f maia
```

### Helm Chart

The recommended way to deploy MAIA on Kubernetes:

```bash
# Install from release
helm install maia https://github.com/ar4mirez/maia/releases/latest/download/maia-chart.tgz

# Or install from source
helm install maia ./deployments/helm/maia

# With custom values
helm install maia ./deployments/helm/maia -f my-values.yaml

# Upgrade
helm upgrade maia ./deployments/helm/maia
```

### Kubernetes (Kustomize)

Kubernetes manifests are available in `deployments/kubernetes/`:

```bash
# Apply all manifests
kubectl apply -k deployments/kubernetes/

# Or apply individually
kubectl apply -f deployments/kubernetes/namespace.yaml
kubectl apply -f deployments/kubernetes/configmap.yaml
kubectl apply -f deployments/kubernetes/secret.yaml
kubectl apply -f deployments/kubernetes/deployment.yaml
kubectl apply -f deployments/kubernetes/service.yaml
```

### Kubernetes CRDs

MAIA provides Custom Resource Definitions for cloud-native deployments:

```bash
# Install CRDs
kubectl apply -k deployments/kubernetes/crds/

# Create a MAIA instance
kubectl apply -f deployments/kubernetes/examples/basic-instance.yaml

# Create tenants
kubectl apply -f deployments/kubernetes/examples/tenant-example.yaml
```

See `deployments/kubernetes/examples/` for more CRD examples.

## Metrics & Monitoring

MAIA exposes Prometheus metrics at `/metrics`:

- `maia_http_requests_total` - Total HTTP requests
- `maia_http_request_duration_seconds` - Request latency histogram
- `maia_memory_operations_total` - Memory operations count
- `maia_search_operations_total` - Search operations count
- `maia_context_assembly_duration_seconds` - Context assembly latency
- `maia_tenant_memories_total` - Memories per tenant
- `maia_tenant_storage_bytes` - Storage usage per tenant
- `maia_tenant_quota_usage_ratio` - Quota usage ratios

### Prometheus Alerting

Prometheus alerting rules are available in `deployments/prometheus/alerts.yaml`.

### Grafana Dashboards

Pre-built Grafana dashboards are available in `deployments/grafana/dashboards/`.

## Backup & Restore

MAIA includes backup and restore utilities:

```bash
# Create a backup
./scripts/backup.sh ./data ./backups

# Create an encrypted backup
./scripts/backup.sh ./data ./backups --encrypt

# Restore from backup
./scripts/restore.sh ./backups/maia-backup-20260120-120000.tar.gz ./data

# List backups
ls -la ./backups/
```

Using Makefile:

```bash
# Backup
make backup

# Encrypted backup
make backup-encrypted

# Restore (interactive)
make restore BACKUP_FILE=./backups/maia-backup-20260120-120000.tar.gz
```

## Database Migrations

The `maia-migrate` tool handles database migrations:

```bash
# Run pending migrations
maia-migrate up

# Rollback last migration
maia-migrate down

# Check migration status
maia-migrate status

# Run to a specific version
maia-migrate goto 5
```

## Audit Logging

MAIA provides comprehensive audit logging for compliance and security:

- All memory operations (create, read, update, delete, search)
- Tenant and namespace management
- Authentication events
- Context assembly requests

Configure audit logging in `config.yaml`:

```yaml
audit:
  enabled: true
  level: write  # all, write, admin
  backend:
    type: file
    file_path: ./logs/audit.log
  redact_fields:
    - password
    - api_key
    - secret
```

## Architecture

```
User/Application
       │
       ├── MCP Server (Claude, Cursor integration)
       ├── OpenAI-compatible Proxy (drop-in replacement)
       └── Native SDKs (Go, TypeScript, Python)
              │
              ▼
       ┌─────────────────────────────────────┐
       │          MAIA Core Engine           │
       │  ┌─────────────────────────────────┐│
       │  │    Query Understanding Layer    ││
       │  └─────────────────────────────────┘│
       │                  │                  │
       │  ┌─────────────────────────────────┐│
       │  │     Memory Retrieval Layer      ││
       │  └─────────────────────────────────┘│
       │                  │                  │
       │  ┌─────────┬─────┴─────┬───────────┐│
       │  │Semantic │ Episodic  │  Working  ││
       │  │ Store   │  Store    │   Store   ││
       │  └─────────┴───────────┴───────────┘│
       │                  │                  │
       │  ┌─────────────────────────────────┐│
       │  │   Context Assembly Layer        ││
       │  └─────────────────────────────────┘│
       └─────────────────────────────────────┘
                         │
                         ▼
                   Target LLM
```

## Development

### Running Tests

```bash
make test
```

### Building

```bash
# Build all binaries
make build

# Build specific binary
make build-server
```

### Code Quality

```bash
# Format code
make fmt

# Run linters
make lint

# All checks
make check
```

## Project Structure

```
maia/
├── cmd/
│   ├── maia/           # Main server binary
│   ├── maiactl/        # CLI tool
│   ├── mcp-server/     # Standalone MCP server
│   └── migrate/        # Database migration tool
├── internal/
│   ├── audit/          # Audit logging
│   ├── config/         # Configuration management
│   ├── server/         # HTTP/gRPC server
│   ├── storage/        # Storage layer (BadgerDB)
│   ├── tenant/         # Multi-tenancy
│   └── ...
├── pkg/
│   ├── maia/           # Go SDK (public)
│   ├── mcp/            # MCP implementation
│   └── proxy/          # OpenAI proxy
├── deployments/
│   ├── helm/           # Helm chart
│   ├── kubernetes/     # Kubernetes manifests & CRDs
│   ├── prometheus/     # Prometheus alerting rules
│   └── grafana/        # Grafana dashboards
├── scripts/
│   ├── backup.sh       # Backup automation
│   └── restore.sh      # Restore automation
└── sdk/
    ├── typescript/     # TypeScript SDK
    └── python/         # Python SDK
```

## Roadmap

- [x] **Phase 1**: Foundation (storage, basic API)
- [x] **Phase 2**: Intelligence (query analysis, embeddings, vector search)
- [x] **Phase 3**: Context Assembly (position-aware optimization)
- [x] **Phase 4**: MCP Integration
- [x] **Phase 5**: CLI Tool (maiactl)
- [x] **Phase 6**: OpenAI-Compatible Proxy
- [x] **Phase 7**: SDKs (Go, TypeScript, Python)
- [x] **Phase 8**: Production Hardening (auth, metrics, tracing, Kubernetes)

## Contributing

Contributions are welcome! Please read the contributing guidelines before submitting PRs.

## License

[MIT License](LICENSE)
