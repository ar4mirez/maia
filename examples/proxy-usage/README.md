# OpenAI Proxy Example

This example demonstrates using MAIA's OpenAI-compatible proxy to automatically inject memory context and extract memories from LLM responses.

## How It Works

```
┌─────────────┐    ┌───────────────────────┐    ┌────────────────┐
│ Application │───►│      MAIA Proxy       │───►│  OpenAI API    │
│             │◄───│ - Inject context      │◄───│  (or compat.)  │
│             │    │ - Extract memories    │    │                │
└─────────────┘    └───────────────────────┘    └────────────────┘
                            │
                            ▼
                   ┌──────────────────┐
                   │  MAIA Storage    │
                   │  - Namespaces    │
                   │  - Memories      │
                   └──────────────────┘
```

**Key Features:**

1. **Automatic Context Injection**: Relevant memories are prepended to the conversation
2. **Memory Extraction**: Key information from assistant responses is automatically stored
3. **Streaming Support**: Full SSE streaming compatibility
4. **Drop-in Replacement**: Works with any OpenAI-compatible client

## Configuration

### MAIA Configuration

```yaml
# config.yaml
proxy:
  enabled: true
  backend: "https://api.openai.com"  # Or any OpenAI-compatible API
  default_namespace: "default"
  context_position: "system"  # system, first_user, before_last
  token_budget: 2000

  # Memory extraction patterns (optional)
  extract_patterns:
    - "remember that"
    - "note that"
    - "important:"
```

### Environment Variables

```bash
export OPENAI_API_KEY="your-openai-api-key"
export MAIA_PROXY_BACKEND="https://api.openai.com"
```

## Usage

### With curl

```bash
# Basic request - context is automatically injected
curl -X POST http://localhost:8080/proxy/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H "X-MAIA-Namespace: myapp" \
  -d '{
    "model": "gpt-4",
    "messages": [
      {"role": "user", "content": "What do you remember about my preferences?"}
    ]
  }'

# Skip memory injection for a request
curl -X POST http://localhost:8080/proxy/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H "X-MAIA-Skip-Memory: true" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'

# Skip memory extraction from response
curl -X POST http://localhost:8080/proxy/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H "X-MAIA-Skip-Extract: true" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Just chat, no memory"}]
  }'

# Custom token budget
curl -X POST http://localhost:8080/proxy/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H "X-MAIA-Token-Budget: 4000" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Give me lots of context!"}]
  }'
```

### With Python (OpenAI SDK)

```python
import os
from openai import OpenAI

# Point the client to MAIA proxy
client = OpenAI(
    api_key=os.environ["OPENAI_API_KEY"],
    base_url="http://localhost:8080/proxy/v1"
)

# Add custom headers for MAIA
response = client.chat.completions.create(
    model="gpt-4",
    messages=[
        {"role": "user", "content": "What are my preferences?"}
    ],
    extra_headers={
        "X-MAIA-Namespace": "user:123",
        "X-MAIA-Token-Budget": "2000"
    }
)

print(response.choices[0].message.content)
```

### With TypeScript/JavaScript

```typescript
import OpenAI from 'openai';

const client = new OpenAI({
  apiKey: process.env.OPENAI_API_KEY,
  baseURL: 'http://localhost:8080/proxy/v1',
});

const response = await client.chat.completions.create(
  {
    model: 'gpt-4',
    messages: [{ role: 'user', content: 'What do you know about me?' }],
  },
  {
    headers: {
      'X-MAIA-Namespace': 'user:123',
    },
  }
);

console.log(response.choices[0].message.content);
```

### With Go

```go
package main

import (
    "context"
    "fmt"

    "github.com/sashabaranov/go-openai"
)

func main() {
    config := openai.DefaultConfig("your-api-key")
    config.BaseURL = "http://localhost:8080/proxy/v1"

    client := openai.NewClientWithConfig(config)

    resp, err := client.CreateChatCompletion(
        context.Background(),
        openai.ChatCompletionRequest{
            Model: "gpt-4",
            Messages: []openai.ChatCompletionMessage{
                {Role: "user", Content: "What are my preferences?"},
            },
        },
    )

    if err != nil {
        panic(err)
    }

    fmt.Println(resp.Choices[0].Message.Content)
}
```

## Proxy Headers

| Header | Description | Default |
|--------|-------------|---------|
| `X-MAIA-Namespace` | Target namespace for operations | `default` |
| `X-MAIA-Skip-Memory` | Skip memory injection (`true`/`false`) | `false` |
| `X-MAIA-Skip-Extract` | Skip memory extraction (`true`/`false`) | `false` |
| `X-MAIA-Token-Budget` | Token budget for context | `2000` |

## Context Position Options

Control where MAIA injects context:

### `system` (default)

Prepends to the system message:

```json
{
  "messages": [
    {"role": "system", "content": "[MAIA Context]\n...\n\n[Original System Message]"},
    {"role": "user", "content": "..."}
  ]
}
```

### `first_user`

Prepends to the first user message:

```json
{
  "messages": [
    {"role": "system", "content": "..."},
    {"role": "user", "content": "[MAIA Context]\n...\n\n[Original User Message]"}
  ]
}
```

### `before_last`

Inserts before the last user message:

```json
{
  "messages": [
    {"role": "system", "content": "..."},
    {"role": "user", "content": "Previous message"},
    {"role": "assistant", "content": "Previous response"},
    {"role": "system", "content": "[MAIA Context]"},
    {"role": "user", "content": "Current message"}
  ]
}
```

## Memory Extraction

MAIA automatically extracts and stores memories from assistant responses when they contain trigger patterns:

**Default Patterns:**
- "remember that"
- "note that"
- "important:"
- "I'll remember"
- "I've noted"

**Example:**

User: "Remember that I prefer TypeScript over JavaScript"