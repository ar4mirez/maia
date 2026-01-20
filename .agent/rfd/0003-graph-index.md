# RFD 0003: Graph Index Implementation

**Status**: Accepted
**Author**: MAIA Team
**Created**: 2026-01-19
**Updated**: 2026-01-19

---

## Summary

Implement a graph index for MAIA that enables relationship-based memory retrieval. This allows discovering related memories through explicit connections (e.g., "related_to", "follows", "references") and enables graph traversal queries that complement vector and full-text search.

---

## Problem Statement

MAIA currently supports vector similarity search and full-text search, which work well for finding semantically similar or keyword-matching memories. However, some use cases require explicit relationship-based retrieval:

1. **Causal chains**: "What led to this decision?" requires following "caused_by" relationships
2. **Reference networks**: Finding all memories that reference a specific fact
3. **Temporal sequences**: Following "follows" or "precedes" relationships
4. **Topic hierarchies**: Navigating "broader_than" / "narrower_than" relationships
5. **User sessions**: Linking memories from the same conversation or session

**Requirements**:
1. Support directed edges with relationship types
2. Support bidirectional traversal (outgoing and incoming edges)
3. Sub-10ms latency for single-hop traversals
4. Support multi-hop traversals with depth limits
5. Persist to disk and recover on restart
6. Thread-safe concurrent access
7. Integrate with existing retrieval scoring

---

## Options Considered

### Option A: In-Memory Adjacency List (Recommended)

Use a simple in-memory adjacency list with mutex-protected access.

**Pros**:
- Simple implementation
- Fast O(1) edge lookups
- Low memory overhead for sparse graphs
- Easy to serialize/deserialize
- No external dependencies

**Cons**:
- All data in memory (limited by RAM)
- No advanced graph algorithms built-in

**Implementation**:
```go
type GraphIndex struct {
    // Forward edges: source -> [(target, relation)]
    outgoing map[string][]Edge
    // Reverse edges: target -> [(source, relation)]
    incoming map[string][]Edge
    mu       sync.RWMutex
}

type Edge struct {
    TargetID string
    Relation string
    Weight   float32
    Metadata map[string]string
}
```

### Option B: External Graph Database (Dgraph/Neo4j)

Use an external graph database for storage and traversal.

**Pros**:
- Advanced graph algorithms (shortest path, PageRank, etc.)
- Scales beyond memory limits
- Query language (Cypher/DQL)

**Cons**:
- External dependency (violates single-binary goal)
- Network latency for queries
- Complex setup and maintenance
- Overkill for typical MAIA workloads

### Option C: Embedded Graph Database (Cayley)

Use Cayley as an embedded graph database.

**Pros**:
- Embedded (single binary possible)
- Multiple backend options
- Graph query language

**Cons**:
- Additional dependency
- Limited community/maintenance
- Complex API for simple use cases

---

## Decision

**Option A: In-Memory Adjacency List**

Rationale:
1. Keeps MAIA as a single-binary deployment
2. Simple to implement and maintain
3. Sufficient for typical memory graph sizes (10k-100k memories)
4. Fast traversals without network overhead
5. Easy to extend with more algorithms later

---

## Design

### Interface

```go
// Index defines the interface for graph-based memory relationships.
type Index interface {
    // AddEdge creates a directed edge between two memories.
    AddEdge(ctx context.Context, sourceID, targetID, relation string, weight float32) error

    // AddEdgeWithMetadata creates an edge with additional metadata.
    AddEdgeWithMetadata(ctx context.Context, edge *Edge) error

    // RemoveEdge removes a specific edge.
    RemoveEdge(ctx context.Context, sourceID, targetID, relation string) error

    // RemoveNode removes all edges to/from a node.
    RemoveNode(ctx context.Context, id string) error

    // GetOutgoing returns all outgoing edges from a node.
    GetOutgoing(ctx context.Context, id string) ([]Edge, error)

    // GetIncoming returns all incoming edges to a node.
    GetIncoming(ctx context.Context, id string) ([]Edge, error)

    // GetRelated returns related nodes with optional relation filter.
    GetRelated(ctx context.Context, id string, opts *TraversalOptions) ([]TraversalResult, error)

    // Traverse performs multi-hop graph traversal.
    Traverse(ctx context.Context, startID string, opts *TraversalOptions) ([]TraversalResult, error)

    // HasEdge checks if a specific edge exists.
    HasEdge(ctx context.Context, sourceID, targetID, relation string) bool

    // Size returns the total number of edges.
    Size() int

    // NodeCount returns the number of nodes with edges.
    NodeCount() int

    // Save persists the index to a writer.
    Save(w io.Writer) error

    // Load restores the index from a reader.
    Load(r io.Reader) error

    // Close releases resources.
    Close() error
}
```

### Types

```go
// Edge represents a directed relationship between memories.
type Edge struct {
    SourceID  string            // Source memory ID
    TargetID  string            // Target memory ID
    Relation  string            // Relationship type (e.g., "related_to", "follows")
    Weight    float32           // Edge weight (0-1, for scoring)
    Metadata  map[string]string // Optional metadata
    CreatedAt time.Time         // When the edge was created
}

// TraversalOptions configures graph traversal.
type TraversalOptions struct {
    // Relations filters by relationship types (empty = all).
    Relations []string

    // Direction specifies traversal direction.
    Direction Direction

    // MaxDepth limits traversal depth (1 = direct neighbors only).
    MaxDepth int

    // MaxResults limits total results.
    MaxResults int

    // MinWeight filters edges by minimum weight.
    MinWeight float32
}

// Direction specifies edge traversal direction.
type Direction int

const (
    DirectionOutgoing Direction = iota
    DirectionIncoming
    DirectionBoth
)

// TraversalResult represents a node found during traversal.
type TraversalResult struct {
    ID       string  // Memory ID
    Depth    int     // Distance from start node
    Relation string  // Relationship that led here
    Weight   float32 // Edge weight
    Path     []string // Path from start to this node
}
```

### Standard Relationship Types

Define standard relationship types for consistency:

```go
const (
    RelationRelatedTo  = "related_to"  // General relation
    RelationReferences = "references"  // Explicitly references
    RelationFollows    = "follows"     // Temporal sequence
    RelationCausedBy   = "caused_by"   // Causal chain
    RelationPartOf     = "part_of"     // Containment
    RelationSameAs     = "same_as"     // Equivalence
)
```

### Persistence Format

Binary format with magic number for integrity checking:

```
[Magic: 4 bytes = "MAIG"]
[Version: 2 bytes]
[Edge Count: 4 bytes]
[Edges...]
  [SourceID len: 2 bytes][SourceID: N bytes]
  [TargetID len: 2 bytes][TargetID: N bytes]
  [Relation len: 2 bytes][Relation: N bytes]
  [Weight: 4 bytes]
  [Metadata count: 2 bytes]
  [Metadata entries...]
    [Key len: 2 bytes][Key: N bytes]
    [Value len: 2 bytes][Value: N bytes]
  [CreatedAt: 8 bytes]
```

### Integration with Retrieval

Add graph score to the retriever's combined score:

```go
type Config struct {
    // ... existing weights
    GraphWeight float64 // Weight for graph connectivity scores
}

// In Retriever
func (r *Retriever) graphScore(ctx context.Context, candidateID string, queryAnalysis *query.Analysis) float64 {
    // If query analysis identified related memories, boost connected nodes
    if queryAnalysis != nil && len(queryAnalysis.EntityIDs) > 0 {
        for _, entityID := range queryAnalysis.EntityIDs {
            if r.graphIndex.HasEdge(ctx, entityID, candidateID, "") {
                return 1.0 // Strong connection
            }
        }
    }
    return 0.0
}
```

---

## Implementation Plan

### Phase 1: Core Types and Interface
- [ ] Define Edge, TraversalOptions, TraversalResult types
- [ ] Define Index interface
- [ ] Add common errors

### Phase 2: In-Memory Implementation
- [ ] Implement adjacency list storage
- [ ] Implement AddEdge/RemoveEdge/RemoveNode
- [ ] Implement GetOutgoing/GetIncoming
- [ ] Implement HasEdge
- [ ] Add thread safety with RWMutex

### Phase 3: Graph Traversal
- [ ] Implement single-hop GetRelated
- [ ] Implement multi-hop Traverse with BFS
- [ ] Add depth and result limits
- [ ] Add cycle detection

### Phase 4: Persistence
- [ ] Implement Save method
- [ ] Implement Load method
- [ ] Add magic number validation
- [ ] Add version handling

### Phase 5: Testing
- [ ] Unit tests for all methods
- [ ] Edge cases (empty graph, cycles, disconnected nodes)
- [ ] Benchmark tests for traversal performance
- [ ] Integration tests with retriever

### Phase 6: Retriever Integration
- [ ] Add GraphIndex to Retriever struct
- [ ] Add GraphWeight to Config
- [ ] Implement graph scoring in Retrieve
- [ ] Update combined score calculation

---

## Test Coverage Requirements

| Component | Target |
|-----------|--------|
| Edge operations | 95% |
| Traversal | 90% |
| Persistence | 90% |
| Overall graph package | 85% |

---

## Performance Targets

| Operation | Target |
|-----------|--------|
| AddEdge | < 1ms |
| RemoveEdge | < 1ms |
| GetOutgoing/Incoming | < 1ms |
| Single-hop GetRelated | < 5ms |
| 3-hop Traverse (100 results) | < 50ms |
| Save (10k edges) | < 100ms |
| Load (10k edges) | < 100ms |

---

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Memory growth with large graphs | Monitor edge count, document limits |
| Slow traversal for dense graphs | MaxResults limit, depth limits |
| Cycles causing infinite loops | Track visited nodes during traversal |
| Orphaned edges after memory deletion | Hook into storage delete to clean up |

---

## Future Enhancements

1. **PageRank-style scoring**: Use graph structure to boost important memories
2. **Community detection**: Group related memories automatically
3. **Path finding**: Find shortest/strongest path between memories
4. **Automatic edge inference**: Use embeddings to suggest relationships
5. **Edge expiry**: Time-to-live for temporary relationships

---

## Appendix: Use Cases

### Use Case 1: Session Context
```go
// Link all memories from a conversation
for _, mem := range sessionMemories {
    graph.AddEdge(ctx, sessionID, mem.ID, "contains", 1.0)
}

// Retrieve session context
results, _ := graph.GetRelated(ctx, sessionID, &TraversalOptions{
    Relations: []string{"contains"},
    Direction: DirectionOutgoing,
})
```

### Use Case 2: Following References
```go
// Memory A references Memory B
graph.AddEdge(ctx, memA.ID, memB.ID, "references", 0.8)

// Find what references a memory
results, _ := graph.GetRelated(ctx, memB.ID, &TraversalOptions{
    Relations: []string{"references"},
    Direction: DirectionIncoming,
})
```

### Use Case 3: Causal Chain Exploration
```go
// Build causal chain: Decision -> Analysis -> Data
graph.AddEdge(ctx, decision.ID, analysis.ID, "caused_by", 1.0)
graph.AddEdge(ctx, analysis.ID, data.ID, "caused_by", 1.0)

// Traverse causal chain
results, _ := graph.Traverse(ctx, decision.ID, &TraversalOptions{
    Relations: []string{"caused_by"},
    MaxDepth:  5,
    Direction: DirectionOutgoing,
})
```
