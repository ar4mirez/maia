# Multi-Agent Memory Sharing Example

This example demonstrates how multiple AI agents can share memory through MAIA, enabling collaborative workflows and knowledge transfer between agents.

## Use Cases

- **Team of Specialists**: Different agents handle different domains (code, docs, testing)
- **Handoffs**: One agent's findings become another agent's context
- **Collaborative Problem Solving**: Agents build on each other's discoveries
- **Persistent Knowledge Base**: All agents contribute to shared organizational memory

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Agent A   │     │   Agent B   │     │   Agent C   │
│  (Research) │     │   (Code)    │     │  (Review)   │
└──────┬──────┘     └──────┬──────┘     └──────┬──────┘
       │                   │                   │
       │    ┌──────────────┴──────────────┐   │
       └────►         MAIA Server         ◄───┘
            │  ┌─────────────────────────┐│
            │  │    Shared Namespaces    ││
            │  │  - project:acme         ││
            │  │  - team:engineering     ││
            │  │  - agent:research       ││
            │  └─────────────────────────┘│
            └─────────────────────────────┘
```

## Namespace Strategy

### Hierarchical Namespaces

```
organization/
├── team/
│   ├── project/          # Project-specific memories
│   │   ├── codebase      # Code understanding
│   │   ├── architecture  # Design decisions
│   │   └── issues        # Known issues
│   └── shared/           # Team knowledge
└── agent/
    ├── researcher/       # Research agent's findings
    ├── coder/            # Code agent's context
    └── reviewer/         # Review agent's notes
```

### Access Patterns

- **Write to own namespace**: Each agent writes to its dedicated namespace
- **Read from shared**: All agents can read from project/shared namespaces
- **Promote important findings**: Move discoveries to shared namespaces

## Running the Example

### Prerequisites

1. MAIA server running on `localhost:8080`
2. Go 1.22 or later

### Start the Agents

```bash
# Terminal 1: Research Agent
go run main.go -agent research -namespace project:acme

# Terminal 2: Code Agent
go run main.go -agent code -namespace project:acme

# Terminal 3: Review Agent
go run main.go -agent review -namespace project:acme
```

## Example Workflow

### Step 1: Research Agent Discovers Information

```go
// Research agent stores findings
client.Remember(ctx, "agent:research",
    "The codebase uses React 18 with TypeScript 5.0",
    maia.WithMemoryType("semantic"),
    maia.WithTags("tech-stack", "frontend"))

client.Remember(ctx, "agent:research",
    "Authentication is handled via JWT tokens stored in httpOnly cookies",
    maia.WithMemoryType("semantic"),
    maia.WithTags("auth", "security"))
```

### Step 2: Code Agent Uses Research

```go
// Code agent recalls research findings
context, _ := client.Recall(ctx,
    "What authentication method is used?",
    maia.WithNamespace("agent:research"))

// Now the code agent knows about JWT + httpOnly cookies
// and can write compatible code
```

### Step 3: Code Agent Shares Implementation

```go
// Code agent stores implementation details
client.Remember(ctx, "project:acme",
    "Created AuthProvider component at src/providers/AuthProvider.tsx",
    maia.WithMemoryType("episodic"),
    maia.WithTags("implementation", "auth"))
```

### Step 4: Review Agent Has Full Context

```go
// Review agent gets complete context
context, _ := client.Recall(ctx,
    "Review the authentication implementation",
    maia.WithNamespace("project:acme"),
    maia.WithTokenBudget(4000))

// Context includes:
// - Research findings about JWT + httpOnly cookies
// - Implementation details about AuthProvider
// - Any related project memories
```

## Memory Types for Multi-Agent

| Type | Use Case | Example |
|------|----------|---------|
| `semantic` | Factual knowledge | "The API uses REST with JSON" |
| `episodic` | Events/actions | "Fixed the login bug in commit abc123" |
| `working` | Temporary context | "Currently investigating memory leak" |

## API Keys and Authorization

Configure per-agent access:

```yaml
auth:
  enabled: true
  api_keys:
    - key: "research-agent-key"
      namespaces: ["agent:research", "project:*"]
    - key: "code-agent-key"
      namespaces: ["agent:code", "project:*"]
    - key: "review-agent-key"
      namespaces: ["*"]  # Reviewers can read everything
```

## Best Practices

### 1. Clear Ownership

Each agent should have a dedicated namespace for its findings:
- Prevents conflicts
- Enables tracing back to source
- Supports access control

### 2. Promote Important Findings

Move discoveries from agent namespaces to shared namespaces:
```go
// Research agent finds something important
mem, _ := client.Remember(ctx, "agent:research", finding)

// Later, promote to project namespace
client.Remember(ctx, "project:acme", finding,
    maia.WithMetadata(map[string]interface{}{
        "source": "research-agent",
        "original_id": mem.ID,
    }))
```

### 3. Use Tags for Cross-Cutting Concerns

```go
// All agents use consistent tags
tags := []string{"auth", "security", "p0-priority"}
```

### 4. Time-Bound Working Memory

```go
// Working memory for current task
client.Remember(ctx, "agent:code",
    "Investigating flaky test in auth module",
    maia.WithMemoryType("working"))

// Clean up when done
client.Forget(ctx, workingMemoryID)
```

## Monitoring

Track multi-agent activity:

```bash
# Get per-namespace stats
curl http://localhost:8080/v1/namespaces/project:acme

# View recent memories across agents
curl "http://localhost:8080/v1/memories/search" \
  -H "Content-Type: application/json" \
  -d '{"namespace": "project:acme", "limit": 20}'
```

## Related Examples

- [basic-usage](../basic-usage/) - Core MAIA functionality
- [mcp-integration](../mcp-integration/) - MCP server setup
- [proxy-usage](../proxy-usage/) - OpenAI proxy for memory injection
