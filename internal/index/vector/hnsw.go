package vector

import (
	"container/heap"
	"context"
	"math"
	"math/rand"
	"sync"

	"github.com/ar4mirez/maia/internal/embedding"
)

// HNSWIndex implements the Hierarchical Navigable Small World algorithm
// for approximate nearest neighbor search.
type HNSWIndex struct {
	dimension      int
	m              int     // Max connections per node
	mMax           int     // Max connections per node at layer 0
	efConstruction int     // Size of dynamic candidate list during construction
	efSearch       int     // Size of dynamic candidate list during search
	levelMult      float64 // Multiplier for random level generation

	nodes     map[string]*hnswNode
	entryNode *hnswNode
	maxLevel  int

	mu     sync.RWMutex
	closed bool
	rng    *rand.Rand
}

// hnswNode represents a node in the HNSW graph.
type hnswNode struct {
	id         string
	vector     []float32
	level      int
	neighbors  []map[string]*hnswNode // neighbors[level] = map of neighbors at that level
}

// NewHNSWIndex creates a new HNSW index with the given configuration.
func NewHNSWIndex(cfg Config) *HNSWIndex {
	if cfg.M <= 0 {
		cfg.M = 16
	}
	if cfg.EfConstruction <= 0 {
		cfg.EfConstruction = 200
	}
	if cfg.EfSearch <= 0 {
		cfg.EfSearch = 50
	}

	return &HNSWIndex{
		dimension:      cfg.Dimension,
		m:              cfg.M,
		mMax:           cfg.M * 2,
		efConstruction: cfg.EfConstruction,
		efSearch:       cfg.EfSearch,
		levelMult:      1.0 / math.Log(float64(cfg.M)),
		nodes:          make(map[string]*hnswNode),
		rng:            rand.New(rand.NewSource(42)),
	}
}

// Add adds a vector to the index.
func (idx *HNSWIndex) Add(ctx context.Context, id string, vector []float32) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.closed {
		return ErrIndexClosed
	}

	if len(vector) != idx.dimension {
		return ErrDimensionMismatch
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Create new node
	level := idx.randomLevel()
	node := &hnswNode{
		id:        id,
		vector:    make([]float32, len(vector)),
		level:     level,
		neighbors: make([]map[string]*hnswNode, level+1),
	}
	copy(node.vector, vector)

	for i := range node.neighbors {
		node.neighbors[i] = make(map[string]*hnswNode)
	}

	// If this is the first node
	if idx.entryNode == nil {
		idx.entryNode = node
		idx.maxLevel = level
		idx.nodes[id] = node
		return nil
	}

	// Find entry point
	currNode := idx.entryNode
	currDist := idx.distance(vector, currNode.vector)

	// Traverse from top level to level+1
	for l := idx.maxLevel; l > level; l-- {
		currNode, currDist = idx.searchLayerClosest(vector, currNode, currDist, l)
	}

	// Insert at each level from level down to 0
	for l := min(level, idx.maxLevel); l >= 0; l-- {
		neighbors := idx.searchLayer(vector, currNode, idx.efConstruction, l)

		// Select M best neighbors
		m := idx.m
		if l == 0 {
			m = idx.mMax
		}

		selectedNeighbors := idx.selectNeighbors(neighbors, m)

		// Connect node to neighbors
		for _, neighbor := range selectedNeighbors {
			node.neighbors[l][neighbor.id] = neighbor
			neighbor.neighbors[l][node.id] = node

			// Shrink neighbor connections if needed
			maxConns := idx.m
			if l == 0 {
				maxConns = idx.mMax
			}
			if len(neighbor.neighbors[l]) > maxConns {
				idx.shrinkConnections(neighbor, l, maxConns)
			}
		}

		if len(neighbors) > 0 {
			currNode = neighbors[0]
		}
	}

	// Update entry point if new node has higher level
	if level > idx.maxLevel {
		idx.entryNode = node
		idx.maxLevel = level
	}

	idx.nodes[id] = node
	return nil
}

// AddBatch adds multiple vectors to the index.
func (idx *HNSWIndex) AddBatch(ctx context.Context, ids []string, vectors [][]float32) error {
	if len(ids) != len(vectors) {
		return ErrDimensionMismatch
	}

	for i := range ids {
		if err := idx.Add(ctx, ids[i], vectors[i]); err != nil {
			return err
		}
	}
	return nil
}

// Remove removes a vector from the index.
func (idx *HNSWIndex) Remove(ctx context.Context, id string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.closed {
		return ErrIndexClosed
	}

	node, exists := idx.nodes[id]
	if !exists {
		return ErrNotFound
	}

	// Remove from all neighbors
	for l := 0; l <= node.level; l++ {
		for _, neighbor := range node.neighbors[l] {
			delete(neighbor.neighbors[l], id)
		}
	}

	delete(idx.nodes, id)

	// Update entry point if needed
	if idx.entryNode == node {
		idx.entryNode = nil
		idx.maxLevel = 0
		for _, n := range idx.nodes {
			if idx.entryNode == nil || n.level > idx.maxLevel {
				idx.entryNode = n
				idx.maxLevel = n.level
			}
		}
	}

	return nil
}

// Search finds the k nearest neighbors to the query vector.
func (idx *HNSWIndex) Search(ctx context.Context, query []float32, k int) ([]SearchResult, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.closed {
		return nil, ErrIndexClosed
	}

	if len(query) != idx.dimension {
		return nil, ErrDimensionMismatch
	}

	if k <= 0 {
		return nil, ErrInvalidK
	}

	if idx.entryNode == nil {
		return []SearchResult{}, nil
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Start from entry point
	currNode := idx.entryNode
	currDist := idx.distance(query, currNode.vector)

	// Traverse from top level to level 1
	for l := idx.maxLevel; l > 0; l-- {
		currNode, currDist = idx.searchLayerClosest(query, currNode, currDist, l)
	}

	// Search at level 0 with ef neighbors
	ef := max(idx.efSearch, k)
	neighbors := idx.searchLayer(query, currNode, ef, 0)

	// Return top k
	if k > len(neighbors) {
		k = len(neighbors)
	}

	results := make([]SearchResult, k)
	for i := 0; i < k; i++ {
		dist := idx.distance(query, neighbors[i].vector)
		sim, _ := embedding.CosineSimilarity(query, neighbors[i].vector)
		results[i] = SearchResult{
			ID:       neighbors[i].id,
			Score:    sim,
			Distance: dist,
		}
	}

	return results, nil
}

// Get retrieves a vector by ID.
func (idx *HNSWIndex) Get(ctx context.Context, id string) ([]float32, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.closed {
		return nil, ErrIndexClosed
	}

	node, exists := idx.nodes[id]
	if !exists {
		return nil, ErrNotFound
	}

	result := make([]float32, len(node.vector))
	copy(result, node.vector)
	return result, nil
}

// Size returns the number of vectors in the index.
func (idx *HNSWIndex) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.nodes)
}

// Dimension returns the vector dimension.
func (idx *HNSWIndex) Dimension() int {
	return idx.dimension
}

// Close closes the index.
func (idx *HNSWIndex) Close() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.closed = true
	idx.nodes = nil
	idx.entryNode = nil
	return nil
}

// randomLevel generates a random level for a new node.
func (idx *HNSWIndex) randomLevel() int {
	r := idx.rng.Float64()
	return int(-math.Log(r) * idx.levelMult)
}

// distance calculates the Euclidean distance between two vectors.
func (idx *HNSWIndex) distance(a, b []float32) float32 {
	var sum float32
	for i := range a {
		diff := a[i] - b[i]
		sum += diff * diff
	}
	return float32(math.Sqrt(float64(sum)))
}

// searchLayerClosest finds the closest node to query starting from entry.
func (idx *HNSWIndex) searchLayerClosest(query []float32, entry *hnswNode, entryDist float32, level int) (*hnswNode, float32) {
	currNode := entry
	currDist := entryDist

	changed := true
	for changed {
		changed = false
		for _, neighbor := range currNode.neighbors[level] {
			dist := idx.distance(query, neighbor.vector)
			if dist < currDist {
				currNode = neighbor
				currDist = dist
				changed = true
			}
		}
	}

	return currNode, currDist
}

// searchLayer searches for ef nearest neighbors at the given level.
func (idx *HNSWIndex) searchLayer(query []float32, entry *hnswNode, ef int, level int) []*hnswNode {
	visited := make(map[string]bool)
	candidates := &distanceHeap{}
	results := &distanceHeap{}

	entryDist := idx.distance(query, entry.vector)
	heap.Push(candidates, &heapItem{node: entry, distance: entryDist})
	heap.Push(results, &heapItem{node: entry, distance: -entryDist}) // Max heap
	visited[entry.id] = true

	for candidates.Len() > 0 {
		curr := heap.Pop(candidates).(*heapItem)

		// Check if we can stop
		if results.Len() >= ef {
			farthest := (*results)[0]
			if curr.distance > -farthest.distance {
				break
			}
		}

		// Explore neighbors
		for _, neighbor := range curr.node.neighbors[level] {
			if visited[neighbor.id] {
				continue
			}
			visited[neighbor.id] = true

			dist := idx.distance(query, neighbor.vector)

			if results.Len() < ef {
				heap.Push(candidates, &heapItem{node: neighbor, distance: dist})
				heap.Push(results, &heapItem{node: neighbor, distance: -dist})
			} else {
				farthest := (*results)[0]
				if dist < -farthest.distance {
					heap.Pop(results)
					heap.Push(candidates, &heapItem{node: neighbor, distance: dist})
					heap.Push(results, &heapItem{node: neighbor, distance: -dist})
				}
			}
		}
	}

	// Convert results to slice, sorted by distance
	result := make([]*hnswNode, results.Len())
	for i := len(result) - 1; i >= 0; i-- {
		item := heap.Pop(results).(*heapItem)
		result[i] = item.node
	}

	return result
}

// selectNeighbors selects the m best neighbors using simple strategy.
func (idx *HNSWIndex) selectNeighbors(candidates []*hnswNode, m int) []*hnswNode {
	if len(candidates) <= m {
		return candidates
	}
	return candidates[:m]
}

// shrinkConnections reduces the number of connections for a node.
func (idx *HNSWIndex) shrinkConnections(node *hnswNode, level int, maxConns int) {
	if len(node.neighbors[level]) <= maxConns {
		return
	}

	// Sort neighbors by distance
	type neighborDist struct {
		neighbor *hnswNode
		dist     float32
	}

	neighbors := make([]neighborDist, 0, len(node.neighbors[level]))
	for _, n := range node.neighbors[level] {
		neighbors = append(neighbors, neighborDist{
			neighbor: n,
			dist:     idx.distance(node.vector, n.vector),
		})
	}

	// Sort by distance
	for i := 0; i < len(neighbors); i++ {
		for j := i + 1; j < len(neighbors); j++ {
			if neighbors[j].dist < neighbors[i].dist {
				neighbors[i], neighbors[j] = neighbors[j], neighbors[i]
			}
		}
	}

	// Keep only the closest maxConns neighbors
	node.neighbors[level] = make(map[string]*hnswNode)
	for i := 0; i < maxConns && i < len(neighbors); i++ {
		node.neighbors[level][neighbors[i].neighbor.id] = neighbors[i].neighbor
	}
}

// heapItem is an item in the priority queue.
type heapItem struct {
	node     *hnswNode
	distance float32
}

// distanceHeap implements heap.Interface for heapItems.
type distanceHeap []*heapItem

func (h distanceHeap) Len() int           { return len(h) }
func (h distanceHeap) Less(i, j int) bool { return h[i].distance < h[j].distance }
func (h distanceHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *distanceHeap) Push(x interface{}) {
	*h = append(*h, x.(*heapItem))
}

func (h *distanceHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[0 : n-1]
	return item
}
