# MAIA Architecture

This document describes the internal architecture of MAIA, including its core components, data flow, and design decisions.

---

## Overview

MAIA is designed as a **modular monolith** with clear internal boundaries. It processes memory operations through a pipeline that includes query understanding, multi-strategy retrieval, and position-aware context assembly.

```
┌─────────────────────────────────────────────────────────────────┐
│                      Client Applications                         │
│   (Claude Desktop, Cursor, Custom Apps, OpenAI-compatible)      │
└─────────────────────────────────────────────────────────────────┘
                              │
           ┌──────────────────┼──────────────────┐
           │                  │                  │
           ▼                  ▼                  ▼
    ┌────────────┐    ┌────────────┐    ┌────────────┐
    │ MCP Server │    │   Proxy    │    │   REST     │
    │  (stdio)   │    │ (OpenAI)   │    │   API      │
    └────────────┘    └────────────┘    └────────────┘
           │                  │                  │
           └──────────────────┼──────────────────┘
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                       MAIA Core Engine                           │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │                    Query Understanding                      │ │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐   │ │
│  │  │  Intent  │  │  Entity  │  │ Context  │  │ Temporal │   │ │
│  │  │ Detector │  │Extractor │  │  Typer   │  │  Scoper  │   │ │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘   │ │
│  └────────────────────────────────────────────────────────────┘ │
│                              │                                   │
│                              ▼                                   │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │                   Retrieval Layer                           │ │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐   │ │
│  │  │  Vector  │  │FullText  │  │ Recency  │  │  Graph   │   │ │
│  │  │ Scorer   │  │ Scorer   │  │ Scorer   │  │ Scorer   │   │ │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘   │ │
│  │                     │                                       │ │
│  │                     ▼                                       │ │
│  │            ┌─────────────────┐                              │ │
│  │            │  Score Fusion   │                              │ │
│  │            │ (RRF Algorithm) │                              │ │
│  │            └─────────────────┘                              │ │
│  └────────────────────────────────────────────────────────────┘ │
│                              │                                   │
│                              ▼                                   │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │                Context Assembly Layer                       │ │
│  │  ┌───────────────────────────────────────────────────────┐ │ │
│  │  │  Position Optimizer (Critical → Middle → Recency)     │ │ │
│  │  └───────────────────────────────────────────────────────┘ │ │
│  │  ┌───────────────────────────────────────────────────────┐ │ │
│  │  │  Token Budget Manager                                  │ │ │
│  │  └───────────────────────────────────────────────────────┘ │ │
│  └────────────────────────────────────────────────────────────┘ │
│                              │                                   │
│  ┌───────────────────────────┼───────────────────────────────┐  │
│  │                     Index Layer                            │  │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────────────────────┐ │  │
│  │  │  Vector  │  │FullText  │  │         Graph            │ │  │
│  │  │  (HNSW)  │  │ (Bleve)  │  │   (Relationships)        │ │  │
│  │  └──────────┘  └──────────┘  └──────────────────────────┘ │  │
│  └────────────────────────────────────────────────────────────┘  │
│                              │                                   │
│  ┌───────────────────────────┼───────────────────────────────┐  │
│  │                  Storage Layer                             │  │
│  │              ┌──────────────────┐                          │  │
│  │              │    BadgerDB      │                          │  │
│  │              │ (Embedded K/V)   │                          │  │
│  │              └──────────────────┘                          │  │
│  └────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────┘
```

---

## Core Components

### 1. API Layer

MAIA exposes multiple integration patterns:

#### REST API (Gin Framework)

- **Endpoint**: `http://localhost:8080`
- **Purpose**: Universal HTTP interface
- **Features**: Full CRUD operations, context assembly, statistics

#### MCP Server

- **Protocol**: Model Context Protocol (stdio)
- **Purpose**: Native Claude Desktop and Cursor integration
- **Features**: Tools, resources, and prompts

#### OpenAI-Compatible Proxy

- **Endpoint**: `http://localhost:8080/proxy`
- **Purpose**: Drop-in replacement for OpenAI API
- **Features**: Context injection, memory extraction, streaming

### 2. Query Understanding Layer

Located in `internal/query/`

The query analyzer processes incoming queries to understand:

```go
type QueryAnalysis struct {
    Query         string
    Intent        Intent           // factual, procedural, conversational, etc.
    Entities      []Entity         // dates, keywords, names
    ContextType   ContextType      // semantic, episodic, working
    TemporalScope TemporalScope    // recent, historical, all
    TokenSuggestion int            // recommended token budget
}
```

#### Intent Detection

| Intent | Description | Example |
|--------|-------------|---------|
| `factual` | Seeking specific facts | "What is the user's email?" |
| `procedural` | How-to questions | "How do I configure CORS?" |
| `conversational` | Discussion context | "What did we talk about yesterday?" |
| `exploratory` | Open-ended queries | "Tell me about the project" |
| `temporal` | Time-specific | "What happened last week?" |

#### Entity Extraction

- **Dates**: "yesterday", "last week", "2024-01-15"
- **Keywords**: Technical terms, product names
- **Names**: People, organizations, projects

### 3. Retrieval Layer

Located in `internal/retrieval/`

Multi-strategy retrieval combines multiple ranking signals:

#### Strategy Weights (Default)

```go
var DefaultWeights = Weights{
    VectorWeight:    0.35,  // Semantic similarity
    TextWeight:      0.25,  // Keyword matching
    RecencyWeight:   0.20,  // Time-based decay
    FrequencyWeight: 0.10,  // Access frequency
    GraphWeight:     0.10,  // Relationship connections
}
```

#### Reciprocal Rank Fusion (RRF)

Results from each strategy are combined using RRF:

```
RRF_score = Σ (1 / (k + rank_i)) for each strategy i
```

Where `k = 60` (constant) and `rank_i` is the rank in strategy `i`.

### 4. Index Layer

Located in `internal/index/`

#### Vector Index (HNSW)

- **Algorithm**: Hierarchical Navigable Small World
- **Distance**: Cosine similarity
- **Parameters**:
  - `M`: 16 (max connections per node)
  - `EfConstruction`: 200 (build quality)
  - `EfSearch`: 50 (search quality)

```go
type VectorIndex interface {
    Add(id string, vector []float32) error
    Search(vector []float32, k int) ([]SearchResult, error)
    Delete(id string) error
    Save(w io.Writer) error
    Load(r io.Reader) error
}
```

#### Full-Text Index (Bleve)

- **Engine**: Bleve v2
- **Features**: Stemming, tokenization, phrase matching
- **Fields**: Content, tags, type, namespace

```go
type FullTextIndex interface {
    Index(doc *Document) error
    Search(query string, limit int) ([]SearchResult, error)
    Delete(id string) error
}
```

#### Graph Index

- **Structure**: Adjacency list
- **Edge Types**: `related_to`, `contradicts`, `supersedes`, `derived_from`, `part_of`
- **Traversal**: BFS with depth limiting

```go
type GraphIndex interface {
    AddEdge(from, to string, relation RelationType, weight float32) error
    Traverse(startID string, opts TraversalOptions) (*TraversalResult, error)
    RemoveNode(id string) error
}
```

### 5. Embedding Layer

Located in `internal/embedding/`

#### Providers

| Provider | Model | Dimensions | Latency |
|----------|-------|------------|---------|
| `local` | all-MiniLM-L6-v2 | 384 | ~10ms |
| `openai` | text-embedding-3-small | 1536 | ~100ms |
| `voyage` | voyage-02 | 1024 | ~80ms |

#### Local Embedding Architecture

```
┌─────────────────────────────────────────┐
│           Local Provider                 │
│  ┌─────────────────────────────────┐    │
│  │     WordPiece Tokenizer         │    │
│  │  (BERT-style tokenization)      │    │
│  └─────────────────────────────────┘    │
│                  │                       │
│                  ▼                       │
│  ┌─────────────────────────────────┐    │
│  │      ONNX Runtime                │    │
│  │  (all-MiniLM-L6-v2.onnx)        │    │
│  └─────────────────────────────────┘    │
│                  │                       │
│                  ▼                       │
│  ┌─────────────────────────────────┐    │
│  │     Mean Pooling + L2 Norm      │    │
│  └─────────────────────────────────┘    │
└─────────────────────────────────────────┘
```

### 6. Context Assembly Layer

Located in `internal/context/`

The context assembler implements position-aware ordering:

#### Zone Strategy

```
┌─────────────────────────────────────────────────────┐
│  CRITICAL ZONE (15% of budget)                      │
│  - Highest relevance memories (score ≥ 0.7)         │
│  - Most important facts                             │
│  - Positioned first for LLM attention               │
├─────────────────────────────────────────────────────┤
│  MIDDLE ZONE (65% of budget)                        │
│  - Supporting context                               │
│  - Decreasing relevance order                       │
│  - Detailed information                             │
├─────────────────────────────────────────────────────┤
│  RECENCY ZONE (20% of budget)                       │
│  - Working memory (always here)                     │
│  - Recent interactions (within 24h, score ≥ 0.3)   │
│  - Positioned last for recency effect              │
└─────────────────────────────────────────────────────┘
```

#### Token Counting

Token estimation uses a heuristic approach:

```go
// Approximate token count (characters / 4 + whitespace overhead)
func EstimateTokens(text string) int {
    words := strings.Fields(text)
    charTokens := len(text) / 4
    wordTokens := len(words) + len(words)/2 // word + subword overhead
    return max(charTokens, wordTokens)
}
```

### 7. Storage Layer

Located in `internal/storage/`

#### BadgerDB Configuration

- **Engine**: BadgerDB v4 (LSM-tree)
- **Mode**: Embedded (no external dependencies)
- **Encryption**: AES-256 at rest (optional)

#### Key Structure

```
memories:<namespace>:<id>     → Memory JSON
namespaces:<id>               → Namespace JSON
vectors:<id>                  → Vector data
indices:fulltext:*            → Bleve index files
indices:vector:*              → HNSW index files
```

#### Store Interface

```go
type Store interface {
    // Memory CRUD
    CreateMemory(ctx context.Context, input *CreateMemoryInput) (*Memory, error)
    GetMemory(ctx context.Context, id string) (*Memory, error)
    UpdateMemory(ctx context.Context, id string, input *UpdateMemoryInput) (*Memory, error)
    DeleteMemory(ctx context.Context, id string) error

    // Search
    SearchMemories(ctx context.Context, opts *SearchOptions) ([]*SearchResult, error)
    ListMemories(ctx context.Context, namespace string, opts *ListOptions) ([]*Memory, error)

    // Namespaces
    CreateNamespace(ctx context.Context, input *CreateNamespaceInput) (*Namespace, error)
    GetNamespace(ctx context.Context, id string) (*Namespace, error)
    GetNamespaceByName(ctx context.Context, name string) (*Namespace, error)
    UpdateNamespace(ctx context.Context, id string, config *NamespaceConfig) (*Namespace, error)
    DeleteNamespace(ctx context.Context, id string) error
    ListNamespaces(ctx context.Context, opts *ListOptions) ([]*Namespace, error)

    // Statistics
    Stats(ctx context.Context) (*StoreStats, error)

    // Lifecycle
    Close() error
}
```

---

## Data Flow

### Memory Write Flow

```
1. Client Request (POST /v1/memories)
       │
       ▼
2. Input Validation
       │
       ▼
3. Generate Embedding (if content provided)
       │
       ▼
4. Store in BadgerDB
       │
       ▼
5. Index in Vector/FullText/Graph indices
       │
       ▼
6. Return Memory with ID
```

### Context Assembly Flow

```
1. Client Request (POST /v1/context)
       │
       ▼
2. Query Analysis (intent, entities, scope)
       │
       ▼
3. Multi-Strategy Retrieval
   ├── Vector Search (embedding similarity)
   ├── Full-Text Search (keyword matching)
   ├── Recency Scoring (time decay)
   ├── Frequency Scoring (access count)
   └── Graph Traversal (relationships)
       │
       ▼
4. Score Fusion (RRF)
       │
       ▼
5. Zone Assignment (Critical/Middle/Recency)
       │
       ▼
6. Token Budget Allocation
       │
       ▼
7. Content Assembly
       │
       ▼
8. Return Context Response
```

### Proxy Flow

```
1. Client Request (POST /proxy/v1/chat/completions)
       │
       ▼
2. Extract query from last user message
       │
       ▼
3. Assemble context (if not skipped)
       │
       ▼
4. Inject context (system/first_user/before_last)
       │
       ▼
5. Forward to backend LLM
       │
       ▼
6. Stream response to client
       │
       ▼
7. Extract memories from response (if not skipped)
       │
       ▼
8. Store extracted memories
```

---

## Design Decisions

### Why BadgerDB?

| Consideration | Decision |
|---------------|----------|
| **Single-binary deployment** | BadgerDB embeds directly (no external processes) |
| **Performance** | LSM-tree excels at write-heavy workloads |
| **Simplicity** | No connection management, no SQL complexity |
| **Trade-off** | Less query flexibility (mitigated by custom indices) |

### Why Local Embeddings First?

| Consideration | Decision |
|---------------|----------|
| **Latency** | ~10ms local vs ~100ms remote |
| **Cost** | Zero marginal cost per embedding |
| **Privacy** | Data never leaves the deployment |
| **Trade-off** | Slightly lower quality (acceptable for retrieval) |

### Why Position-Aware Assembly?

Research shows LLM accuracy varies 20-30% based on information position in context:

- **Primacy effect**: Information at the start gets more attention
- **Recency effect**: Information at the end is remembered better
- **Middle blindness**: Middle content is often overlooked

MAIA's zone strategy addresses this by:
1. Putting critical facts first (primacy)
2. Recent/working memory last (recency)
3. Supporting context in the middle

---

## Performance Characteristics

### Latency Targets

| Operation | Target | Actual |
|-----------|--------|--------|
| Memory write | < 50ms p99 | ~1ms |
| Memory read | < 20ms p99 | ~0.5ms |
| Vector search (10k memories) | < 50ms p99 | ~5ms |
| Context assembly (500 memories) | < 200ms p99 | ~0.3ms |

### Throughput

| Operation | Throughput |
|-----------|------------|
| Concurrent reads | 80,000+ ops/sec |
| Concurrent writes | 21,000+ ops/sec |
| Mixed workload (80/20) | 57,000+ ops/sec |

### Memory Usage

- **Baseline**: ~50MB (empty store)
- **Per memory**: ~1KB (compressed)
- **Vector index**: ~4KB per vector (384 dims)

---

## Security Architecture

### Authentication

```
┌─────────────────────────────────────────┐
│           Request                        │
│  Authorization: Bearer <api-key>         │
│  X-API-Key: <api-key>                   │
│  ?api_key=<api-key>                     │
└─────────────────────────────────────────┘
              │
              ▼
┌─────────────────────────────────────────┐
│        Auth Middleware                   │
│  1. Extract key from header/query        │
│  2. Validate against configured keys     │
│  3. Attach identity to context           │
└─────────────────────────────────────────┘
```

### Authorization (Namespace-Level)

```yaml
security:
  authorization:
    enabled: true
    default_policy: "deny"
    api_key_permissions:
      "key-admin": ["*"]           # All namespaces
      "key-team-a": ["team-a/*"]   # Hierarchical access
      "key-project": ["project-x"] # Specific namespace
```

### Rate Limiting

Token bucket algorithm:
- **RPS limit**: Configurable (default: 100)
- **Burst**: 1.5x RPS limit
- **Per-client**: Based on IP address

---

## Observability

### Metrics (Prometheus)

```
# HTTP metrics
maia_http_requests_total{method,path,status}
maia_http_request_duration_seconds{method,path}
maia_http_requests_in_flight

# Operation metrics
maia_memory_operations_total{operation,namespace}
maia_memory_operations_duration_seconds{operation}
maia_search_operations_total{namespace}
maia_context_assembly_duration_seconds

# Tenant metrics (multi-tenancy)
maia_tenant_memories_total{tenant}
maia_tenant_storage_bytes{tenant}
maia_tenant_requests_total{tenant}
```

### Tracing (OpenTelemetry)

```go
// Spans created for major operations
span := tracer.Start(ctx, "context.assembly")
defer span.End()

span.SetAttributes(
    attribute.String("namespace", namespace),
    attribute.Int("token_budget", budget),
    attribute.Int("memories_retrieved", len(memories)),
)
```

### Logging (Zap)

```go
logger.Info("memory created",
    zap.String("id", memory.ID),
    zap.String("namespace", memory.Namespace),
    zap.String("type", string(memory.Type)),
    zap.Duration("latency", elapsed),
)
```

---

## Scalability

### Current Design (Single Node)

MAIA is designed as a single-node deployment optimized for:
- Developer workstations
- Small team deployments
- Edge deployments

### Future Considerations

For larger deployments, the architecture supports:

1. **Storage sharding**: Partition by namespace
2. **Index replication**: Read replicas for search
3. **Embedding service**: Dedicated embedding workers
4. **Cache layer**: Redis/Memcached for hot memories

---

## Related Documents

- [Configuration](configuration.md) - All configuration options
- [API Reference](api-reference.md) - REST API documentation
- [Deployment](deployment.md) - Production deployment guide
