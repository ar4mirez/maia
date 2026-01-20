# MAIA CLI Reference

`maiactl` is the command-line interface for managing MAIA servers, memories, and namespaces.

---

## Installation

### From Binary

```bash
# Download the latest release
curl -LO https://github.com/cuemby/maia/releases/latest/download/maiactl-$(uname -s)-$(uname -m)
chmod +x maiactl-*
sudo mv maiactl-* /usr/local/bin/maiactl
```

### From Source

```bash
go install github.com/cuemby/maia/cmd/maiactl@latest
```

### Verify Installation

```bash
maiactl version
```

---

## Configuration

### Server URL

Set the MAIA server URL:

```bash
# Environment variable (recommended)
export MAIA_URL=http://localhost:8080

# Or use --server flag
maiactl --server http://localhost:8080 stats
```

### API Key

If authentication is enabled:

```bash
# Environment variable
export MAIA_API_KEY=your-api-key

# Or use --api-key flag
maiactl --api-key your-key stats
```

### Output Format

Get JSON output for scripting:

```bash
maiactl --json memory list -n default
```

---

## Global Flags

| Flag | Short | Environment | Description |
|------|-------|-------------|-------------|
| `--server` | `-s` | `MAIA_URL` | MAIA server URL |
| `--api-key` | | `MAIA_API_KEY` | API key for authentication |
| `--json` | `-j` | | Output in JSON format |
| `--help` | `-h` | | Show help |

---

## Commands

### memory

Manage memories.

#### memory create

Create a new memory.

```bash
maiactl memory create [flags]
```

**Flags:**
| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--namespace` | `-n` | Yes | Target namespace |
| `--content` | `-c` | Yes | Memory content |
| `--type` | `-t` | No | Type: semantic, episodic, working |
| `--tags` | | No | Comma-separated tags |
| `--metadata` | `-m` | No | JSON metadata |
| `--confidence` | | No | Confidence score (0.0-1.0) |

**Examples:**

```bash
# Create a semantic memory
maiactl memory create \
  -n default \
  -c "User prefers dark mode and compact layouts" \
  -t semantic \
  --tags "preferences,ui"

# Create with metadata
maiactl memory create \
  -n default \
  -c "Meeting notes from standup" \
  -t episodic \
  -m '{"source": "meeting", "date": "2026-01-19"}'

# Create working memory
maiactl memory create \
  -n session \
  -c "Currently editing file: main.go" \
  -t working
```

#### memory list

List memories in a namespace.

```bash
maiactl memory list [flags]
```

**Flags:**
| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--namespace` | `-n` | No | Target namespace (default: all) |
| `--type` | `-t` | No | Filter by type |
| `--tags` | | No | Filter by tags |
| `--limit` | `-l` | No | Max results (default: 100) |
| `--offset` | `-o` | No | Pagination offset |

**Examples:**

```bash
# List all memories in default namespace
maiactl memory list -n default

# List semantic memories only
maiactl memory list -n default -t semantic

# List with pagination
maiactl memory list -n default -l 20 -o 40

# JSON output for scripting
maiactl -j memory list -n default
```

**Output:**
```
ID                    NAMESPACE   TYPE      CONTENT                            CREATED
mem_01HQ5K3ABC...    default     semantic  User prefers dark mode and...     2026-01-19 10:00:00
mem_01HQ5K4DEF...    default     episodic  Meeting notes from standup...     2026-01-19 11:30:00
```

#### memory get

Get a memory by ID.

```bash
maiactl memory get <id>
```

**Examples:**

```bash
maiactl memory get mem_01HQ5K3ABC

# JSON output
maiactl -j memory get mem_01HQ5K3ABC
```

**Output:**
```
ID:          mem_01HQ5K3ABC
Namespace:   default
Type:        semantic
Content:     User prefers dark mode and compact layouts
Tags:        preferences, ui
Confidence:  0.90
Created:     2026-01-19 10:00:00
Updated:     2026-01-19 10:00:00
Accessed:    2026-01-19 14:30:00
Access Count: 5
```

#### memory update

Update an existing memory.

```bash
maiactl memory update <id> [flags]
```

**Flags:**
| Flag | Short | Description |
|------|-------|-------------|
| `--content` | `-c` | New content |
| `--tags` | | New tags (replaces existing) |
| `--metadata` | `-m` | JSON metadata (merged) |
| `--confidence` | | New confidence score |

**Examples:**

```bash
# Update content
maiactl memory update mem_01HQ5K3ABC \
  -c "User prefers dark mode, compact layouts, and monospace fonts"

# Update tags
maiactl memory update mem_01HQ5K3ABC \
  --tags "preferences,ui,fonts"

# Update metadata
maiactl memory update mem_01HQ5K3ABC \
  -m '{"updated_by": "user"}'
```

#### memory delete

Delete a memory.

```bash
maiactl memory delete <id>
```

**Examples:**

```bash
maiactl memory delete mem_01HQ5K3ABC

# Delete multiple (using shell)
for id in mem_01 mem_02 mem_03; do
  maiactl memory delete $id
done
```

#### memory search

Search memories.

```bash
maiactl memory search [flags]
```

**Flags:**
| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--query` | `-q` | Yes | Search query |
| `--namespace` | `-n` | No | Target namespace |
| `--types` | `-t` | No | Filter by types (comma-separated) |
| `--tags` | | No | Filter by tags |
| `--limit` | `-l` | No | Max results (default: 20) |
| `--min-score` | | No | Minimum relevance score |

**Examples:**

```bash
# Basic search
maiactl memory search -q "user preferences" -n default

# Search with filters
maiactl memory search \
  -q "dark mode" \
  -n default \
  -t semantic,episodic \
  --tags "preferences"

# Search with minimum score
maiactl memory search -q "authentication" --min-score 0.5
```

**Output:**
```
SCORE   ID                    TYPE      CONTENT
0.92    mem_01HQ5K3ABC...    semantic  User prefers dark mode and...
0.78    mem_01HQ5K4DEF...    episodic  Discussed UI preferences...
0.65    mem_01HQ5K5GHI...    working   Working on settings page...
```

---

### namespace

Manage namespaces.

#### namespace create

Create a new namespace.

```bash
maiactl namespace create <name> [flags]
```

**Flags:**
| Flag | Short | Description |
|------|-------|-------------|
| `--parent` | `-p` | Parent namespace |
| `--token-budget` | `-b` | Token budget (default: 4000) |
| `--max-memories` | | Maximum memories |
| `--retention-days` | | Auto-delete after N days |

**Examples:**

```bash
# Create basic namespace
maiactl namespace create my-project

# Create with config
maiactl namespace create my-project \
  --token-budget 8000 \
  --max-memories 50000

# Create hierarchical namespace
maiactl namespace create project-alpha -p team-a

# Create with retention
maiactl namespace create temp-session --retention-days 7
```

#### namespace list

List all namespaces.

```bash
maiactl namespace list [flags]
```

**Flags:**
| Flag | Short | Description |
|------|-------|-------------|
| `--parent` | `-p` | Filter by parent |

**Examples:**

```bash
# List all namespaces
maiactl namespace list

# List child namespaces
maiactl namespace list -p team-a

# JSON output
maiactl -j namespace list
```

**Output:**
```
NAME            PARENT    MEMORIES   TOKENS     CREATED
default         -         150        45,000     2026-01-15 09:00:00
team-a          -         50         12,000     2026-01-16 10:30:00
team-a/alpha    team-a    25         8,000      2026-01-17 14:00:00
```

#### namespace get

Get namespace details.

```bash
maiactl namespace get <name>
```

**Examples:**

```bash
maiactl namespace get my-project
```

**Output:**
```
Name:           my-project
Parent:         -
Token Budget:   8000
Max Memories:   50000
Retention:      -
Memory Count:   150
Total Tokens:   45,000
Created:        2026-01-15 09:00:00
Updated:        2026-01-19 10:00:00
```

#### namespace update

Update a namespace.

```bash
maiactl namespace update <name> [flags]
```

**Flags:**
| Flag | Short | Description |
|------|-------|-------------|
| `--token-budget` | `-b` | New token budget |
| `--max-memories` | | New max memories |
| `--retention-days` | | New retention policy |

**Examples:**

```bash
maiactl namespace update my-project --token-budget 10000
```

#### namespace delete

Delete a namespace.

```bash
maiactl namespace delete <name> [flags]
```

**Flags:**
| Flag | Short | Description |
|------|-------|-------------|
| `--force` | `-f` | Delete even if not empty |

**Examples:**

```bash
# Delete empty namespace
maiactl namespace delete temp-session

# Force delete with all memories
maiactl namespace delete old-project -f
```

---

### context

Assemble context from memories.

```bash
maiactl context <query> [flags]
```

**Flags:**
| Flag | Short | Description |
|------|-------|-------------|
| `--namespace` | `-n` | Target namespace |
| `--budget` | `-b` | Token budget (default: 4000) |
| `--system-prompt` | `-s` | System prompt to include |
| `--min-score` | | Minimum relevance score |
| `--show-zones` | `-z` | Show zone statistics |

**Examples:**

```bash
# Basic context assembly
maiactl context "What are the user preferences?" -n default

# With token budget
maiactl context "project requirements" -n project -b 8000

# With system prompt
maiactl context "help with authentication" \
  -n default \
  -s "You are a senior developer helping with code."

# Show zone statistics
maiactl context "user preferences" -n default -z
```

**Output:**
```
CONTEXT ASSEMBLY
================
Query:        What are the user preferences?
Namespace:    default
Token Budget: 4000
Tokens Used:  1,250

MEMORIES INCLUDED (3)
---------------------
[CRITICAL] User prefers dark mode and compact layouts (score: 0.92)
[MIDDLE]   Discussed UI preferences in last meeting (score: 0.71)
[RECENCY]  Currently working on settings page (score: 0.45)

ZONE STATISTICS
---------------
Critical:  300/600 tokens (50%)
Middle:    750/2600 tokens (29%)
Recency:   200/800 tokens (25%)

ASSEMBLED CONTENT
-----------------
## Relevant Context

User prefers dark mode and compact layouts. They value clean interfaces
and have expressed interest in customizable themes.

In our last meeting, we discussed various UI preferences including...

Currently working on the settings page to implement these preferences.
```

**JSON Output:**
```bash
maiactl -j context "user preferences" -n default
```

```json
{
  "content": "## Relevant Context\n\nUser prefers dark mode...",
  "memories": [
    {
      "id": "mem_01HQ5K3ABC",
      "content": "User prefers dark mode and compact layouts",
      "type": "semantic",
      "score": 0.92,
      "position": "critical",
      "token_count": 45
    }
  ],
  "token_count": 1250,
  "token_budget": 4000,
  "zone_stats": {
    "critical_used": 300,
    "critical_budget": 600,
    "middle_used": 750,
    "middle_budget": 2600,
    "recency_used": 200,
    "recency_budget": 800
  }
}
```

---

### stats

Show server statistics.

```bash
maiactl stats
```

**Output:**
```
MAIA SERVER STATISTICS
======================

Storage
-------
Memory Count:     15,000
Namespace Count:  12
Total Size:       15.0 MB
Index Size:       5.0 MB

Server
------
Uptime:           1d 2h 30m
Total Requests:   150,000
Requests/sec:     1.74

Performance
-----------
Avg Write Latency:   1.2ms
Avg Read Latency:    0.5ms
Avg Search Latency:  5.3ms
Avg Context Latency: 45.2ms
```

**JSON Output:**
```bash
maiactl -j stats
```

---

### version

Show version information.

```bash
maiactl version
```

**Output:**
```
maiactl version 1.0.0
  Git Commit: abc1234
  Build Date: 2026-01-19
  Go Version: go1.22.0
  OS/Arch:    darwin/arm64
```

---

## Scripting Examples

### Backup All Memories

```bash
#!/bin/bash
# backup-memories.sh

NAMESPACE=${1:-default}
OUTPUT="backup-$(date +%Y%m%d).json"

maiactl -j memory list -n "$NAMESPACE" -l 10000 > "$OUTPUT"
echo "Backed up to $OUTPUT"
```

### Import Memories

```bash
#!/bin/bash
# import-memories.sh

NAMESPACE=$1
FILE=$2

cat "$FILE" | jq -c '.memories[]' | while read -r memory; do
  content=$(echo "$memory" | jq -r '.content')
  type=$(echo "$memory" | jq -r '.type')
  tags=$(echo "$memory" | jq -r '.tags | join(",")')

  maiactl memory create \
    -n "$NAMESPACE" \
    -c "$content" \
    -t "$type" \
    --tags "$tags"
done
```

### Cleanup Old Memories

```bash
#!/bin/bash
# cleanup-old.sh

# Get memories older than 30 days
CUTOFF=$(date -v-30d +%Y-%m-%dT%H:%M:%SZ)

maiactl -j memory list -n default | \
  jq -r ".memories[] | select(.created_at < \"$CUTOFF\") | .id" | \
  while read -r id; do
    echo "Deleting $id"
    maiactl memory delete "$id"
  done
```

### Monitor Server Health

```bash
#!/bin/bash
# health-check.sh

while true; do
  response=$(curl -s http://localhost:8080/health)
  status=$(echo "$response" | jq -r '.status')

  if [ "$status" != "ok" ]; then
    echo "ALERT: MAIA health check failed"
    # Send alert...
  fi

  sleep 60
done
```

### Bulk Search

```bash
#!/bin/bash
# bulk-search.sh

QUERIES=("user preferences" "project requirements" "authentication")

for query in "${QUERIES[@]}"; do
  echo "=== Searching: $query ==="
  maiactl memory search -q "$query" -n default --limit 5
  echo
done
```

---

## Shell Completion

### Bash

```bash
# Add to ~/.bashrc
source <(maiactl completion bash)
```

### Zsh

```bash
# Add to ~/.zshrc
source <(maiactl completion zsh)
```

### Fish

```bash
maiactl completion fish | source
```

---

## Related Documentation

- [Getting Started](getting-started.md) - Installation and setup
- [API Reference](api-reference.md) - REST API documentation
- [Configuration](configuration.md) - Server configuration
