// Package retrieval provides multi-strategy memory retrieval for MAIA.
package retrieval

import (
	"context"
	"sort"
	"time"

	"github.com/ar4mirez/maia/internal/embedding"
	"github.com/ar4mirez/maia/internal/index/fulltext"
	"github.com/ar4mirez/maia/internal/index/graph"
	"github.com/ar4mirez/maia/internal/index/vector"
	"github.com/ar4mirez/maia/internal/query"
	"github.com/ar4mirez/maia/internal/storage"
)

// Retriever performs multi-strategy memory retrieval.
type Retriever struct {
	store       storage.Store
	vectorIndex vector.Index
	textIndex   fulltext.Index
	graphIndex  graph.Index
	embedder    embedding.Provider
	scorer      *Scorer
	config      Config
}

// Config holds retrieval configuration.
type Config struct {
	// DefaultLimit is the default number of results to return.
	DefaultLimit int

	// VectorWeight is the weight for vector similarity scores.
	VectorWeight float64

	// TextWeight is the weight for full-text match scores.
	TextWeight float64

	// RecencyWeight is the weight for recency scores.
	RecencyWeight float64

	// FrequencyWeight is the weight for access frequency scores.
	FrequencyWeight float64

	// GraphWeight is the weight for graph connectivity scores.
	GraphWeight float64

	// MinScore is the minimum score threshold for results.
	MinScore float64
}

// DefaultConfig returns the default retrieval configuration.
func DefaultConfig() Config {
	return Config{
		DefaultLimit:    10,
		VectorWeight:    0.35,
		TextWeight:      0.25,
		RecencyWeight:   0.20,
		FrequencyWeight: 0.10,
		GraphWeight:     0.10,
		MinScore:        0.0,
	}
}

// NewRetriever creates a new retriever.
func NewRetriever(
	store storage.Store,
	vectorIndex vector.Index,
	textIndex fulltext.Index,
	embedder embedding.Provider,
	config Config,
) *Retriever {
	return &Retriever{
		store:       store,
		vectorIndex: vectorIndex,
		textIndex:   textIndex,
		embedder:    embedder,
		scorer:      NewScorer(config),
		config:      config,
	}
}

// NewRetrieverWithGraph creates a new retriever with graph index support.
func NewRetrieverWithGraph(
	store storage.Store,
	vectorIndex vector.Index,
	textIndex fulltext.Index,
	graphIndex graph.Index,
	embedder embedding.Provider,
	config Config,
) *Retriever {
	return &Retriever{
		store:       store,
		vectorIndex: vectorIndex,
		textIndex:   textIndex,
		graphIndex:  graphIndex,
		embedder:    embedder,
		scorer:      NewScorer(config),
		config:      config,
	}
}

// SetGraphIndex sets the graph index for the retriever.
func (r *Retriever) SetGraphIndex(idx graph.Index) {
	r.graphIndex = idx
}

// RetrieveOptions configures a retrieval operation.
type RetrieveOptions struct {
	// Namespace to search within.
	Namespace string

	// Limit is the maximum number of results.
	Limit int

	// Types filters by memory types.
	Types []storage.MemoryType

	// Tags filters by tags (AND logic).
	Tags []string

	// MinScore is the minimum score threshold.
	MinScore float64

	// UseVector enables vector similarity search.
	UseVector bool

	// UseText enables full-text search.
	UseText bool

	// UseGraph enables graph-based retrieval.
	UseGraph bool

	// RelatedTo specifies memory IDs to find related memories for.
	// When set, graph traversal will boost memories connected to these IDs.
	RelatedTo []string

	// GraphRelations filters graph edges by relationship types.
	GraphRelations []string

	// GraphDepth specifies the maximum graph traversal depth (default: 2).
	GraphDepth int

	// Analysis is the query analysis result (optional).
	Analysis *query.Analysis
}

// DefaultRetrieveOptions returns default retrieval options.
func DefaultRetrieveOptions() *RetrieveOptions {
	return &RetrieveOptions{
		Limit:      10,
		UseVector:  true,
		UseText:    true,
		UseGraph:   true,
		GraphDepth: 2,
		MinScore:   0.0,
	}
}

// Result represents a single retrieval result.
type Result struct {
	Memory       *storage.Memory
	Score        float64
	VectorScore  float64
	TextScore    float64
	RecencyScore float64
	GraphScore   float64
	Highlights   map[string][]string
}

// Results represents retrieval results.
type Results struct {
	Items      []*Result
	Total      int
	QueryTime  time.Duration
}

// Retrieve performs multi-strategy retrieval for the given query.
func (r *Retriever) Retrieve(ctx context.Context, queryText string, opts *RetrieveOptions) (*Results, error) {
	startTime := time.Now()

	if opts == nil {
		opts = DefaultRetrieveOptions()
	}
	if opts.Limit <= 0 {
		opts.Limit = r.config.DefaultLimit
	}

	// Collect candidates from different strategies
	candidates := make(map[string]*candidateScore)

	// Vector search
	if opts.UseVector && r.vectorIndex != nil && r.embedder != nil {
		vectorResults, err := r.vectorSearch(ctx, queryText, opts)
		if err != nil {
			return nil, err
		}
		for id, score := range vectorResults {
			if _, exists := candidates[id]; !exists {
				candidates[id] = &candidateScore{}
			}
			candidates[id].vectorScore = score
		}
	}

	// Full-text search
	if opts.UseText && r.textIndex != nil {
		textResults, highlights, err := r.textSearch(ctx, queryText, opts)
		if err != nil {
			return nil, err
		}
		for id, score := range textResults {
			if _, exists := candidates[id]; !exists {
				candidates[id] = &candidateScore{}
			}
			candidates[id].textScore = score
			candidates[id].highlights = highlights[id]
		}
	}

	// Graph search - find memories related to specified IDs
	if opts.UseGraph && r.graphIndex != nil && len(opts.RelatedTo) > 0 {
		graphResults, err := r.graphSearch(ctx, opts)
		if err != nil {
			return nil, err
		}
		for id, score := range graphResults {
			if _, exists := candidates[id]; !exists {
				candidates[id] = &candidateScore{}
			}
			candidates[id].graphScore = score
		}
	}

	// If no candidates from searches, try storage search
	if len(candidates) == 0 {
		storageResults, err := r.storageSearch(ctx, queryText, opts)
		if err != nil {
			return nil, err
		}
		for id := range storageResults {
			candidates[id] = &candidateScore{textScore: 0.5}
		}
	}

	// Fetch memories and calculate final scores
	results := make([]*Result, 0, len(candidates))
	for id, scores := range candidates {
		mem, err := r.store.GetMemory(ctx, id)
		if err != nil {
			continue // Skip if memory no longer exists
		}

		// Apply filters
		if opts.Namespace != "" && mem.Namespace != opts.Namespace {
			continue
		}
		if len(opts.Types) > 0 && !containsType(opts.Types, mem.Type) {
			continue
		}
		if len(opts.Tags) > 0 && !containsAllTags(mem.Tags, opts.Tags) {
			continue
		}

		// Calculate recency score
		recencyScore := r.scorer.RecencyScore(mem.AccessedAt)

		// Calculate graph score if not already set
		graphScore := scores.graphScore
		if graphScore == 0 && opts.UseGraph && r.graphIndex != nil && len(opts.RelatedTo) > 0 {
			graphScore = r.calculateGraphScore(ctx, id, opts.RelatedTo)
		}

		// Calculate final score
		finalScore := r.scorer.CombinedScoreWithGraph(
			scores.vectorScore,
			scores.textScore,
			recencyScore,
			float64(mem.AccessCount),
			graphScore,
		)

		if finalScore < opts.MinScore {
			continue
		}

		results = append(results, &Result{
			Memory:       mem,
			Score:        finalScore,
			VectorScore:  scores.vectorScore,
			TextScore:    scores.textScore,
			RecencyScore: recencyScore,
			GraphScore:   graphScore,
			Highlights:   scores.highlights,
		})
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Apply limit
	if len(results) > opts.Limit {
		results = results[:opts.Limit]
	}

	return &Results{
		Items:     results,
		Total:     len(results),
		QueryTime: time.Since(startTime),
	}, nil
}

// vectorSearch performs vector similarity search.
func (r *Retriever) vectorSearch(ctx context.Context, queryText string, opts *RetrieveOptions) (map[string]float64, error) {
	// Generate query embedding
	queryEmbedding, err := r.embedder.Embed(ctx, queryText)
	if err != nil {
		return nil, err
	}

	// Search vector index
	searchResults, err := r.vectorIndex.Search(ctx, queryEmbedding, opts.Limit*2) // Get more for filtering
	if err != nil {
		return nil, err
	}

	scores := make(map[string]float64)
	for _, result := range searchResults {
		scores[result.ID] = float64(result.Score)
	}

	return scores, nil
}

// textSearch performs full-text search.
func (r *Retriever) textSearch(ctx context.Context, queryText string, opts *RetrieveOptions) (map[string]float64, map[string]map[string][]string, error) {
	searchOpts := &fulltext.SearchOptions{
		Limit:           opts.Limit * 2,
		Namespace:       opts.Namespace,
		HighlightFields: []string{"content"},
	}

	if len(opts.Tags) > 0 {
		searchOpts.Tags = opts.Tags
	}

	results, err := r.textIndex.Search(ctx, queryText, searchOpts)
	if err != nil {
		return nil, nil, err
	}

	scores := make(map[string]float64)
	highlights := make(map[string]map[string][]string)

	for _, hit := range results.Hits {
		// Normalize score (Bleve scores can be > 1)
		normalizedScore := hit.Score
		if results.MaxScore > 0 {
			normalizedScore = hit.Score / results.MaxScore
		}
		scores[hit.ID] = normalizedScore
		highlights[hit.ID] = hit.Highlights
	}

	return scores, highlights, nil
}

// storageSearch performs a direct storage search as fallback.
func (r *Retriever) storageSearch(ctx context.Context, queryText string, opts *RetrieveOptions) (map[string]struct{}, error) {
	searchOpts := &storage.SearchOptions{
		Namespace: opts.Namespace,
		Types:     opts.Types,
		Tags:      opts.Tags,
		Limit:     opts.Limit,
	}

	results, err := r.store.SearchMemories(ctx, searchOpts)
	if err != nil {
		return nil, err
	}

	ids := make(map[string]struct{})
	for _, result := range results {
		ids[result.Memory.ID] = struct{}{}
	}

	return ids, nil
}

// graphSearch finds memories related to the specified IDs via graph traversal.
func (r *Retriever) graphSearch(ctx context.Context, opts *RetrieveOptions) (map[string]float64, error) {
	scores := make(map[string]float64)

	depth := opts.GraphDepth
	if depth <= 0 {
		depth = 2
	}

	traversalOpts := &graph.TraversalOptions{
		Direction:  graph.DirectionBoth,
		MaxDepth:   depth,
		MaxResults: opts.Limit * 3, // Get more for filtering
		Relations:  opts.GraphRelations,
	}

	for _, sourceID := range opts.RelatedTo {
		results, err := r.graphIndex.Traverse(ctx, sourceID, traversalOpts)
		if err != nil {
			continue // Skip errors for individual sources
		}

		for _, result := range results {
			// Calculate score based on depth and cumulative weight
			// Closer nodes get higher scores
			depthFactor := 1.0 / float64(result.Depth)
			score := float64(result.Cumulative) * depthFactor

			// Keep the highest score if we've seen this ID before
			if existing, ok := scores[result.ID]; !ok || score > existing {
				scores[result.ID] = score
			}
		}
	}

	return scores, nil
}

// calculateGraphScore calculates a graph connectivity score for a candidate.
func (r *Retriever) calculateGraphScore(ctx context.Context, candidateID string, relatedTo []string) float64 {
	if r.graphIndex == nil || len(relatedTo) == 0 {
		return 0
	}

	var maxScore float64
	for _, sourceID := range relatedTo {
		// Check direct connection
		if r.graphIndex.HasEdge(ctx, sourceID, candidateID, "") {
			return 1.0 // Direct connection = max score
		}
		if r.graphIndex.HasEdge(ctx, candidateID, sourceID, "") {
			return 1.0 // Direct connection in reverse = max score
		}

		// Check 2-hop connection
		outgoing, err := r.graphIndex.GetOutgoing(ctx, sourceID)
		if err == nil {
			for _, edge := range outgoing {
				if r.graphIndex.HasEdge(ctx, edge.TargetID, candidateID, "") {
					score := float64(edge.Weight) * 0.5 // 2-hop penalty
					if score > maxScore {
						maxScore = score
					}
				}
			}
		}

		incoming, err := r.graphIndex.GetIncoming(ctx, sourceID)
		if err == nil {
			for _, edge := range incoming {
				if r.graphIndex.HasEdge(ctx, edge.SourceID, candidateID, "") {
					score := float64(edge.Weight) * 0.5 // 2-hop penalty
					if score > maxScore {
						maxScore = score
					}
				}
			}
		}
	}

	return maxScore
}

// candidateScore holds scores for a candidate.
type candidateScore struct {
	vectorScore float64
	textScore   float64
	graphScore  float64
	highlights  map[string][]string
}

// Helper functions

func containsType(types []storage.MemoryType, t storage.MemoryType) bool {
	for _, mt := range types {
		if mt == t {
			return true
		}
	}
	return false
}

func containsAllTags(memoryTags, requiredTags []string) bool {
	tagSet := make(map[string]bool)
	for _, t := range memoryTags {
		tagSet[t] = true
	}
	for _, t := range requiredTags {
		if !tagSet[t] {
			return false
		}
	}
	return true
}
