# MAIA Product Definition

> **Purpose**: Define MAIA's vision, goals, and implementation phases
>
> **Created**: 2026-01-19
> **Last Updated**: 2026-01-19

---

## Vision

MAIA (Memory AI Architecture) is an **AI-native distributed memory system** that solves the fundamental problem of LLM context window limitations. Rather than fighting the context window constraint, MAIA intelligently manages what information goes INTO that window.

### Core Problem

LLMs have limited context windows. Current solutions either:
1. Truncate context (losing important information)
2. Summarize context (losing nuance)
3. Use RAG without position awareness (suboptimal retrieval)

### Our Solution

MAIA provides **position-aware context assembly** that:
1. Understands query intent to retrieve relevant memories
2. Scores memories using multi-strategy retrieval (vector + text + recency + frequency)
3. Assembles context with position-aware ordering (critical info first, recent info last)
4. Optimizes for token budget constraints

---

## Key Differentiators

| Feature | MAIA | Mem0 | LangChain Memory |
|---------|------|------|------------------|
| Position-aware assembly | ✅ | ❌ | ❌ |
| Multi-strategy retrieval | ✅ | Partial | ❌ |
| Sub-200ms latency | ✅ Target | Unknown | Variable |
| Single-binary deployment | ✅ | ❌ | N/A |
| Hierarchical namespaces | ✅ | ❌ | ❌ |
| MCP Server integration | ✅ | ❌ | ❌ |

---

## Implementation Phases

### Phase 1: Foundation (✅ COMPLETE)

**Goal**: Core storage and basic memory operations

**Deliverables**:
- [x] BadgerDB storage layer with full CRUD
- [x] Memory types: Semantic, Episodic, Working, Procedural
- [x] Namespace management with hierarchical support
- [x] HTTP API with Gin
- [x] Batch operations
- [x] Basic configuration management

**Test Coverage**: 74.1% storage layer

---

### Phase 2: Intelligence (✅ COMPLETE)

**Goal**: Query understanding and multi-strategy retrieval

**Deliverables**:
- [x] Query analyzer with intent detection
- [x] Entity extraction (dates, keywords)
- [x] Context type determination
- [x] Temporal scope detection
- [x] Token budget suggestions
- [x] Mock embedding provider
- [x] Vector index (HNSW implementation)
- [x] Full-text index (Bleve)
- [x] Multi-strategy scorer (vector, text, recency, frequency)
- [x] Retriever with score fusion

**Test Coverage**:
- Query analyzer: 97.4%
- Embedding: 95.5%
- Full-text index: 82.8%
- Vector index: 77.8%
- Scorer: 27.8% (needs improvement)

---

### Phase 3: Context Assembly (✅ COMPLETE)

**Goal**: Position-aware context assembly with token budget optimization

**Deliverables**:
- [x] Token counter (heuristic-based estimation)
- [x] Context assembler with position strategies
- [x] Token budget optimizer
- [x] Memory prioritization algorithms
- [x] Update `/v1/context` endpoint to use assembler
- [x] Integration with retriever

**Position Strategy** (implemented):
1. **Critical Zone** (first 15%): Most important factual information (score >= 0.7)
2. **Middle Zone** (65%): Supporting context, decreasing relevance
3. **Recency Zone** (last 20%): Recent/temporal information, working memory

**Results**:
- Context assembly: ~309µs p99 ✅ (target: 200ms)
- Test coverage: 93.1%

---

### Phase 4: Embedding & Indexing (✅ COMPLETE)

**Goal**: Production-ready embedding generation and index management

**Deliverables**:
- [x] Local embedding model (all-MiniLM-L6-v2 via ONNX)
- [x] WordPiece tokenizer implementation
- [x] ONNX Runtime integration
- [x] Model download/caching utilities
- [x] Index persistence and recovery

**Architecture Decision**: RFD 0001 - Local Embedding Provider
- Uses onnxruntime-go for native ONNX inference
- Requires CGO for ONNX Runtime native libraries
- Model auto-downloaded on first use (~90MB)

---

### Phase 5: MCP Integration (✅ COMPLETE)

**Goal**: Model Context Protocol server for Claude/Cursor integration

**Deliverables**:
- [x] MCP server implementation using modelcontextprotocol/go-sdk
- [x] Tools: remember, recall, forget, list_memories, get_context
- [x] Resources: namespaces, memories, stats
- [x] Prompts: inject_context, summarize_memories, explore_memories
- [x] Standalone mcp-server binary

---

### Phase 6: CLI Tool (✅ COMPLETE)

**Goal**: Command-line interface for MAIA management

**Deliverables**:
- [x] maiactl CLI using Cobra
- [x] Memory commands: create, list, get, update, delete, search
- [x] Namespace commands: create, list, get, update, delete
- [x] Context command with zone statistics
- [x] Stats command for server statistics
- [x] JSON and table output formats

---

### Phase 7: OpenAI Proxy (✅ COMPLETE)

**Goal**: Drop-in replacement for OpenAI API with automatic memory

**Deliverables**:
- [x] Chat completions proxy with SSE streaming
- [x] Automatic memory extraction from responses
- [x] Context injection with multiple position strategies
- [x] Token bucket rate limiting

---

### Phase 8: SDKs (🔲 PLANNED)

**Goal**: Client libraries for common languages

**Deliverables**:
- [ ] Go SDK (pkg/maia)
- [ ] TypeScript SDK
- [ ] Python SDK

---

### Phase 9: Production Hardening (🔲 PLANNED)

**Goal**: Production-ready deployment

**Deliverables**:
- [ ] Authentication/authorization
- [ ] Rate limiting
- [ ] Metrics and tracing
- [ ] Kubernetes deployment
- [ ] Documentation

---

## Success Metrics

### Performance
- Memory write: < 50ms p99
- Memory read: < 20ms p99
- Vector search: < 50ms p99
- Context assembly: < 200ms p99

### Quality
- Test coverage: > 80% business logic
- Zero critical security vulnerabilities
- Zero data loss scenarios

### Adoption
- Single-binary deployment working
- MCP integration with Claude Code
- Documentation complete

---

## Non-Goals (Out of Scope)

- Replacement for long-term databases
- Real-time collaboration
- Multi-tenant SaaS (single-tenant focus first)
- Mobile SDKs
- GUI/Dashboard (CLI first)
