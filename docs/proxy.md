# MAIA OpenAI Proxy

MAIA provides an OpenAI-compatible proxy that automatically injects context and extracts memories from conversations.

---

## Overview

The proxy acts as a drop-in replacement for the OpenAI API, adding intelligent context management:

- **Automatic context injection** — Relevant memories added to requests
- **Memory extraction** — Learn from assistant responses
- **Full streaming support** — SSE streaming preserved
- **OpenAI API compatibility** — Works with existing clients

---

## Quick Start

Point your OpenAI client to MAIA:

```bash
# Standard OpenAI request
curl -X POST http://localhost:8080/proxy/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H "X-MAIA-Namespace: default" \
  -d '{
    "model": "gpt-4",
    "messages": [
      {"role": "user", "content": "What do you know about me?"}
    ]
  }'
```

MAIA will:
1. Retrieve relevant memories from the `default` namespace
2. Inject context into the request
3. Forward to OpenAI
4. Return the response
5. Extract any new information to remember

---

## Configuration

### Basic Setup

```yaml
proxy:
  backend: "https://api.openai.com"
  backend_api_key: "${OPENAI_API_KEY}"
  auto_remember: true
  auto_context: true
  context_position: "system"
  token_budget: 4000
  timeout: 60s
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MAIA_PROXY_BACKEND` | | Backend LLM API endpoint |
| `MAIA_PROXY_BACKEND_API_KEY` | | Backend API key |
| `MAIA_PROXY_AUTO_REMEMBER` | `true` | Auto-extract memories |
| `MAIA_PROXY_AUTO_CONTEXT` | `true` | Auto-inject context |
| `MAIA_PROXY_CONTEXT_POSITION` | `system` | Context injection position |
| `MAIA_PROXY_TOKEN_BUDGET` | `4000` | Default token budget |
| `MAIA_PROXY_TIMEOUT` | `60s` | Backend request timeout |

---

## Request Headers

Control proxy behavior with headers:

| Header | Description | Example |
|--------|-------------|---------|
| `X-MAIA-Namespace` | Target namespace | `X-MAIA-Namespace: user-123` |
| `X-MAIA-Skip-Memory` | Skip context injection | `X-MAIA-Skip-Memory: true` |
| `X-MAIA-Skip-Extract` | Skip memory extraction | `X-MAIA-Skip-Extract: true` |
| `X-MAIA-Token-Budget` | Override token budget | `X-MAIA-Token-Budget: 8000` |

---

## Context Position

Configure where MAIA injects context:

### System (Default)

Context added as a system message:

```json
{
  "messages": [
    {"role": "system", "content": "## Relevant Context\n\nUser prefers dark mode..."},
    {"role": "user", "content": "What settings should I configure?"}
  ]
}
```

### First User

Context prepended to first user message:

```json
{
  "messages": [
    {"role": "user", "content": "## Relevant Context\n\nUser prefers dark mode...\n\n---\n\nWhat settings should I configure?"}
  ]
}
```

### Before Last

Context inserted before the last user message:

```json
{
  "messages": [
    {"role": "user", "content": "I want to customize the UI"},
    {"role": "assistant", "content": "I can help with that!"},
    {"role": "system", "content": "## Relevant Context\n\nUser prefers dark mode..."},
    {"role": "user", "content": "What settings should I configure?"}
  ]
}
```

---

## SDK Integration

### Python (OpenAI SDK)

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8080/proxy/v1",
    api_key="your-openai-key",
    default_headers={
        "X-MAIA-Namespace": "default"
    }
)

response = client.chat.completions.create(
    model="gpt-4",
    messages=[
        {"role": "user", "content": "What do you know about me?"}
    ]
)

print(response.choices[0].message.content)
```

### TypeScript (OpenAI SDK)

```typescript
import OpenAI from 'openai';

const client = new OpenAI({
  baseURL: 'http://localhost:8080/proxy/v1',
  apiKey: 'your-openai-key',
  defaultHeaders: {
    'X-MAIA-Namespace': 'default',
  },
});

const response = await client.chat.completions.create({
  model: 'gpt-4',
  messages: [
    { role: 'user', content: 'What do you know about me?' },
  ],
});

console.log(response.choices[0].message.content);
```

### Go (OpenAI SDK)

```go
package main

import (
    "context"
    "fmt"
    "github.com/sashabaranov/go-openai"
)

func main() {
    config := openai.DefaultConfig("your-openai-key")
    config.BaseURL = "http://localhost:8080/proxy/v1"

    client := openai.NewClientWithConfig(config)

    resp, err := client.CreateChatCompletion(
        context.Background(),
        openai.ChatCompletionRequest{
            Model: openai.GPT4,
            Messages: []openai.ChatCompletionMessage{
                {Role: openai.ChatMessageRoleUser, Content: "What do you know about me?"},
            },
        },
    )

    if err != nil {
        panic(err)
    }

    fmt.Println(resp.Choices[0].Message.Content)
}
```

---

## Streaming

The proxy fully supports SSE streaming:

```bash
curl -X POST http://localhost:8080/proxy/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H "X-MAIA-Namespace: default" \
  -d '{
    "model": "gpt-4",
    "messages": [
      {"role": "user", "content": "Tell me a story"}
    ],
    "stream": true
  }'
```

Memory extraction occurs after the stream completes.

---

## Memory Extraction

MAIA automatically extracts information from assistant responses using pattern matching:

### Extraction Patterns

- "I'll remember that..."
- "I'll note that..."
- "User prefers..."
- "Important: ..."
- Factual statements about the user

### Controlling Extraction

```bash
# Disable extraction for a request
curl -X POST http://localhost:8080/proxy/v1/chat/completions \
  -H "X-MAIA-Skip-Extract: true" \
  -d '...'
```

Or disable globally:

```yaml
proxy:
  auto_remember: false
```

---

## Rate Limiting

The proxy includes built-in rate limiting:

```yaml
security:
  rate_limit_rps: 100    # Requests per second
  rate_limit_burst: 150  # Burst capacity
```

Rate limit responses return HTTP 429:

```json
{
  "error": {
    "code": "rate_limited",
    "message": "Rate limit exceeded"
  }
}
```

---

## Error Handling

The proxy handles backend errors gracefully:

### Backend Timeout

```json
{
  "error": {
    "code": "backend_timeout",
    "message": "Backend request timed out"
  }
}
```

### Backend Error

```json
{
  "error": {
    "code": "backend_error",
    "message": "Backend returned error: ..."
  }
}
```

### Context Errors

Context injection failures are logged but don't fail the request. The request proceeds without injected context.

---

## Backend Options

The proxy works with any OpenAI-compatible API:

### OpenAI

```yaml
proxy:
  backend: "https://api.openai.com"
  backend_api_key: "${OPENAI_API_KEY}"
```

### Azure OpenAI

```yaml
proxy:
  backend: "https://your-resource.openai.azure.com"
  backend_api_key: "${AZURE_OPENAI_KEY}"
```

### Local Models (Ollama)

```yaml
proxy:
  backend: "http://localhost:11434/v1"
  backend_api_key: ""  # Not required for Ollama
```

### Other Providers

Any OpenAI-compatible endpoint:

```yaml
proxy:
  backend: "https://api.together.xyz/v1"
  backend_api_key: "${TOGETHER_API_KEY}"
```

---

## Monitoring

The proxy exposes Prometheus metrics:

| Metric | Description |
|--------|-------------|
| `maia_proxy_requests_total` | Total proxy requests |
| `maia_proxy_request_duration_seconds` | Request latency |
| `maia_proxy_backend_errors_total` | Backend error count |
| `maia_proxy_context_injections_total` | Context injections |
| `maia_proxy_memory_extractions_total` | Memory extractions |

---

## Troubleshooting

### No Context Injected

1. Check namespace exists: `curl http://localhost:8080/v1/namespaces/default`
2. Verify memories exist: `curl http://localhost:8080/v1/namespaces/default/memories`
3. Check `auto_context: true` in config
4. Ensure `X-MAIA-Skip-Memory` header is not set

### Memory Not Extracted

1. Check `auto_remember: true` in config
2. Ensure `X-MAIA-Skip-Extract` header is not set
3. Verify response contains extractable content

### Connection Refused

1. Check backend URL is correct
2. Verify backend is running
3. Check network connectivity
4. Review firewall rules

### Timeout Errors

Increase timeout for slow backends:

```yaml
proxy:
  timeout: 120s  # 2 minutes
```

---

## Related Documentation

- [Configuration](configuration.md) - Full configuration reference
- [Inference](inference.md) - Multi-provider inference
- [API Reference](api-reference.md) - Complete API documentation
