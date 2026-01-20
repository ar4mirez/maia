package graph

import (
	"context"
	"io"
	"sync"
	"time"
)

// InMemoryIndex is an in-memory graph index using adjacency lists.
type InMemoryIndex struct {
	// outgoing maps source ID -> list of edges
	outgoing map[string][]Edge
	// incoming maps target ID -> list of edges (for reverse lookups)
	incoming map[string][]Edge
	// nodes tracks all unique node IDs
	nodes map[string]struct{}
	// edgeCount is the total number of edges
	edgeCount int

	mu     sync.RWMutex
	closed bool
}

// NewInMemoryIndex creates a new in-memory graph index.
func NewInMemoryIndex() *InMemoryIndex {
	return &InMemoryIndex{
		outgoing: make(map[string][]Edge),
		incoming: make(map[string][]Edge),
		nodes:    make(map[string]struct{}),
	}
}

// AddEdge creates a directed edge between two memories.
func (idx *InMemoryIndex) AddEdge(ctx context.Context, sourceID, targetID, relation string, weight float32) error {
	edge := &Edge{
		SourceID:  sourceID,
		TargetID:  targetID,
		Relation:  relation,
		Weight:    weight,
		CreatedAt: time.Now(),
	}
	return idx.AddEdgeWithMetadata(ctx, edge)
}

// AddEdgeWithMetadata creates an edge with additional metadata.
func (idx *InMemoryIndex) AddEdgeWithMetadata(ctx context.Context, edge *Edge) error {
	if err := edge.Validate(); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.closed {
		return ErrIndexClosed
	}

	// Check for duplicate edge
	if idx.hasEdgeLocked(edge.SourceID, edge.TargetID, edge.Relation) {
		// Update existing edge
		idx.removeEdgeLocked(edge.SourceID, edge.TargetID, edge.Relation)
	}

	// Add to outgoing adjacency list
	edgeCopy := *edge
	idx.outgoing[edge.SourceID] = append(idx.outgoing[edge.SourceID], edgeCopy)

	// Add to incoming adjacency list
	idx.incoming[edge.TargetID] = append(idx.incoming[edge.TargetID], edgeCopy)

	// Track nodes
	idx.nodes[edge.SourceID] = struct{}{}
	idx.nodes[edge.TargetID] = struct{}{}

	idx.edgeCount++
	return nil
}

// RemoveEdge removes a specific edge.
func (idx *InMemoryIndex) RemoveEdge(ctx context.Context, sourceID, targetID, relation string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.closed {
		return ErrIndexClosed
	}

	if !idx.hasEdgeLocked(sourceID, targetID, relation) {
		return ErrEdgeNotFound
	}

	idx.removeEdgeLocked(sourceID, targetID, relation)
	return nil
}

// removeEdgeLocked removes an edge without acquiring lock (caller must hold lock).
func (idx *InMemoryIndex) removeEdgeLocked(sourceID, targetID, relation string) {
	// Remove from outgoing list
	outEdges := idx.outgoing[sourceID]
	for i, e := range outEdges {
		if e.TargetID == targetID && e.Relation == relation {
			idx.outgoing[sourceID] = append(outEdges[:i], outEdges[i+1:]...)
			break
		}
	}

	// Remove from incoming list
	inEdges := idx.incoming[targetID]
	for i, e := range inEdges {
		if e.SourceID == sourceID && e.Relation == relation {
			idx.incoming[targetID] = append(inEdges[:i], inEdges[i+1:]...)
			break
		}
	}

	idx.edgeCount--

	// Clean up empty slices and orphan nodes
	if len(idx.outgoing[sourceID]) == 0 {
		delete(idx.outgoing, sourceID)
	}
	if len(idx.incoming[targetID]) == 0 {
		delete(idx.incoming, targetID)
	}

	// Check if nodes are still connected
	idx.cleanupOrphanNode(sourceID)
	idx.cleanupOrphanNode(targetID)
}

// cleanupOrphanNode removes a node from tracking if it has no edges.
func (idx *InMemoryIndex) cleanupOrphanNode(id string) {
	if len(idx.outgoing[id]) == 0 && len(idx.incoming[id]) == 0 {
		delete(idx.nodes, id)
	}
}

// RemoveNode removes all edges to/from a node.
func (idx *InMemoryIndex) RemoveNode(ctx context.Context, id string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.closed {
		return ErrIndexClosed
	}

	// Remove all outgoing edges
	for _, edge := range idx.outgoing[id] {
		// Remove from target's incoming list
		inEdges := idx.incoming[edge.TargetID]
		for i, e := range inEdges {
			if e.SourceID == id {
				idx.incoming[edge.TargetID] = append(inEdges[:i], inEdges[i+1:]...)
				idx.edgeCount--
				break
			}
		}
		if len(idx.incoming[edge.TargetID]) == 0 {
			delete(idx.incoming, edge.TargetID)
		}
		idx.cleanupOrphanNode(edge.TargetID)
	}
	delete(idx.outgoing, id)

	// Remove all incoming edges
	for _, edge := range idx.incoming[id] {
		// Remove from source's outgoing list
		outEdges := idx.outgoing[edge.SourceID]
		for i, e := range outEdges {
			if e.TargetID == id {
				idx.outgoing[edge.SourceID] = append(outEdges[:i], outEdges[i+1:]...)
				idx.edgeCount--
				break
			}
		}
		if len(idx.outgoing[edge.SourceID]) == 0 {
			delete(idx.outgoing, edge.SourceID)
		}
		idx.cleanupOrphanNode(edge.SourceID)
	}
	delete(idx.incoming, id)

	// Remove node from tracking
	delete(idx.nodes, id)

	return nil
}

// GetOutgoing returns all outgoing edges from a node.
func (idx *InMemoryIndex) GetOutgoing(ctx context.Context, id string) ([]Edge, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.closed {
		return nil, ErrIndexClosed
	}

	edges := idx.outgoing[id]
	if len(edges) == 0 {
		return nil, nil
	}

	// Return a copy
	result := make([]Edge, len(edges))
	copy(result, edges)
	return result, nil
}

// GetIncoming returns all incoming edges to a node.
func (idx *InMemoryIndex) GetIncoming(ctx context.Context, id string) ([]Edge, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.closed {
		return nil, ErrIndexClosed
	}

	edges := idx.incoming[id]
	if len(edges) == 0 {
		return nil, nil
	}

	// Return a copy
	result := make([]Edge, len(edges))
	copy(result, edges)
	return result, nil
}

// GetRelated returns related nodes with optional relation filter (single-hop).
func (idx *InMemoryIndex) GetRelated(ctx context.Context, id string, opts *TraversalOptions) ([]TraversalResult, error) {
	if opts == nil {
		opts = DefaultTraversalOptions()
	}
	// Force single hop for GetRelated
	singleHopOpts := *opts
	singleHopOpts.MaxDepth = 1
	return idx.Traverse(ctx, id, &singleHopOpts)
}

// Traverse performs multi-hop graph traversal using BFS.
func (idx *InMemoryIndex) Traverse(ctx context.Context, startID string, opts *TraversalOptions) ([]TraversalResult, error) {
	if opts == nil {
		opts = DefaultTraversalOptions()
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.closed {
		return nil, ErrIndexClosed
	}

	// BFS state
	type queueItem struct {
		id         string
		depth      int
		relation   string
		weight     float32
		path       []string
		cumulative float32
	}

	visited := make(map[string]bool)
	visited[startID] = true

	queue := []queueItem{{
		id:         startID,
		depth:      0,
		path:       []string{startID},
		cumulative: 1.0,
	}}

	var results []TraversalResult

	for len(queue) > 0 && (opts.MaxResults <= 0 || len(results) < opts.MaxResults) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Dequeue
		current := queue[0]
		queue = queue[1:]

		// Check depth limit
		if opts.MaxDepth > 0 && current.depth >= opts.MaxDepth {
			continue
		}

		// Get neighbors based on direction
		var neighbors []Edge
		if opts.Direction == DirectionOutgoing || opts.Direction == DirectionBoth {
			neighbors = append(neighbors, idx.outgoing[current.id]...)
		}
		if opts.Direction == DirectionIncoming || opts.Direction == DirectionBoth {
			for _, e := range idx.incoming[current.id] {
				// For incoming edges, we want to go to the source
				neighbors = append(neighbors, Edge{
					SourceID:  e.TargetID,
					TargetID:  e.SourceID,
					Relation:  e.Relation,
					Weight:    e.Weight,
					Metadata:  e.Metadata,
					CreatedAt: e.CreatedAt,
				})
			}
		}

		for _, edge := range neighbors {
			neighborID := edge.TargetID

			// Skip if already visited
			if visited[neighborID] {
				continue
			}

			// Apply filters
			if !idx.matchesFilters(edge, opts) {
				continue
			}

			visited[neighborID] = true

			// Build path if needed
			var path []string
			if opts.IncludePath {
				path = make([]string, len(current.path)+1)
				copy(path, current.path)
				path[len(current.path)] = neighborID
			}

			cumulative := current.cumulative * edge.Weight

			// Add to results
			result := TraversalResult{
				ID:         neighborID,
				Depth:      current.depth + 1,
				Relation:   edge.Relation,
				Weight:     edge.Weight,
				Path:       path,
				Cumulative: cumulative,
			}
			results = append(results, result)

			// Stop if we've reached max results
			if opts.MaxResults > 0 && len(results) >= opts.MaxResults {
				break
			}

			// Enqueue for further traversal
			queue = append(queue, queueItem{
				id:         neighborID,
				depth:      current.depth + 1,
				relation:   edge.Relation,
				weight:     edge.Weight,
				path:       path,
				cumulative: cumulative,
			})
		}
	}

	return results, nil
}

// matchesFilters checks if an edge matches the traversal options.
func (idx *InMemoryIndex) matchesFilters(edge Edge, opts *TraversalOptions) bool {
	// Check weight
	if edge.Weight < opts.MinWeight {
		return false
	}

	// Check relations filter
	if len(opts.Relations) > 0 {
		found := false
		for _, r := range opts.Relations {
			if edge.Relation == r {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// HasEdge checks if a specific edge exists.
func (idx *InMemoryIndex) HasEdge(ctx context.Context, sourceID, targetID, relation string) bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.closed {
		return false
	}

	return idx.hasEdgeLocked(sourceID, targetID, relation)
}

// hasEdgeLocked checks for edge without acquiring lock.
func (idx *InMemoryIndex) hasEdgeLocked(sourceID, targetID, relation string) bool {
	edges := idx.outgoing[sourceID]
	for _, e := range edges {
		if e.TargetID == targetID {
			// If relation is empty, match any relation
			if relation == "" || e.Relation == relation {
				return true
			}
		}
	}
	return false
}

// Size returns the total number of edges.
func (idx *InMemoryIndex) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.edgeCount
}

// NodeCount returns the number of nodes with edges.
func (idx *InMemoryIndex) NodeCount() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.nodes)
}

// Save persists the index to a writer.
func (idx *InMemoryIndex) Save(w io.Writer) error {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.closed {
		return ErrIndexClosed
	}

	return saveIndex(w, idx.outgoing)
}

// Load restores the index from a reader.
func (idx *InMemoryIndex) Load(r io.Reader) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.closed {
		return ErrIndexClosed
	}

	edges, err := loadIndex(r)
	if err != nil {
		return err
	}

	// Clear existing data
	idx.outgoing = make(map[string][]Edge)
	idx.incoming = make(map[string][]Edge)
	idx.nodes = make(map[string]struct{})
	idx.edgeCount = 0

	// Rebuild from loaded edges
	for sourceID, edgeList := range edges {
		for _, edge := range edgeList {
			idx.outgoing[sourceID] = append(idx.outgoing[sourceID], edge)
			idx.incoming[edge.TargetID] = append(idx.incoming[edge.TargetID], edge)
			idx.nodes[sourceID] = struct{}{}
			idx.nodes[edge.TargetID] = struct{}{}
			idx.edgeCount++
		}
	}

	return nil
}

// Close releases resources.
func (idx *InMemoryIndex) Close() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.closed = true
	idx.outgoing = nil
	idx.incoming = nil
	idx.nodes = nil
	idx.edgeCount = 0

	return nil
}
