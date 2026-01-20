# MAIA SDKs

MAIA provides official SDKs for Go, TypeScript, and Python. All SDKs share a consistent API design based on three core operations: **Remember**, **Recall**, and **Forget**.

---

## SDK Overview

| SDK | Package | Language Version | Features |
|-----|---------|------------------|----------|
| Go | `github.com/cuemby/maia/pkg/maia` | Go 1.22+ | Full API, type-safe |
| TypeScript | `@maia/sdk` | Node 18+, browsers | Async/Promise, type definitions |
| Python | `maia-sdk` | Python 3.9+ | Sync + async, Pydantic models |

---

## Go SDK

### Installation

```bash
go get github.com/cuemby/maia/pkg/maia
```

### Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/cuemby/maia/pkg/maia"
)

func main() {
    ctx := context.Background()

    // Create client
    client := maia.New(
        maia.WithBaseURL("http://localhost:8080"),
    )

    // Remember something
    mem, err := client.Remember(ctx, "default", "User prefers dark mode")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Stored: %s\n", mem.ID)

    // Recall context
    result, err := client.Recall(ctx, "user preferences",
        maia.WithNamespace("default"),
        maia.WithTokenBudget(2000),
    )
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Context: %s\n", result.Content)

    // Forget
    if err := client.Forget(ctx, mem.ID); err != nil {
        log.Fatal(err)
    }
}
```

### Client Configuration

```go
// Full configuration
client := maia.New(
    maia.WithBaseURL("http://localhost:8080"),
    maia.WithTimeout(30 * time.Second),
    maia.WithHeader("X-API-Key", "your-api-key"),
    maia.WithRetries(3),
)

// From environment
client := maia.NewFromEnv() // Uses MAIA_URL, MAIA_API_KEY
```

### Memory Operations

#### Remember (Create Memory)

```go
// Simple
mem, err := client.Remember(ctx, "namespace", "content")

// With options
mem, err := client.Remember(ctx, "namespace", "content",
    maia.WithMemoryType(maia.MemoryTypeSemantic),
    maia.WithTags("preferences", "settings"),
    maia.WithMetadata(map[string]interface{}{
        "source": "user-input",
        "confidence": 0.95,
    }),
    maia.WithConfidence(0.9),
)
```

#### Recall (Get Context)

```go
// Simple
result, err := client.Recall(ctx, "query")

// With options
result, err := client.Recall(ctx, "user preferences",
    maia.WithNamespace("default"),
    maia.WithTokenBudget(4000),
    maia.WithSystemPrompt("You are a helpful assistant."),
    maia.WithMinScore(0.3),
    maia.WithIncludeScores(true),
)

// Access results
fmt.Printf("Content: %s\n", result.Content)
fmt.Printf("Token count: %d\n", result.TokenCount)
for _, m := range result.Memories {
    fmt.Printf("- %s (score: %.2f, position: %s)\n",
        m.Content, m.Score, m.Position)
}
```

#### Forget (Delete Memory)

```go
err := client.Forget(ctx, memoryID)
```

#### Search Memories

```go
results, err := client.Search(ctx, &maia.SearchMemoriesInput{
    Namespace: "default",
    Query:     "preferences",
    Types:     []maia.MemoryType{maia.MemoryTypeSemantic},
    Tags:      []string{"settings"},
    Limit:     20,
})

for _, r := range results {
    fmt.Printf("- %s (score: %.2f)\n", r.Memory.Content, r.Score)
}
```

### Namespace Operations

```go
// Create namespace
ns, err := client.CreateNamespace(ctx, &maia.CreateNamespaceInput{
    Name: "my-project",
    Config: maia.NamespaceConfig{
        TokenBudget: 8000,
        MaxMemories: 50000,
    },
})

// List namespaces
namespaces, err := client.ListNamespaces(ctx)

// Get namespace
ns, err := client.GetNamespace(ctx, "my-project")

// Update namespace
ns, err := client.UpdateNamespace(ctx, "my-project", &maia.UpdateNamespaceInput{
    Config: maia.NamespaceConfig{
        TokenBudget: 10000,
    },
})

// Delete namespace
err := client.DeleteNamespace(ctx, "my-project")

// List memories in namespace
memories, err := client.ListNamespaceMemories(ctx, "my-project", &maia.ListOptions{
    Limit:  100,
    Offset: 0,
})
```

### Full CRUD Operations

```go
// Create
mem, err := client.CreateMemory(ctx, &maia.CreateMemoryInput{
    Namespace:  "default",
    Content:    "User prefers dark mode",
    Type:       maia.MemoryTypeSemantic,
    Tags:       []string{"preferences"},
    Confidence: 0.9,
})

// Get
mem, err := client.GetMemory(ctx, memoryID)

// Update
mem, err := client.UpdateMemory(ctx, memoryID, &maia.UpdateMemoryInput{
    Content: "User prefers dark mode and monospace fonts",
    Tags:    []string{"preferences", "fonts"},
})

// Delete
err := client.DeleteMemory(ctx, memoryID)
```

### Health and Statistics

```go
// Health check
health, err := client.Health(ctx)
fmt.Printf("Status: %s\n", health.Status)

// Statistics
stats, err := client.Stats(ctx)
fmt.Printf("Memories: %d\n", stats.Storage.MemoryCount)
fmt.Printf("Namespaces: %d\n", stats.Storage.NamespaceCount)
```

### Error Handling

```go
mem, err := client.GetMemory(ctx, "invalid-id")
if err != nil {
    if maia.IsNotFound(err) {
        fmt.Println("Memory not found")
    } else if maia.IsUnauthorized(err) {
        fmt.Println("Invalid API key")
    } else if maia.IsRateLimited(err) {
        fmt.Println("Rate limited, retry later")
    } else {
        fmt.Printf("Error: %v\n", err)
    }
}
```

### Types

```go
// Memory types
type MemoryType string
const (
    MemoryTypeSemantic  MemoryType = "semantic"   // Facts, profiles
    MemoryTypeEpisodic  MemoryType = "episodic"   // Conversations
    MemoryTypeWorking   MemoryType = "working"    // Current session
)

// Memory sources
type MemorySource string
const (
    MemorySourceUser      MemorySource = "user"
    MemorySourceExtracted MemorySource = "extracted"
    MemorySourceInferred  MemorySource = "inferred"
    MemorySourceImported  MemorySource = "imported"
)

// Memory structure
type Memory struct {
    ID          string
    Namespace   string
    Content     string
    Type        MemoryType
    Embedding   []float32
    Metadata    map[string]interface{}
    Tags        []string
    CreatedAt   time.Time
    UpdatedAt   time.Time
    AccessedAt  time.Time
    AccessCount int64
    Confidence  float64
    Source      MemorySource
}

// Context result
type ContextResult struct {
    Content     string
    Memories    []ContextMemory
    TokenCount  int
    TokenBudget int
    Truncated   bool
    ZoneStats   ZoneStats
}
```

---

## TypeScript SDK

### Installation

```bash
npm install @maia/sdk
# or
yarn add @maia/sdk
# or
pnpm add @maia/sdk
```

### Quick Start

```typescript
import { MAIAClient } from '@maia/sdk';

const client = new MAIAClient({
  baseUrl: 'http://localhost:8080',
});

// Remember
const memory = await client.remember('default', 'User prefers dark mode');
console.log('Stored:', memory.id);

// Recall
const context = await client.recall('user preferences', {
  namespace: 'default',
  tokenBudget: 2000,
});
console.log('Context:', context.content);

// Forget
await client.forget(memory.id);
```

### Client Configuration

```typescript
import { MAIAClient } from '@maia/sdk';

const client = new MAIAClient({
  baseUrl: 'http://localhost:8080',
  timeout: 30000, // 30 seconds
  headers: {
    'X-API-Key': 'your-api-key',
  },
  // Custom fetch implementation (optional)
  fetch: customFetch,
});
```

### Memory Operations

#### Remember

```typescript
// Simple
const memory = await client.remember('namespace', 'content');

// With options
const memory = await client.remember('namespace', 'content', {
  type: 'semantic',
  tags: ['preferences', 'settings'],
  metadata: {
    source: 'user-input',
  },
  confidence: 0.9,
});
```

#### Recall

```typescript
// Simple
const context = await client.recall('query');

// With options
const context = await client.recall('user preferences', {
  namespace: 'default',
  tokenBudget: 4000,
  systemPrompt: 'You are a helpful assistant.',
  minScore: 0.3,
  includeScores: true,
});

// Access results
console.log('Content:', context.content);
console.log('Token count:', context.token_count);
for (const m of context.memories) {
  console.log(`- ${m.content} (score: ${m.score}, position: ${m.position})`);
}
```

#### Forget

```typescript
await client.forget(memoryId);
```

#### Search

```typescript
const results = await client.searchMemories({
  namespace: 'default',
  query: 'preferences',
  types: ['semantic'],
  tags: ['settings'],
  limit: 20,
});

for (const r of results.results) {
  console.log(`- ${r.memory.content} (score: ${r.score})`);
}
```

### Namespace Operations

```typescript
// Create
const ns = await client.createNamespace({
  name: 'my-project',
  config: {
    token_budget: 8000,
    max_memories: 50000,
  },
});

// List
const namespaces = await client.listNamespaces();

// Get
const ns = await client.getNamespace('my-project');

// Update
const ns = await client.updateNamespace('my-project', {
  config: { token_budget: 10000 },
});

// Delete
await client.deleteNamespace('my-project');

// List memories
const memories = await client.listNamespaceMemories('my-project', {
  limit: 100,
  offset: 0,
});
```

### Full CRUD Operations

```typescript
// Create
const memory = await client.createMemory({
  namespace: 'default',
  content: 'User prefers dark mode',
  type: 'semantic',
  tags: ['preferences'],
});

// Get
const memory = await client.getMemory(memoryId);

// Update
const memory = await client.updateMemory(memoryId, {
  content: 'User prefers dark mode and monospace fonts',
  tags: ['preferences', 'fonts'],
});

// Delete
await client.deleteMemory(memoryId);
```

### Error Handling

```typescript
import { MAIAError, NotFoundError, UnauthorizedError, RateLimitError } from '@maia/sdk';

try {
  const memory = await client.getMemory('invalid-id');
} catch (err) {
  if (err instanceof NotFoundError) {
    console.log('Memory not found');
  } else if (err instanceof UnauthorizedError) {
    console.log('Invalid API key');
  } else if (err instanceof RateLimitError) {
    console.log('Rate limited, retry later');
  } else if (err instanceof MAIAError) {
    console.log('MAIA error:', err.message);
  } else {
    throw err;
  }
}
```

### Types

```typescript
// Memory types
type MemoryType = 'semantic' | 'episodic' | 'working';
type MemorySource = 'user' | 'extracted' | 'inferred' | 'imported';

// Memory
interface Memory {
  id: string;
  namespace: string;
  content: string;
  type: MemoryType;
  embedding?: number[];
  metadata?: Record<string, unknown>;
  tags?: string[];
  created_at: string;
  updated_at: string;
  accessed_at: string;
  access_count: number;
  confidence: number;
  source: MemorySource;
}

// Context result
interface ContextResult {
  content: string;
  memories: ContextMemory[];
  token_count: number;
  token_budget: number;
  truncated: boolean;
  zone_stats: ZoneStats;
}

// Context memory
interface ContextMemory {
  id: string;
  content: string;
  type: MemoryType;
  score: number;
  position: 'critical' | 'middle' | 'recency';
  token_count: number;
  truncated: boolean;
}
```

### Browser Usage

```typescript
import { MAIAClient } from '@maia/sdk';

// Works in browsers with native fetch
const client = new MAIAClient({
  baseUrl: 'https://maia.example.com',
});
```

---

## Python SDK

### Installation

```bash
pip install maia-sdk
# or
poetry add maia-sdk
```

### Quick Start

```python
from maia import MAIAClient

client = MAIAClient(base_url="http://localhost:8080")

# Remember
memory = client.remember("default", "User prefers dark mode")
print(f"Stored: {memory.id}")

# Recall
context = client.recall("user preferences", namespace="default", token_budget=2000)
print(f"Context: {context.content}")

# Forget
client.forget(memory.id)
```

### Async Client

```python
import asyncio
from maia import AsyncMAIAClient

async def main():
    async with AsyncMAIAClient(base_url="http://localhost:8080") as client:
        # Remember
        memory = await client.remember("default", "User prefers dark mode")

        # Recall
        context = await client.recall("user preferences", namespace="default")

        # Forget
        await client.forget(memory.id)

asyncio.run(main())
```

### Client Configuration

```python
from maia import MAIAClient

# Full configuration
client = MAIAClient(
    base_url="http://localhost:8080",
    api_key="your-api-key",
    timeout=30.0,
    headers={"X-Custom-Header": "value"},
)

# From environment
import os
client = MAIAClient(
    base_url=os.getenv("MAIA_URL", "http://localhost:8080"),
    api_key=os.getenv("MAIA_API_KEY"),
)
```

### Memory Operations

#### Remember

```python
# Simple
memory = client.remember("namespace", "content")

# With options
memory = client.remember(
    "namespace",
    "content",
    type="semantic",
    tags=["preferences", "settings"],
    metadata={"source": "user-input"},
    confidence=0.9,
)
```

#### Recall

```python
# Simple
context = client.recall("query")

# With options
context = client.recall(
    "user preferences",
    namespace="default",
    token_budget=4000,
    system_prompt="You are a helpful assistant.",
    min_score=0.3,
    include_scores=True,
)

# Access results
print(f"Content: {context.content}")
print(f"Token count: {context.token_count}")
for m in context.memories:
    print(f"- {m.content} (score: {m.score}, position: {m.position})")
```

#### Forget

```python
client.forget(memory_id)
```

#### Search

```python
results = client.search_memories(
    namespace="default",
    query="preferences",
    types=["semantic"],
    tags=["settings"],
    limit=20,
)

for r in results.results:
    print(f"- {r.memory.content} (score: {r.score})")
```

### Namespace Operations

```python
# Create
ns = client.create_namespace(
    name="my-project",
    token_budget=8000,
    max_memories=50000,
)

# List
namespaces = client.list_namespaces()

# Get
ns = client.get_namespace("my-project")

# Update
ns = client.update_namespace("my-project", token_budget=10000)

# Delete
client.delete_namespace("my-project")

# List memories
memories = client.list_namespace_memories("my-project", limit=100)
```

### Full CRUD Operations

```python
# Create
memory = client.create_memory(
    namespace="default",
    content="User prefers dark mode",
    type="semantic",
    tags=["preferences"],
)

# Get
memory = client.get_memory(memory_id)

# Update
memory = client.update_memory(
    memory_id,
    content="User prefers dark mode and monospace fonts",
    tags=["preferences", "fonts"],
)

# Delete
client.delete_memory(memory_id)
```

### Error Handling

```python
from maia import (
    MAIAClient,
    MAIAError,
    NotFoundError,
    UnauthorizedError,
    RateLimitError,
)

try:
    memory = client.get_memory("invalid-id")
except NotFoundError:
    print("Memory not found")
except UnauthorizedError:
    print("Invalid API key")
except RateLimitError as e:
    print(f"Rate limited, retry after {e.retry_after}s")
except MAIAError as e:
    print(f"MAIA error: {e}")
```

### Context Manager

```python
# Sync client
with MAIAClient() as client:
    memory = client.remember("default", "content")

# Async client
async with AsyncMAIAClient() as client:
    memory = await client.remember("default", "content")
```

### Types (Pydantic Models)

```python
from enum import Enum
from datetime import datetime
from pydantic import BaseModel
from typing import Optional

class MemoryType(str, Enum):
    SEMANTIC = "semantic"
    EPISODIC = "episodic"
    WORKING = "working"

class MemorySource(str, Enum):
    USER = "user"
    EXTRACTED = "extracted"
    INFERRED = "inferred"
    IMPORTED = "imported"

class Memory(BaseModel):
    id: str
    namespace: str
    content: str
    type: MemoryType
    embedding: Optional[list[float]] = None
    metadata: Optional[dict] = None
    tags: Optional[list[str]] = None
    created_at: datetime
    updated_at: datetime
    accessed_at: datetime
    access_count: int
    confidence: float
    source: MemorySource

class ContextMemory(BaseModel):
    id: str
    content: str
    type: MemoryType
    score: float
    position: str  # "critical", "middle", "recency"
    token_count: int
    truncated: bool

class ContextResult(BaseModel):
    content: str
    memories: list[ContextMemory]
    token_count: int
    token_budget: int
    truncated: bool
    zone_stats: ZoneStats
```

---

## Common Patterns

### Retry with Exponential Backoff

**Go:**
```go
client := maia.New(
    maia.WithRetries(3),
    maia.WithRetryBackoff(time.Second),
)
```

**TypeScript:**
```typescript
async function withRetry<T>(fn: () => Promise<T>, retries = 3): Promise<T> {
  for (let i = 0; i < retries; i++) {
    try {
      return await fn();
    } catch (err) {
      if (i === retries - 1) throw err;
      await new Promise(r => setTimeout(r, Math.pow(2, i) * 1000));
    }
  }
  throw new Error('Unreachable');
}

const memory = await withRetry(() => client.remember('ns', 'content'));
```

**Python:**
```python
import time

def with_retry(fn, retries=3):
    for i in range(retries):
        try:
            return fn()
        except Exception as e:
            if i == retries - 1:
                raise
            time.sleep(2 ** i)

memory = with_retry(lambda: client.remember("ns", "content"))
```

### Batch Operations

**Go:**
```go
contents := []string{"fact 1", "fact 2", "fact 3"}
var memories []*maia.Memory

for _, content := range contents {
    mem, err := client.Remember(ctx, "default", content)
    if err != nil {
        // Handle error
        continue
    }
    memories = append(memories, mem)
}
```

**TypeScript:**
```typescript
const contents = ['fact 1', 'fact 2', 'fact 3'];
const memories = await Promise.all(
  contents.map(c => client.remember('default', c))
);
```

**Python:**
```python
import asyncio

contents = ["fact 1", "fact 2", "fact 3"]

# Async
async with AsyncMAIAClient() as client:
    memories = await asyncio.gather(*[
        client.remember("default", c) for c in contents
    ])
```

### Memory Lifecycle Management

```go
// Create with TTL metadata
mem, _ := client.Remember(ctx, "session", "temp data",
    maia.WithMetadata(map[string]interface{}{
        "expires_at": time.Now().Add(24 * time.Hour).Unix(),
    }),
)

// Cleanup expired memories
memories, _ := client.ListNamespaceMemories(ctx, "session", nil)
for _, m := range memories {
    if expiresAt, ok := m.Metadata["expires_at"].(float64); ok {
        if time.Now().Unix() > int64(expiresAt) {
            client.Forget(ctx, m.ID)
        }
    }
}
```

---

## Related Documentation

- [API Reference](api-reference.md) - REST API documentation
- [Getting Started](getting-started.md) - Quick start guide
- [Configuration](configuration.md) - Client configuration options
