# MAIA Documentation

> **MAIA** (Memory AI Architecture) — An AI-native distributed memory system for LLM context management

Welcome to the MAIA documentation. This guide covers everything you need to get started, integrate, and deploy MAIA in your applications.

---

## Quick Navigation

| Document | Description |
|----------|-------------|
| [Getting Started](getting-started.md) | Installation, first steps, and quick examples |
| [Architecture](architecture.md) | System design, components, and data flow |
| [API Reference](api-reference.md) | Complete REST API documentation |
| [Configuration](configuration.md) | All configuration options explained |
| [SDKs](sdks.md) | Go, TypeScript, and Python client libraries |
| [MCP Integration](mcp-integration.md) | Claude Desktop and Cursor integration |
| [CLI Reference](cli.md) | maiactl command-line tool |
| [Deployment](deployment.md) | Docker, Kubernetes, and production setup |
| [Multi-Tenancy](multi-tenancy.md) | Tenant management and isolation |

---

## What is MAIA?

MAIA is an **intelligent context management system** that sits between your application and LLMs. Instead of fighting context window limitations, MAIA intelligently manages what information goes INTO that window.

### The Problem

LLMs have limited context windows. Current solutions either:
1. **Truncate** context (losing important information)
2. **Summarize** context (losing nuance and detail)
3. **Use basic RAG** without position awareness (suboptimal retrieval)

Research shows that LLM accuracy varies by **20-30%** based on where information appears in the context—a phenomenon known as "context rot."

### The MAIA Solution

MAIA provides **position-aware context assembly** that:

1. **Understands query intent** to retrieve the most relevant memories
2. **Scores memories** using multi-strategy retrieval (vector + text + graph + recency)
3. **Assembles context** with position-aware ordering (critical info first, recent info last)
4. **Optimizes for token budget** constraints automatically

---

## Key Features

### Position-Aware Context Assembly

MAIA organizes context into strategic zones:

```
┌─────────────────────────────────────────┐
│  CRITICAL ZONE (15%)                    │  ← High-relevance facts (score ≥ 0.7)
│  Most important information first       │
├─────────────────────────────────────────┤
│  MIDDLE ZONE (65%)                      │  ← Supporting context
│  Decreasing relevance, detailed info    │
├─────────────────────────────────────────┤
│  RECENCY ZONE (20%)                     │  ← Recent/temporal context
│  Working memory, recent interactions    │
└─────────────────────────────────────────┘
```

### Multi-Strategy Retrieval

MAIA combines multiple retrieval strategies for optimal results:

| Strategy | Weight | Purpose |
|----------|--------|---------|
| Vector Similarity | 35% | Semantic matching via embeddings |
| Full-Text Search | 25% | Keyword and phrase matching |
| Recency | 20% | Favor recent information |
| Access Frequency | 10% | Frequently accessed = important |
| Graph Connectivity | 10% | Related memories boost score |

### Multiple Integration Patterns

Choose the integration pattern that fits your use case:

| Pattern | Best For | Effort |
|---------|----------|--------|
| **MCP Server** | Claude Desktop, Cursor | Minimal (config only) |
| **OpenAI Proxy** | Existing OpenAI apps | Drop-in replacement |
| **Native SDKs** | Custom integrations | Full control |
| **REST API** | Any language/platform | Universal |

---

## Quick Start

### 1. Install and Run

```bash
# Using Docker
docker run -d -p 8080:8080 -v maia-data:/data ghcr.io/cuemby/maia:latest

# Or build from source
git clone https://github.com/cuemby/maia
cd maia
go build -o maia ./cmd/maia
./maia
```

### 2. Store a Memory

```bash
curl -X POST http://localhost:8080/v1/memories \
  -H "Content-Type: application/json" \
  -d '{
    "namespace": "default",
    "content": "User prefers dark mode and compact layouts",
    "type": "semantic"
  }'
```

### 3. Retrieve Context

```bash
curl -X POST http://localhost:8080/v1/context \
  -H "Content-Type: application/json" \
  -d '{
    "query": "What are the user preferences?",
    "namespace": "default",
    "token_budget": 2000
  }'
```

---

## Memory Types

MAIA supports three types of memories, each optimized for different use cases:

| Type | Description | Example | Zone Preference |
|------|-------------|---------|-----------------|
| **Semantic** | Facts, profiles, structured knowledge | "User is a senior developer at Acme Corp" | Critical |
| **Episodic** | Conversations, experiences, temporal context | "Yesterday discussed migration strategy" | Middle |
| **Working** | Current session state, transient info | "Currently editing file: main.go" | Recency |

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                     Your Application                         │
│  (Claude Desktop, Cursor, Custom App, OpenAI-compatible)    │
└─────────────────────────────────────────────────────────────┘
                              │
           ┌──────────────────┼──────────────────┐
           │                  │                  │
           ▼                  ▼                  ▼
    ┌────────────┐    ┌────────────┐    ┌────────────┐
    │ MCP Server │    │   Proxy    │    │   SDKs     │
    │  (stdio)   │    │ (OpenAI)   │    │ (Go/TS/Py) │
    └────────────┘    └────────────┘    └────────────┘
           │                  │                  │
           └──────────────────┼──────────────────┘
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                      MAIA Core Engine                        │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │   Query     │  │  Retrieval  │  │ Context Assembly    │  │
│  │  Analyzer   │──│   Layer     │──│ (Position-Aware)    │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
│         │               │                    │              │
│  ┌──────┴───────────────┴────────────────────┴───────┐     │
│  │                   Index Layer                       │     │
│  │  ┌─────────┐  ┌──────────┐  ┌─────────────────┐   │     │
│  │  │ Vector  │  │FullText  │  │     Graph       │   │     │
│  │  │ (HNSW)  │  │ (Bleve)  │  │ (Relationships) │   │     │
│  │  └─────────┘  └──────────┘  └─────────────────┘   │     │
│  └───────────────────────────────────────────────────┘     │
│                          │                                  │
│  ┌───────────────────────┴───────────────────────────┐     │
│  │              Storage Layer (BadgerDB)              │     │
│  └───────────────────────────────────────────────────┘     │
└─────────────────────────────────────────────────────────────┘
```

---

## Performance Targets

MAIA is designed for production workloads with strict latency requirements:

| Operation | Target | Actual |
|-----------|--------|--------|
| Memory write | < 50ms p99 | ~1ms |
| Memory read | < 20ms p99 | ~0.5ms |
| Vector search | < 50ms p99 | ~5ms |
| Context assembly | < 200ms p99 | ~0.3ms |

---

## Next Steps

1. **[Getting Started](getting-started.md)** — Install MAIA and run your first queries
2. **[Architecture](architecture.md)** — Understand how MAIA works internally
3. **[MCP Integration](mcp-integration.md)** — Connect Claude Desktop or Cursor
4. **[SDKs](sdks.md)** — Integrate with Go, TypeScript, or Python

---

## Community & Support

- **GitHub Issues**: [github.com/cuemby/maia/issues](https://github.com/cuemby/maia/issues)
- **Discussions**: [github.com/cuemby/maia/discussions](https://github.com/cuemby/maia/discussions)

---

## License

MAIA is released under the [Apache 2.0 License](../LICENSE).
