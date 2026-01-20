# MCP Integration Guide

MAIA provides a Model Context Protocol (MCP) server that integrates with Claude Desktop, Cursor, and other MCP-compatible clients.

---

## What is MCP?

The [Model Context Protocol](https://modelcontextprotocol.io/) (MCP) is an open standard for connecting AI assistants to external data sources and tools. MAIA's MCP server allows Claude and other AI assistants to:

- **Remember** information during conversations
- **Recall** relevant context automatically
- **Manage** memories and namespaces
- **Access** server statistics

---

## Quick Setup

### Claude Desktop

1. **Start the MAIA MCP server:**

```bash
# Using the binary
maia-mcp-server

# Or using go run
go run ./cmd/mcp-server
```

2. **Configure Claude Desktop:**

Edit `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS) or `%APPDATA%\Claude\claude_desktop_config.json` (Windows):

```json
{
  "mcpServers": {
    "maia": {
      "command": "/path/to/maia-mcp-server",
      "args": [],
      "env": {
        "MAIA_URL": "http://localhost:8080",
        "MAIA_DEFAULT_NAMESPACE": "claude"
      }
    }
  }
}
```

3. **Restart Claude Desktop**

### Cursor

1. **Configure Cursor settings:**

Open Cursor settings and add to `.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "maia": {
      "command": "/path/to/maia-mcp-server",
      "args": [],
      "env": {
        "MAIA_URL": "http://localhost:8080",
        "MAIA_DEFAULT_NAMESPACE": "cursor"
      }
    }
  }
}
```

2. **Restart Cursor**

---

## MCP Server Configuration

The MCP server connects to a running MAIA HTTP server and exposes MAIA functionality through MCP.

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MAIA_URL` | `http://localhost:8080` | MAIA server URL |
| `MAIA_API_KEY` | `` | API key for authentication |
| `MAIA_DEFAULT_NAMESPACE` | `default` | Default namespace for operations |

### Command-Line Options

```bash
maia-mcp-server \
  --url http://localhost:8080 \
  --api-key your-key \
  --namespace default
```

---

## Available Tools

MAIA's MCP server provides five tools:

### 1. remember

Store information in MAIA's memory.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `content` | string | Yes | Information to remember |
| `namespace` | string | No | Target namespace |
| `type` | string | No | `semantic`, `episodic`, `working` |
| `tags` | array | No | Tags for filtering |
| `metadata` | object | No | Custom metadata |

**Example usage in Claude:**
```
Please remember that I prefer using TypeScript over JavaScript for new projects.
```

Claude will call:
```json
{
  "tool": "remember",
  "arguments": {
    "content": "User prefers TypeScript over JavaScript for new projects",
    "type": "semantic",
    "tags": ["preferences", "programming"]
  }
}
```

### 2. recall

Retrieve relevant context based on a query.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `query` | string | Yes | Query to search for |
| `namespace` | string | No | Target namespace |
| `token_budget` | int | No | Max tokens to return |
| `min_score` | float | No | Minimum relevance score |

**Example usage:**
```
What do you know about my programming preferences?
```

Claude will call:
```json
{
  "tool": "recall",
  "arguments": {
    "query": "programming preferences",
    "token_budget": 2000
  }
}
```

### 3. forget

Delete a specific memory.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | string | Yes | Memory ID to delete |

**Example usage:**
```
Please forget memory ID mem_abc123.
```

### 4. list_memories

List memories in a namespace.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `namespace` | string | No | Target namespace |
| `type` | string | No | Filter by type |
| `tags` | array | No | Filter by tags |
| `limit` | int | No | Max results |
| `offset` | int | No | Pagination offset |

**Example usage:**
```
Show me all memories tagged with "project-x".
```

### 5. get_context

Assemble position-aware context (advanced version of recall).

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `query` | string | Yes | Context query |
| `namespace` | string | No | Target namespace |
| `token_budget` | int | No | Token limit |
| `system_prompt` | string | No | System prompt |
| `include_scores` | bool | No | Include scores |

**Returns detailed context with zone statistics:**
```json
{
  "content": "...",
  "memories": [...],
  "zone_stats": {
    "critical_used": 150,
    "middle_used": 500,
    "recency_used": 100
  }
}
```

---

## Available Resources

Resources provide read-only access to MAIA data:

### maia://namespaces

List all available namespaces.

```
List all MAIA namespaces.
```

### maia://namespace/{namespace}/memories

List memories in a specific namespace.

```
Show memories in the "project" namespace.
```

### maia://memory/{id}

Get a specific memory by ID.

```
Show me memory mem_abc123.
```

### maia://stats

Get server statistics.

```
What are the current MAIA statistics?
```

---

## Available Prompts

Prompts are pre-defined templates for common operations:

### inject_context

Inject MAIA context into the conversation.

**Arguments:**
- `query`: What context to retrieve
- `namespace`: Target namespace
- `token_budget`: Token limit

**Usage:**
Claude will automatically use this prompt when it needs context.

### summarize_memories

Summarize memories in a namespace.

**Arguments:**
- `namespace`: Target namespace

**Usage:**
```
Summarize all memories in the "project" namespace.
```

### explore_memories

Explore and understand memories in detail.

**Arguments:**
- `namespace`: Target namespace
- `query`: Optional search query

**Usage:**
```
Help me explore my memories about authentication.
```

---

## Best Practices

### Namespace Organization

Use namespaces to organize memories:

```json
{
  "env": {
    "MAIA_DEFAULT_NAMESPACE": "claude/user-123"
  }
}
```

Suggested namespace hierarchy:
- `claude/user-{id}` - Per-user Claude memories
- `cursor/project-{name}` - Per-project Cursor memories
- `shared/team-{name}` - Shared team knowledge

### Memory Types

Use appropriate memory types:

| Type | Use Case | Example |
|------|----------|---------|
| `semantic` | Facts, preferences, knowledge | "User prefers dark mode" |
| `episodic` | Conversations, experiences | "Discussed migration strategy" |
| `working` | Current session state | "Working on file: main.go" |

### Token Budget

Adjust token budget based on context window:

| Model | Suggested Budget |
|-------|-----------------|
| Claude 3 Haiku | 2000-4000 |
| Claude 3 Sonnet | 4000-8000 |
| Claude 3 Opus | 8000-16000 |

### Tagging Strategy

Use consistent tags:

```json
{
  "tags": ["preferences", "programming/typescript", "project/maia"]
}
```

---

## Integration Examples

### Persistent Project Context

Configure Claude to remember project-specific information:

```json
{
  "mcpServers": {
    "maia": {
      "command": "maia-mcp-server",
      "env": {
        "MAIA_URL": "http://localhost:8080",
        "MAIA_DEFAULT_NAMESPACE": "claude/project-maia"
      }
    }
  }
}
```

Then in Claude:
```
Remember that this project uses Go 1.22, BadgerDB for storage,
and targets sub-200ms context assembly latency.
```

### Multi-User Setup

For teams sharing a MAIA instance:

```json
{
  "mcpServers": {
    "maia": {
      "command": "maia-mcp-server",
      "env": {
        "MAIA_URL": "https://maia.internal.company.com",
        "MAIA_API_KEY": "${MAIA_USER_KEY}",
        "MAIA_DEFAULT_NAMESPACE": "team/${USER}"
      }
    }
  }
}
```

### Development Workflow

Store development context automatically:

1. **At project start:**
```
Remember: Working on feature branch feat/user-auth.
Goal is to implement JWT authentication with refresh tokens.
```

2. **During development:**
```
What do you know about my current task?
```

3. **When stuck:**
```
I'm getting an error with token validation. What context do you have about this?
```

---

## Troubleshooting

### MCP Server Not Starting

```bash
# Check if MAIA server is running
curl http://localhost:8080/health

# Start MCP server with debug logging
MAIA_LOG_LEVEL=debug maia-mcp-server
```

### Claude Not Seeing Tools

1. Verify `claude_desktop_config.json` syntax
2. Check file permissions on the MCP server binary
3. Restart Claude Desktop completely
4. Check Claude Desktop logs: `~/Library/Logs/Claude/`

### Memory Not Being Retrieved

```bash
# Verify memories exist
curl http://localhost:8080/v1/memories/search \
  -H "Content-Type: application/json" \
  -d '{"namespace": "claude", "query": "test"}'

# Check namespace
curl http://localhost:8080/v1/namespaces/claude
```

### Connection Refused

1. Verify MAIA server is running
2. Check `MAIA_URL` environment variable
3. Verify network connectivity (especially in containers)

---

## Security Considerations

### API Key Management

Never hardcode API keys in config files:

```json
{
  "mcpServers": {
    "maia": {
      "command": "maia-mcp-server",
      "env": {
        "MAIA_API_KEY": "${MAIA_API_KEY}"
      }
    }
  }
}
```

Set the key in your shell:
```bash
export MAIA_API_KEY=your-secure-key
```

### Namespace Isolation

Use authorization to restrict access:

```yaml
# MAIA server config
security:
  authorization:
    enabled: true
    api_key_permissions:
      "claude-user-key": ["claude/user-123"]
      "cursor-project-key": ["cursor/project-*"]
```

### Local-Only Access

For development, bind MAIA to localhost:

```yaml
server:
  http_port: 8080
  bind_address: 127.0.0.1
```

---

## Advanced Configuration

### Custom MCP Server Build

Build with custom defaults:

```go
package main

import (
    "os"
    "github.com/cuemby/maia/pkg/mcp"
)

func main() {
    server := mcp.NewServer(mcp.Config{
        BaseURL:   os.Getenv("MAIA_URL"),
        APIKey:    os.Getenv("MAIA_API_KEY"),
        Namespace: "custom-default",
    })
    server.Run()
}
```

### Multiple MAIA Instances

Connect to different MAIA servers:

```json
{
  "mcpServers": {
    "maia-personal": {
      "command": "maia-mcp-server",
      "env": {
        "MAIA_URL": "http://localhost:8080",
        "MAIA_DEFAULT_NAMESPACE": "personal"
      }
    },
    "maia-work": {
      "command": "maia-mcp-server",
      "env": {
        "MAIA_URL": "https://maia.company.com",
        "MAIA_DEFAULT_NAMESPACE": "work"
      }
    }
  }
}
```

---

## Related Documentation

- [Getting Started](getting-started.md) - MAIA installation
- [API Reference](api-reference.md) - Full API documentation
- [Configuration](configuration.md) - Server configuration
- [MCP Specification](https://modelcontextprotocol.io/) - Official MCP docs
