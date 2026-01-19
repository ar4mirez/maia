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

### Phase 3: Context Assembly (🔴 IN PROGRESS)

**Goal**: Position-aware context assembly with token budget optimization

**Deliverables**:
- [ ] Token counter (tiktoken-based estimation)
- [ ] Context assembler with position strategies
- [ ] Token budget optimizer
- [ ] Memory prioritization algorithms
- [ ] Update `/v1/context` endpoint to use assembler
- [ ] Integration with retriever

**Position Strategy** (from research):
1. **Critical Zone** (first 10-15%): Most important factual information
2. **Middle Zone** (70%): Supporting context, decreasing relevance
3. **Recency Zone** (last 15-20%): Recent/temporal information

**Success Criteria**:
- Context assembly < 200ms p99
- Token budget adherence within 5%
- Position-aware ordering demonstrably improves retrieval

---

### Phase 4: Embedding & Indexing (🔲 PLANNED)

**Goal**: Production-ready embedding generation and index management

**Deliverables**:
- [ ] Local embedding model (all-MiniLM-L6-v2 via ONNX)
- [ ] Remote embedding fallback (OpenAI, Voyage)
- [ ] Index persistence and recovery
- [ ] Index rebuild/migration tools
- [ ] Background index updates

---

### Phase 5: MCP Integration (🔲 PLANNED)

**Goal**: Model Context Protocol server for Claude/Cursor integration

**Deliverables**:
- [ ] MCP server implementation
- [ ] Tools: remember, recall, forget
- [ ] Resources: memory browser
- [ ] Prompts: context injection templates

---

### Phase 6: OpenAI Proxy (🔲 PLANNED)

**Goal**: Drop-in replacement for OpenAI API with automatic memory

**Deliverables**:
- [ ] Chat completions proxy
- [ ] Automatic memory extraction
- [ ] Context injection
- [ ] Rate limiting

---

### Phase 7: SDKs (🔲 PLANNED)

**Goal**: Client libraries for common languages

**Deliverables**:
- [ ] Go SDK (pkg/maia)
- [ ] TypeScript SDK
- [ ] Python SDK

---

### Phase 8: Production Hardening (🔲 PLANNED)

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
