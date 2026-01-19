# Project: MAIA (Memory AI Architecture)

> **Purpose**: AI-native distributed memory system for LLM context management
>
> **Created**: 2026-01-19
> **Last Updated**: 2026-01-19

---

## Tech Stack

### Language & Runtime

- **Language**: Go 1.22+
- **Runtime**: Go 1.22+ (native compilation)
- **Package Manager**: Go modules

### Core Dependencies

- **HTTP Framework**: Gin (high-performance, middleware support)
- **gRPC**: google.golang.org/grpc (for high-performance internal APIs)
- **Storage**: BadgerDB v4 (embedded key-value, LSM-tree)
- **Full-text Search**: Bleve v2 (Go-native full-text indexing)
- **Vector Index**: Custom HNSW implementation or hnswlib-go
- **Configuration**: Viper (environment + file config)
- **Logging**: Zap (structured, high-performance)

### Embedding Model

- **Local Model**: all-MiniLM-L6-v2 (via ONNX runtime)
- **Fallback**: Configurable remote API (OpenAI, Voyage)

### Infrastructure

- **Containerization**: Docker
- **Orchestration**: Kubernetes (Helm charts)
- **CI/CD**: GitHub Actions

### Key Libraries & Tools

- **Validation**: go-playground/validator
- **Testing**: Go testing + testify
- **Linting**: golangci-lint
- **Formatting**: gofmt + goimports

---

## Architecture

### Type

Modular monolith with plugin architecture (designed for future distribution)

### Structure

```text
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
       │  │  (intent, entities, context)    ││
       │  └─────────────────────────────────┘│
       │                  │                  │
       │                  ▼                  │
       │  ┌─────────────────────────────────┐│
       │  │     Memory Retrieval Layer      ││
       │  │  (vector + graph + fulltext)    ││
       │  └─────────────────────────────────┘│
       │                  │                  │
       │  ┌─────────┬─────┴─────┬───────────┐│
       │  │Semantic │ Episodic  │  Working  ││
       │  │ Store   │  Store    │   Store   ││
       │  └─────────┴───────────┴───────────┘│
       │                  │                  │
       │                  ▼                  │
       │  ┌─────────────────────────────────┐│
       │  │   Context Assembly Layer        ││
       │  │  (position-aware, optimized)    ││
       │  └─────────────────────────────────┘│
       └─────────────────────────────────────┘
                         │
                         ▼
                   Target LLM
            (Claude, GPT-4, Llama, etc.)
```

### Key Patterns

- **Repository pattern**: Storage abstraction for swappable backends
- **Strategy pattern**: Pluggable retrieval strategies
- **Pipeline pattern**: Query → Retrieve → Assemble → Respond
- **Interface segregation**: Small, focused interfaces

### Folder Structure

```text
maia/
├── cmd/
│   ├── maia/           # Main server binary
│   ├── maiactl/        # CLI tool
│   └── mcp-server/     # Standalone MCP server
├── internal/
│   ├── config/         # Configuration management
│   ├── server/         # HTTP/gRPC server
│   ├── storage/        # Storage layer (BadgerDB)
│   ├── index/          # Indexing (vector, fulltext, graph)
│   ├── query/          # Query understanding
│   ├── retrieval/      # Multi-strategy retrieval
│   ├── context/        # Context assembly
│   ├── namespace/      # Namespace management
│   ├── embedding/      # Embedding generation
│   └── writeback/      # Memory extraction & consolidation
├── pkg/
│   ├── maia/           # Go SDK (public)
│   ├── mcp/            # MCP implementation
│   └── proxy/          # OpenAI proxy
├── api/
│   ├── proto/          # Protobuf definitions
│   └── openapi/        # OpenAPI spec
├── sdk/
│   ├── typescript/     # TypeScript SDK
│   └── python/         # Python SDK
├── deployments/        # Docker, Kubernetes configs
├── docs/               # Documentation
└── examples/           # Usage examples
```

---

## Key Design Decisions

### Decision: Embedded Storage (BadgerDB)

**Date**: 2026-01-19

**Context**: Need fast, reliable storage without operational complexity

**Options Considered**:

1. PostgreSQL - Pros: Mature, powerful. Cons: Requires separate process, complex setup
2. SQLite - Pros: Embedded, simple. Cons: Limited concurrency, not ideal for this workload
3. BadgerDB - Pros: Embedded, pure Go, excellent performance. Cons: Less ecosystem

**Decision**: BadgerDB

**Rationale**: Single-binary deployment is critical for developer adoption. BadgerDB provides excellent read/write performance for our access patterns (write-heavy with burst reads).

**Trade-offs**: Less query flexibility than SQL, but we're building custom indices anyway.

---

### Decision: Local Embedding Model First

**Date**: 2026-01-19

**Context**: Embeddings required for semantic search

**Options Considered**:

1. OpenAI API only - Pros: High quality. Cons: Latency, cost, external dependency
2. Local model only - Pros: Fast, free, private. Cons: Lower quality
3. Hybrid - Pros: Flexibility. Cons: Complexity

**Decision**: Local model (all-MiniLM-L6-v2) with optional remote fallback

**Rationale**: <10ms embedding time is critical for our <200ms latency target. Quality is sufficient for retrieval (we're not doing semantic similarity scoring for user output).

**Trade-offs**: May need to revisit for specialized domains.

---

### Decision: Position-Aware Context Assembly

**Date**: 2026-01-19

**Context**: Research shows "context rot" - LLM accuracy varies 20-30% based on where information appears in context

**Decision**: Implement position-aware ordering strategy:

1. Critical info first (system prompt area)
2. Decreasing relevance in middle
3. Recent/temporal context last

**Rationale**: Novel differentiator - no existing system addresses this directly.

---

### Decision: Multiple Integration Patterns

**Date**: 2026-01-19

**Context**: Need to support diverse use cases

**Decision**: Support all three patterns:

1. MCP Server (Claude/Cursor native)
2. OpenAI-compatible Proxy (drop-in replacement)
3. Native SDKs (Go, TypeScript, Python)

**Rationale**: MCP for modern tooling, proxy for legacy apps, SDKs for custom integrations.

**Trade-offs**: More surface area to maintain, but necessary for adoption.

---

## Environment & Configuration

### Environment Variables

```bash
# Required
MAIA_DATA_DIR=./data           # Storage directory
MAIA_HTTP_PORT=8080            # HTTP API port
MAIA_GRPC_PORT=9090            # gRPC API port

# Optional
MAIA_LOG_LEVEL=info            # debug, info, warn, error
MAIA_EMBEDDING_MODEL=local     # local, openai, voyage
MAIA_OPENAI_API_KEY=           # If using OpenAI embeddings
MAIA_DEFAULT_NAMESPACE=default # Default namespace for operations
MAIA_TOKEN_BUDGET=4000         # Default context token budget
```

### Local Development Setup

1. Install Go 1.22+: `brew install go` (macOS) or download from golang.org
2. Clone repository: `git clone github.com/ar4mirez/maia && cd maia`
3. Install dependencies: `go mod download`
4. Copy config: `cp .env.example .env`
5. Run server: `make dev` or `go run ./cmd/maia`
6. Verify: `curl http://localhost:8080/health`

---

## Dependencies & Version Constraints

### Critical Constraints

- Go >= 1.22 (required for range over int, improved performance)
- BadgerDB v4 (breaking changes from v3)

### Pinned Versions (Do Not Upgrade Without Testing)

- `github.com/dgraph-io/badger/v4` - Core storage
- `github.com/blevesearch/bleve/v2` - Full-text search

---

## Testing Strategy

### Test Types

- **Unit**: Core logic, storage operations, retrieval algorithms
- **Integration**: API endpoints, MCP tools, proxy behavior
- **E2E**: Full flows (remember → retrieve → context assembly)
- **Benchmark**: Latency measurements for all critical paths

### Coverage Targets

- Critical paths (storage, retrieval, assembly): 90%+
- Core business logic: 80%+
- Overall: 70%+

### Test Commands

```bash
go test ./...              # Run all tests
go test -v ./internal/...  # Verbose internal tests
go test -cover ./...       # With coverage
go test -bench=. ./...     # Run benchmarks
make test                  # All tests + linting
```

---

## Performance Targets

### API Response Times

- Memory write: <50ms p99
- Memory read: <20ms p99
- Vector search: <50ms p99
- Context assembly: <200ms p99 (end-to-end)

### Resource Usage

- Memory: <500MB baseline (scales with data)
- Disk: ~1KB per memory entry (compressed)
- CPU: Burst during embedding, low otherwise

---

## Security Considerations

### Authentication

- API key-based auth for REST/gRPC
- Namespace-scoped permissions
- MCP inherits client authentication

### Data Protection

- All data encrypted at rest (BadgerDB encryption)
- No sensitive data in logs
- Namespace isolation enforced at storage level

### API Security

- Rate limiting configurable per namespace
- Input validation on all endpoints
- No SQL/injection vectors (no SQL)

---

## Deployment

### Environments

1. **Development**: Local machine, single binary
2. **Staging**: Docker container, test data
3. **Production**: Kubernetes, persistent volumes

### Deployment Process

1. Build: `make build`
2. Test: `make test`
3. Docker: `docker build -t maia:latest .`
4. Deploy: Helm chart or docker-compose

### Rollback Procedure

1. Kubernetes: `kubectl rollout undo deployment/maia`
2. Docker: `docker-compose down && docker-compose up -d --no-deps maia:previous`

---

## Known Issues & Gotchas

### HNSW Index Rebuild

**Description**: Vector index must be rebuilt if embedding model changes

**Workaround**: Provide migration tool in maiactl

**Tracking**: Expected behavior, document clearly

---

### BadgerDB Compaction

**Description**: Large deletes require manual compaction for space reclaim

**Workaround**: Schedule periodic `db.Flatten()` during low-traffic periods

**Tracking**: Document in operations guide

---

## External Resources

- **Research**: Context Rot paper (Chroma 2024)
- **Benchmarks**: Mem0 research paper for comparison
- **MCP Spec**: <https://modelcontextprotocol.io/>
