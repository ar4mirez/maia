# Basic Usage Example

This example demonstrates the core MAIA functionality: creating memories, searching, and assembling context.

## Prerequisites

1. MAIA server running on `localhost:8080`
2. Go 1.22 or later

## Running MAIA

```bash
# From the project root
go run ./cmd/maia
```

## Examples

### Using curl

```bash
# Health check
curl http://localhost:8080/health

# Create a namespace
curl -X POST http://localhost:8080/v1/namespaces \
  -H "Content-Type: application/json" \
  -d '{"name": "example", "config": {"token_budget": 4000}}'

# Create memories
curl -X POST http://localhost:8080/v1/memories \
  -H "Content-Type: application/json" \
  -d '{
    "namespace": "example",
    "content": "The user prefers dark mode for all applications",
    "type": "semantic",
    "tags": ["preference", "ui"]
  }'

curl -X POST http://localhost:8080/v1/memories \
  -H "Content-Type: application/json" \
  -d '{
    "namespace": "example",
    "content": "User timezone is America/New_York",
    "type": "semantic",
    "tags": ["preference", "timezone"]
  }'

curl -X POST http://localhost:8080/v1/memories \
  -H "Content-Type: application/json" \
  -d '{
    "namespace": "example",
    "content": "Working on a React project called dashboard-v2",
    "type": "episodic",
    "tags": ["project", "context"]
  }'

# List memories
curl http://localhost:8080/v1/namespaces/example/memories

# Search memories by tag
curl -X POST http://localhost:8080/v1/memories/search \
  -H "Content-Type: application/json" \
  -d '{"namespace": "example", "tags": ["preference"]}'

# Get assembled context for a query
curl -X POST http://localhost:8080/v1/context \
  -H "Content-Type: application/json" \
  -d '{
    "query": "What are the user preferences?",
    "namespace": "example",
    "token_budget": 2000
  }'
```

### Using the Go SDK

```bash
cd examples/basic-usage
go run main.go
```

### Using maiactl CLI

```bash
# Create memories
maiactl memory create -n example -c "User prefers dark mode" -t semantic --tags preference,ui
maiactl memory create -n example -c "Timezone: America/New_York" -t semantic --tags preference

# List memories
maiactl memory list -n example

# Search
maiactl memory search -q "preferences" -n example

# Get context
maiactl context "What are the user preferences?" -n example -b 2000

# View stats
maiactl stats
```

## Understanding the Output

### Context Response

The context endpoint returns assembled context with zone information:

```json
{
  "context": "## Relevant Context\n\n- The user prefers dark mode...",
  "tokens_used": 150,
  "token_budget": 2000,
  "memories_used": 3,
  "zones": {
    "critical": { "tokens": 50, "memories": 1 },
    "middle": { "tokens": 80, "memories": 2 },
    "recency": { "tokens": 20, "memories": 0 }
  }
}
```

### Zone Allocation

MAIA uses position-aware context assembly to optimize LLM accuracy:

- **Critical Zone (15%)**: High-relevance memories (score >= 0.7)
- **Middle Zone (65%)**: Supporting context, decreasing relevance
- **Recency Zone (20%)**: Recent/working memories

This addresses "context rot" - the 20-30% accuracy variance based on information position in the context window.
