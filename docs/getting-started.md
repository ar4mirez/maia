# Getting Started with MAIA

This guide walks you through installing MAIA, running your first server, and performing basic memory operations.

---

## Prerequisites

- **Go 1.22+** (for building from source)
- **Docker** (for containerized deployment)
- **curl** or similar HTTP client (for testing)

---

## Installation

### Option 1: Docker (Recommended)

The fastest way to get started:

```bash
# Pull and run MAIA
docker run -d \
  --name maia \
  -p 8080:8080 \
  -v maia-data:/data \
  ghcr.io/ar4mirez/maia:latest

# Verify it's running
curl http://localhost:8080/health
# Response: {"status":"ok"}
```

### Option 2: Download Pre-built Binaries

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

### Option 3: Build from Source

```bash
# Clone the repository
git clone https://github.com/ar4mirez/maia
cd maia

# Build all binaries
make build

# Or build individually
go build -o maia ./cmd/maia
go build -o maiactl ./cmd/maiactl
go build -o maia-mcp ./cmd/mcp-server
go build -o maia-migrate ./cmd/migrate

# Run the server
./build/maia
```

### Option 4: Go Install

```bash
# Install server and CLI
go install github.com/ar4mirez/maia/cmd/maia@latest
go install github.com/ar4mirez/maia/cmd/maiactl@latest
go install github.com/ar4mirez/maia/cmd/mcp-server@latest
go install github.com/ar4mirez/maia/cmd/migrate@latest

# Run
maia
```

---

## Verify Installation

Once MAIA is running, verify with:

```bash
# Health check
curl http://localhost:8080/health

# Expected response:
{"status":"ok"}

# Readiness check (includes storage verification)
curl http://localhost:8080/ready

# Expected response:
{"status":"ready","checks":{"storage":"ok"}}
```

---

## Quick Tutorial

### Step 1: Create Your First Memory

Store information that MAIA will remember:

```bash
curl -X POST http://localhost:8080/v1/memories \
  -H "Content-Type: application/json" \
  -d '{
    "namespace": "default",
    "content": "User prefers dark mode and compact layouts. They are a senior developer working on distributed systems.",
    "type": "semantic",
    "tags": ["preferences", "profile"]
  }'
```

Response:
```json
{
  "id": "mem_abc123...",
  "namespace": "default",
  "content": "User prefers dark mode and compact layouts...",
  "type": "semantic",
  "tags": ["preferences", "profile"],
  "created_at": "2026-01-19T10:00:00Z",
  "updated_at": "2026-01-19T10:00:00Z"
}
```

### Step 2: Add More Memories

Add different types of memories:

```bash
# Episodic memory (conversation/experience)
curl -X POST http://localhost:8080/v1/memories \
  -H "Content-Type: application/json" \
  -d '{
    "namespace": "default",
    "content": "Yesterday we discussed migrating the monolith to microservices. The user expressed concerns about team capacity.",
    "type": "episodic",
    "tags": ["architecture", "meeting"]
  }'

# Working memory (current session)
curl -X POST http://localhost:8080/v1/memories \
  -H "Content-Type: application/json" \
  -d '{
    "namespace": "default",
    "content": "Currently debugging a race condition in the user authentication module.",
    "type": "working",
    "tags": ["current-task"]
  }'
```

### Step 3: Search Memories

Find memories matching a query:

```bash
curl -X POST http://localhost:8080/v1/memories/search \
  -H "Content-Type: application/json" \
  -d '{
    "query": "user preferences display settings",
    "namespace": "default",
    "limit": 10
  }'
```

Response:
```json
{
  "results": [
    {
      "memory": {
        "id": "mem_abc123...",
        "content": "User prefers dark mode and compact layouts...",
        "type": "semantic"
      },
      "score": 0.87
    }
  ],
  "total": 1
}
```

### Step 4: Assemble Context

Get position-aware context optimized for LLM consumption:

```bash
curl -X POST http://localhost:8080/v1/context \
  -H "Content-Type: application/json" \
  -d '{
    "query": "What should I know about this user before helping them?",
    "namespace": "default",
    "token_budget": 2000,
    "include_scores": true
  }'
```

Response:
```json
{
  "content": "## Relevant Context\n\nUser prefers dark mode and compact layouts...\n\nYesterday we discussed migrating the monolith...\n\nCurrently debugging a race condition...",
  "memories": [
    {
      "id": "mem_abc123...",
      "content": "User prefers dark mode...",
      "type": "semantic",
      "score": 0.92,
      "position": "critical",
      "token_count": 45
    },
    {
      "id": "mem_def456...",
      "content": "Yesterday we discussed...",
      "type": "episodic",
      "score": 0.71,
      "position": "middle",
      "token_count": 52
    },
    {
      "id": "mem_ghi789...",
      "content": "Currently debugging...",
      "type": "working",
      "score": 0.45,
      "position": "recency",
      "token_count": 38
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
  }
}
```

---

## Using the CLI

The `maiactl` CLI provides a convenient interface for MAIA operations:

### Basic Usage

```bash
# Set server URL (optional, defaults to localhost:8080)
export MAIA_URL=http://localhost:8080

# Create a memory
maiactl memory create \
  -n default \
  -c "User prefers dark mode" \
  -t semantic \
  --tags "preferences,ui"

# List memories
maiactl memory list -n default

# Search memories
maiactl memory search -q "preferences" -n default

# Get context
maiactl context "What are the user's preferences?" -n default -b 2000

# Show server stats
maiactl stats
```

### Namespace Management

```bash
# Create a namespace
maiactl namespace create my-project --token-budget 8000

# List namespaces
maiactl namespace list

# Delete a namespace
maiactl namespace delete my-project
```

---

## Using SDKs

### Go SDK

```go
package main

import (
    "context"
    "fmt"
    "github.com/ar4mirez/maia/pkg/maia"
)

func main() {
    ctx := context.Background()

    // Create client
    client := maia.New(maia.WithBaseURL("http://localhost:8080"))

    // Store a memory
    mem, err := client.Remember(ctx, "default", "User prefers dark mode")
    if err != nil {
        panic(err)
    }
    fmt.Printf("Created memory: %s\n", mem.ID)

    // Recall context
    result, err := client.Recall(ctx, "user preferences",
        maia.WithNamespace("default"),
        maia.WithTokenBudget(2000),
    )
    if err != nil {
        panic(err)
    }
    fmt.Printf("Context: %s\n", result.Content)
}
```

### TypeScript SDK

```typescript
import { MAIAClient } from '@maia/sdk';

const client = new MAIAClient({
  baseUrl: 'http://localhost:8080',
});

// Store a memory
const memory = await client.remember('default', 'User prefers dark mode');
console.log('Created:', memory.id);

// Recall context
const context = await client.recall('user preferences', {
  namespace: 'default',
  tokenBudget: 2000,
});
console.log('Context:', context.content);
```

### Python SDK

```python
from maia import MAIAClient

client = MAIAClient(base_url="http://localhost:8080")

# Store a memory
memory = client.remember("default", "User prefers dark mode")
print(f"Created: {memory.id}")

# Recall context
context = client.recall("user preferences", namespace="default", token_budget=2000)
print(f"Context: {context.content}")
```

---

## Configuration

MAIA can be configured via environment variables or a config file:

### Environment Variables

```bash
# Server settings
export MAIA_HTTP_PORT=8080
export MAIA_DATA_DIR=./data
export MAIA_LOG_LEVEL=info

# Memory defaults
export MAIA_DEFAULT_NAMESPACE=default
export MAIA_DEFAULT_TOKEN_BUDGET=4000

# Security (optional)
export MAIA_API_KEY=your-api-key
export MAIA_ENABLE_TLS=false
```

### Config File

Create `config.yaml`:

```yaml
server:
  http_port: 8080
  max_concurrent_requests: 100
  request_timeout: 30s

storage:
  data_dir: ./data
  sync_writes: false

memory:
  default_namespace: default
  default_token_budget: 4000

security:
  api_key: ""  # Set for authentication
  rate_limit_rps: 100
```

Run with config file:

```bash
maia --config config.yaml
```

---

## Next Steps

Now that you have MAIA running, explore these topics:

1. **[Architecture](architecture.md)** — Understand how MAIA works internally
2. **[API Reference](api-reference.md)** — Complete REST API documentation
3. **[MCP Integration](mcp-integration.md)** — Connect to Claude Desktop or Cursor
4. **[SDKs](sdks.md)** — Detailed SDK documentation
5. **[Deployment](deployment.md)** — Production deployment guide

---

## Troubleshooting

### Server won't start

```bash
# Check if port is in use
lsof -i :8080

# Check logs
MAIA_LOG_LEVEL=debug maia
```

### Memory operations fail

```bash
# Verify health
curl http://localhost:8080/health

# Check storage directory permissions
ls -la ./data
```

### Cannot connect to server

```bash
# Verify server is running
curl http://localhost:8080/health

# Check firewall rules (if accessing remotely)
# Verify CORS settings for browser clients
```

---

## Getting Help

- **GitHub Issues**: [Report bugs or request features](https://github.com/ar4mirez/maia/issues)
- **Discussions**: [Ask questions and share ideas](https://github.com/ar4mirez/maia/discussions)
