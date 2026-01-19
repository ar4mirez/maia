# MAIA - Memory AI Architecture

An AI-native distributed memory system that acts as an intelligent interceptor between applications and LLMs. MAIA solves the fundamental problem of context window limitations by intelligently managing what information goes into that window.

## Key Features

- **Distributed-first architecture** - Designed for multi-agent, multi-instance deployments
- **Position-aware context assembly** - Solves the "context rot" problem (20-30% accuracy variance)
- **Sub-200ms latency target** - Production-grade performance
- **Flexible namespace model** - Per-user, per-org, or custom hierarchies
- **Multiple integration patterns** - MCP Server, OpenAI-compatible proxy, native SDKs

## Quick Start

### Prerequisites

- Go 1.22 or later

### Installation

```bash
# Clone the repository
git clone https://github.com/ar4mirez/maia.git
cd maia

# Build
make build

# Or run directly
go run ./cmd/maia
```

### Configuration

Copy the example environment file and configure:

```bash
cp .env.example .env
# Edit .env with your settings
```

Key environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `MAIA_DATA_DIR` | `./data` | Storage directory |
| `MAIA_HTTP_PORT` | `8080` | HTTP API port |
| `MAIA_GRPC_PORT` | `9090` | gRPC API port |
| `MAIA_LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `MAIA_DEFAULT_NAMESPACE` | `default` | Default namespace for operations |

### Running

```bash
# Development mode
make dev

# Or with the built binary
./build/maia
```

## API Usage

### Health Check

```bash
curl http://localhost:8080/health
```

### Create a Memory

```bash
curl -X POST http://localhost:8080/v1/memories \
  -H "Content-Type: application/json" \
  -d '{
    "namespace": "default",
    "content": "The user prefers dark mode",
    "type": "semantic",
    "tags": ["preference", "ui"]
  }'
```

### Search Memories

```bash
curl -X POST http://localhost:8080/v1/memories/search \
  -H "Content-Type: application/json" \
  -d '{
    "namespace": "default",
    "tags": ["preference"]
  }'
```

### Create a Namespace

```bash
curl -X POST http://localhost:8080/v1/namespaces \
  -H "Content-Type: application/json" \
  -d '{
    "name": "user:john",
    "config": {
      "token_budget": 4000
    }
  }'
```

### Get Context (Assembled)

```bash
curl -X POST http://localhost:8080/v1/context \
  -H "Content-Type: application/json" \
  -d '{
    "query": "What are the user preferences?",
    "namespace": "default",
    "token_budget": 4000
  }'
```

## Architecture

```
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
       │  └─────────────────────────────────┘│
       │                  │                  │
       │  ┌─────────────────────────────────┐│
       │  │     Memory Retrieval Layer      ││
       │  └─────────────────────────────────┘│
       │                  │                  │
       │  ┌─────────┬─────┴─────┬───────────┐│
       │  │Semantic │ Episodic  │  Working  ││
       │  │ Store   │  Store    │   Store   ││
       │  └─────────┴───────────┴───────────┘│
       │                  │                  │
       │  ┌─────────────────────────────────┐│
       │  │   Context Assembly Layer        ││
       │  └─────────────────────────────────┘│
       └─────────────────────────────────────┘
                         │
                         ▼
                   Target LLM
```

## Development

### Running Tests

```bash
make test
```

### Building

```bash
# Build all binaries
make build

# Build specific binary
make build-server
```

### Code Quality

```bash
# Format code
make fmt

# Run linters
make lint

# All checks
make check
```

## Project Structure

```
maia/
├── cmd/
│   ├── maia/           # Main server binary
│   ├── maiactl/        # CLI tool
│   └── mcp-server/     # Standalone MCP server
├── internal/
│   ├── config/         # Configuration management
│   ├── server/         # HTTP/gRPC server
│   ├── storage/        # Storage layer (BadgerDB)
│   └── ...
├── pkg/
│   ├── maia/           # Go SDK (public)
│   ├── mcp/            # MCP implementation
│   └── proxy/          # OpenAI proxy
└── sdk/
    ├── typescript/     # TypeScript SDK
    └── python/         # Python SDK
```

## Roadmap

- [x] **Phase 1**: Foundation (storage, basic API)
- [ ] **Phase 2**: Intelligence (embeddings, vector search)
- [ ] **Phase 3**: Context Assembly (position-aware optimization)
- [ ] **Phase 4**: MCP Integration
- [ ] **Phase 5**: Proxy + SDKs
- [ ] **Phase 6**: Production Hardening

## Contributing

Contributions are welcome! Please read the contributing guidelines before submitting PRs.

## License

[MIT License](LICENSE)
