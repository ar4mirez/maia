# MCP Integration Example

This example demonstrates how to integrate MAIA as an MCP (Model Context Protocol) server with Claude, Cursor, and other MCP-compatible tools.

## What is MCP?

The Model Context Protocol (MCP) is an open standard for connecting AI assistants to external data sources and tools. MAIA implements MCP to provide persistent memory capabilities to any MCP-compatible client.

## Running the MCP Server

### Standalone Mode

```bash
# From the project root
go run ./cmd/mcp-server
```

The MCP server communicates via stdio (standard input/output), which is the standard transport for MCP.

### With Claude Desktop

Add MAIA to your Claude Desktop configuration:

**macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
**Windows**: `%APPDATA%\Claude\claude_desktop_config.json`

```json
{
  "mcpServers": {
    "maia": {
      "command": "/path/to/maia/mcp-server",
      "args": [],
      "env": {
        "MAIA_DATA_DIR": "/path/to/maia/data",
        "MAIA_DEFAULT_NAMESPACE": "claude"
      }
    }
  }
}
```

### With Cursor

Add to your Cursor MCP configuration:

```json
{
  "mcpServers": {
    "maia": {
      "command": "go",
      "args": ["run", "./cmd/mcp-server"],
      "cwd": "/path/to/maia"
    }
  }
}
```

## Available Tools

MAIA exposes the following MCP tools:

### remember

Store a new memory.

**Arguments:**
- `namespace` (string, required): Target namespace
- `content` (string, required): Memory content
- `type` (string, optional): Memory type (semantic, episodic, working)
- `tags` (array, optional): Tags for categorization

**Example:**
```
Use the remember tool to store: "User prefers TypeScript over JavaScript"
with namespace "project" and tags ["preference", "language"]
```

### recall

Retrieve relevant context for a query.

**Arguments:**
- `query` (string, required): The query to match against
- `namespace` (string, optional): Target namespace
- `token_budget` (number, optional): Maximum tokens to return
- `system_prompt` (string, optional): System prompt to prepend

**Example:**
```
Use the recall tool to get context for: "What programming languages does the user prefer?"
```

### forget

Delete a memory by ID.

**Arguments:**
- `id` (string, required): Memory ID to delete

### list_memories

List all memories in a namespace.

**Arguments:**
- `namespace` (string, required): Target namespace
- `limit` (number, optional): Maximum memories to return
- `offset` (number, optional): Pagination offset

### get_context

Get fully assembled context with position optimization.

**Arguments:**
- `query` (string, required): The query for context assembly
- `namespace` (string, optional): Target namespace
- `token_budget` (number, optional): Token budget

## Available Resources

### namespaces

List all available namespaces.

**URI:** `maia://namespaces`

### memories

List memories in a namespace.

**URI:** `maia://memories?namespace={namespace}`

### memory

Get a specific memory.

**URI:** `maia://memory/{id}`

### stats

Get server statistics.

**URI:** `maia://stats`

## Available Prompts

### inject_context

A prompt template for injecting MAIA context into conversations.

**Arguments:**
- `query` (string, required): The user's query
- `namespace` (string, optional): Target namespace

### summarize_memories

A prompt for summarizing all memories in a namespace.

**Arguments:**
- `namespace` (string, required): Namespace to summarize

### explore_memories

A prompt for exploring and understanding stored memories.

**Arguments:**
- `namespace` (string, required): Namespace to explore

## Usage Patterns

### Building Context-Aware Assistants

With MAIA + MCP, Claude can:

1. **Remember user preferences** across sessions
2. **Recall relevant context** automatically
3. **Build up knowledge** over time

Example conversation:
```
User: Remember that I'm working on a Next.js project called "acme-dashboard"

Claude: [Uses remember tool] I've stored that you're working on a Next.js
        project called "acme-dashboard".

User: What project am I working on?

Claude: [Uses recall tool] Based on my memory, you're working on a Next.js
        project called "acme-dashboard".
```

### Multi-Namespace Organization

Use namespaces to organize memories:

```
- user:{user_id}       # Per-user preferences
- project:{project}    # Project-specific context
- session:{session_id} # Session-specific working memory
- org:{org_id}         # Organization-wide knowledge
```

### Context Window Optimization

MAIA's position-aware assembly ensures:
- High-relevance memories appear at the start
- Recent context is preserved at the end
- Token budget is respected
- LLM accuracy is optimized (addresses "context rot")

## Configuration

Environment variables for the MCP server:

| Variable | Default | Description |
|----------|---------|-------------|
| `MAIA_DATA_DIR` | `./data` | Storage directory |
| `MAIA_DEFAULT_NAMESPACE` | `default` | Default namespace |
| `MAIA_LOG_LEVEL` | `info` | Log level |

## Troubleshooting

### Server not connecting

1. Check that the mcp-server binary path is correct
2. Ensure the data directory is writable
3. Check logs for errors

### Memories not persisting

1. Verify `MAIA_DATA_DIR` is set and writable
2. Check that the namespace exists
3. Ensure memories are being created (check with list_memories)

### Context not relevant

1. Use more specific queries
2. Add relevant tags to memories
3. Adjust token budget for more context
4. Check memory types (semantic vs episodic vs working)
