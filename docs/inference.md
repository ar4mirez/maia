# MAIA Inference Integration

MAIA provides integrated inference capabilities with multi-provider routing, automatic failover, and response caching.

---

## Overview

The inference system allows MAIA to route LLM requests to multiple providers based on model patterns. This enables:

- **Multi-provider routing** — Route requests to different providers based on model name
- **Automatic failover** — Switch to backup providers when primary is unhealthy
- **Response caching** — Cache responses to reduce latency and costs
- **Automatic context injection** — MAIA memories automatically included in requests
- **Memory extraction** — Learn from LLM responses automatically

---

## Enabling Inference

Inference is opt-in. Enable it in your configuration:

```yaml
inference:
  enabled: true
  default_provider: "ollama"

  providers:
    ollama:
      type: "ollama"
      base_url: "http://localhost:11434/v1"
      timeout: 60s

    openrouter:
      type: "openrouter"
      base_url: "https://openrouter.ai/api/v1"
      api_key: "${OPENROUTER_API_KEY}"

    anthropic:
      type: "anthropic"
      base_url: "https://api.anthropic.com/v1"
      api_key: "${ANTHROPIC_API_KEY}"

  routing:
    model_mapping:
      "llama*": "ollama"
      "mistral*": "ollama"
      "claude*": "anthropic"
      "*": "openrouter"

  cache:
    enabled: true
    ttl: 24h
    max_entries: 1000
```

---

## Supported Providers

| Provider | Type | Description |
|----------|------|-------------|
| **Ollama** | `ollama` | Local models via Ollama |
| **OpenRouter** | `openrouter` | 100+ models via OpenRouter API |
| **Anthropic** | `anthropic` | Claude models directly |

### Ollama

Run local models using Ollama:

```yaml
providers:
  ollama:
    type: "ollama"
    base_url: "http://localhost:11434/v1"
    timeout: 120s  # Longer timeout for local models
    models:
      - "llama3.2"
      - "mistral"
      - "codellama"
```

### OpenRouter

Access 100+ models through a single API:

```yaml
providers:
  openrouter:
    type: "openrouter"
    base_url: "https://openrouter.ai/api/v1"
    api_key: "${OPENROUTER_API_KEY}"
    headers:
      HTTP-Referer: "https://your-app.com"
      X-Title: "Your App Name"
```

### Anthropic

Direct access to Claude models:

```yaml
providers:
  anthropic:
    type: "anthropic"
    base_url: "https://api.anthropic.com/v1"
    api_key: "${ANTHROPIC_API_KEY}"
    models:
      - "claude-3-5-sonnet-*"
      - "claude-3-opus-*"
```

---

## Model Routing

Route requests to different providers based on model name patterns:

```yaml
routing:
  model_mapping:
    "llama*": "ollama"        # llama3.2, llama2, etc.
    "mistral*": "ollama"      # mistral, mistral-7b, etc.
    "claude*": "anthropic"    # claude-3-5-sonnet, etc.
    "gpt*": "openrouter"      # gpt-4, gpt-4-turbo, etc.
    "*": "openrouter"         # Default fallback
```

**Pattern Matching:**
- `*` matches any characters
- Patterns are checked in order
- First match wins

---

## Making Inference Requests

### REST API

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "X-MAIA-Namespace: default" \
  -d '{
    "model": "llama3.2",
    "messages": [
      {"role": "user", "content": "What do you know about me?"}
    ]
  }'
```

### With Streaming

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "X-MAIA-Namespace: default" \
  -d '{
    "model": "llama3.2",
    "messages": [
      {"role": "user", "content": "Explain context rot in LLMs"}
    ],
    "stream": true
  }'
```

### MCP Tools

MAIA MCP server includes inference tools:

| Tool | Description |
|------|-------------|
| `maia_complete` | Non-streaming completion |
| `maia_stream` | Streaming completion |
| `maia_list_models` | List available models |

---

## Request Headers

| Header | Description |
|--------|-------------|
| `X-MAIA-Namespace` | Namespace for memory operations |
| `X-MAIA-Skip-Memory` | Skip memory context injection |
| `X-MAIA-Skip-Extract` | Skip memory extraction from response |
| `X-MAIA-Token-Budget` | Override token budget for context |

---

## Response Caching

Enable caching to reduce latency and costs for repeated queries:

```yaml
inference:
  cache:
    enabled: true
    ttl: 24h           # Cache entries expire after 24 hours
    max_entries: 1000  # Maximum cached responses
```

**Cache Behavior:**
- Only non-streaming requests are cached
- Cache key is a SHA-256 hash of model, messages, and parameters
- LRU eviction when max entries reached
- TTL expiration for stale entries

### Cache API

```bash
# Get cache statistics
curl http://localhost:8080/v1/inference/cache/stats

# Clear cache
curl -X POST http://localhost:8080/v1/inference/cache/clear
```

**Stats Response:**
```json
{
  "enabled": true,
  "hits": 150,
  "misses": 45,
  "evictions": 2,
  "size": 48,
  "last_access": "2026-01-20T17:30:00Z"
}
```

---

## Health Monitoring

Monitor provider health status:

```bash
# Get all providers' health
curl http://localhost:8080/v1/inference/health

# Check specific provider
curl -X POST http://localhost:8080/v1/inference/health/ollama
```

**Health Response:**
```json
{
  "enabled": true,
  "providers": {
    "ollama": {
      "status": "healthy",
      "last_check": "2026-01-20T10:30:00Z",
      "consecutive_errors": 0,
      "consecutive_ok": 5
    },
    "openrouter": {
      "status": "unhealthy",
      "last_check": "2026-01-20T10:29:45Z",
      "last_error": "connection refused",
      "consecutive_errors": 3,
      "consecutive_ok": 0
    }
  }
}
```

---

## Automatic Failover

When a provider becomes unhealthy, requests automatically route to healthy alternatives:

1. Primary provider checked for health
2. If unhealthy, check routing rules for alternatives
3. If model supported by alternative, route there
4. If no alternatives, return error

**Health Status Values:**
- `healthy` — Provider responding normally
- `unhealthy` — Provider failing health checks
- `unknown` — Never checked or status expired

---

## Context Integration

When inference is enabled, MAIA automatically:

1. **Injects Context** — Retrieves relevant memories and adds to request
2. **Extracts Memories** — Learns from assistant responses

### Context Position

Configure where context is injected:

```yaml
proxy:
  context_position: "system"  # system, first_user, before_last
```

| Position | Description |
|----------|-------------|
| `system` | Prepend to system message (default) |
| `first_user` | Prepend to first user message |
| `before_last` | Insert before last user message |

### Skip Context

Skip context operations for specific requests:

```bash
# Skip memory retrieval
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "X-MAIA-Skip-Memory: true" \
  -d '...'

# Skip memory extraction
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "X-MAIA-Skip-Extract: true" \
  -d '...'
```

---

## Configuration Reference

| Setting | Default | Description |
|---------|---------|-------------|
| `inference.enabled` | `false` | Enable inference system |
| `inference.default_provider` | `` | Default provider when no routing match |
| `inference.providers.<name>.type` | | Provider type (ollama, openrouter, anthropic) |
| `inference.providers.<name>.base_url` | | Provider API endpoint |
| `inference.providers.<name>.api_key` | | Provider API key |
| `inference.providers.<name>.timeout` | `60s` | Request timeout |
| `inference.routing.model_mapping` | | Model pattern to provider mapping |
| `inference.cache.enabled` | `false` | Enable response caching |
| `inference.cache.ttl` | `24h` | Cache entry TTL |
| `inference.cache.max_entries` | `1000` | Maximum cache entries |

---

## Related Documentation

- [Configuration](configuration.md) - Full configuration reference
- [MCP Integration](mcp-integration.md) - MCP tools including inference
- [API Reference](api-reference.md) - Complete API documentation
