package retrieval

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ar4mirez/maia/internal/embedding"
	"github.com/ar4mirez/maia/internal/index/fulltext"
	"github.com/ar4mirez/maia/internal/index/graph"
	"github.com/ar4mirez/maia/internal/index/vector"
	"github.com/ar4mirez/maia/internal/query"
	"github.com/ar4mirez/maia/internal/storage"
	"github.com/ar4mirez/maia/internal/storage/badger"
)

// testStore wraps a badger store for testing
type testStore struct {
	*badger.Store
	cleanup func()
}

func setupTestStore(t testing.TB) *testStore {
	t.Helper()

	dir, err := os.MkdirTemp("", "maia-retrieval-test-*")
	require.NoError(t, err)

	store, err := badger.NewWithPath(dir)
	require.NoError(t, err)

	cleanup := func() {
		store.Close()
		os.RemoveAll(dir)
	}

	return &testStore{Store: store, cleanup: cleanup}
}

func setupTestRetriever(t *testing.T) (*Retriever, *testStore, func()) {
	// Create test store
	ts := setupTestStore(t)

	// Create mock embedder
	embedder := embedding.NewMockProvider(128)

	// Create vector index
	vectorIndex := vector.NewBruteForceIndex(128)

	// Create fulltext index (in-memory for testing)
	fulltextIndex, err := fulltext.NewBleveIndex(fulltext.Config{InMemory: true})
	require.NoError(t, err)

	// Create retriever
	config := DefaultConfig()
	retriever := NewRetriever(ts.Store, vectorIndex, fulltextIndex, embedder, config)

	cleanup := func() {
		ts.cleanup()
		vectorIndex.Close()
		fulltextIndex.Close()
	}

	return retriever, ts, cleanup
}

func TestNewRetriever(t *testing.T) {
	retriever, _, cleanup := setupTestRetriever(t)
	defer cleanup()

	assert.NotNil(t, retriever)
	assert.NotNil(t, retriever.scorer)
}

func TestDefaultRetrieveOptions(t *testing.T) {
	opts := DefaultRetrieveOptions()

	assert.Equal(t, 10, opts.Limit)
	assert.True(t, opts.UseVector)
	assert.True(t, opts.UseText)
	assert.Equal(t, 0.0, opts.MinScore)
}

func TestRetriever_Retrieve_EmptyQuery(t *testing.T) {
	retriever, _, cleanup := setupTestRetriever(t)
	defer cleanup()

	ctx := context.Background()

	// Empty query with vector search enabled returns error from embedder
	_, err := retriever.Retrieve(ctx, "", &RetrieveOptions{
		UseVector: true,
		UseText:   false,
	})
	require.Error(t, err, "empty query should return error when vector search is enabled")

	// Empty query without vector search should work (falls back to storage)
	results, err := retriever.Retrieve(ctx, "", &RetrieveOptions{
		UseVector: false,
		UseText:   false,
	})
	require.NoError(t, err)
	assert.NotNil(t, results)
}

func TestRetriever_Retrieve_BasicSearch(t *testing.T) {
	retriever, ts, cleanup := setupTestRetriever(t)
	defer cleanup()

	ctx := context.Background()

	// Create test memories
	memories := []*storage.CreateMemoryInput{
		{
			Namespace: "test",
			Content:   "The quick brown fox jumps over the lazy dog.",
			Type:      storage.MemoryTypeSemantic,
		},
		{
			Namespace: "test",
			Content:   "A journey of a thousand miles begins with a single step.",
			Type:      storage.MemoryTypeSemantic,
		},
		{
			Namespace: "test",
			Content:   "The fox is a clever animal that hunts at night.",
			Type:      storage.MemoryTypeSemantic,
		},
	}

	for _, input := range memories {
		mem, err := ts.Store.CreateMemory(ctx, input)
		require.NoError(t, err)

		// Index in vector index
		emb, err := retriever.embedder.Embed(ctx, input.Content)
		require.NoError(t, err)
		err = retriever.vectorIndex.Add(ctx, mem.ID, emb)
		require.NoError(t, err)

		// Index in fulltext index
		err = retriever.textIndex.Index(ctx, mem.ID, &fulltext.Document{
			ID:        mem.ID,
			Content:   input.Content,
			Namespace: input.Namespace,
			Type:      string(input.Type),
		})
		require.NoError(t, err)
	}

	// Search for fox-related content
	results, err := retriever.Retrieve(ctx, "fox", &RetrieveOptions{
		Namespace: "test",
		Limit:     10,
		UseVector: true,
		UseText:   true,
	})

	require.NoError(t, err)
	assert.NotNil(t, results)
	assert.Greater(t, len(results.Items), 0)
	assert.Greater(t, results.QueryTime, time.Duration(0))
}

func TestRetriever_Retrieve_WithVectorOnly(t *testing.T) {
	retriever, ts, cleanup := setupTestRetriever(t)
	defer cleanup()

	ctx := context.Background()

	// Create and index a memory
	mem, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Machine learning is a subset of artificial intelligence.",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	emb, err := retriever.embedder.Embed(ctx, mem.Content)
	require.NoError(t, err)
	err = retriever.vectorIndex.Add(ctx, mem.ID, emb)
	require.NoError(t, err)

	// Search with vector only
	results, err := retriever.Retrieve(ctx, "AI machine learning", &RetrieveOptions{
		Namespace: "test",
		Limit:     10,
		UseVector: true,
		UseText:   false,
	})

	require.NoError(t, err)
	assert.NotNil(t, results)
}

func TestRetriever_Retrieve_WithTextOnly(t *testing.T) {
	retriever, ts, cleanup := setupTestRetriever(t)
	defer cleanup()

	ctx := context.Background()

	// Create and index a memory
	mem, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Kubernetes is an open-source container orchestration platform.",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	err = retriever.textIndex.Index(ctx, mem.ID, &fulltext.Document{
		ID:        mem.ID,
		Content:   mem.Content,
		Namespace: mem.Namespace,
		Type:      string(mem.Type),
	})
	require.NoError(t, err)

	// Search with text only
	results, err := retriever.Retrieve(ctx, "kubernetes container", &RetrieveOptions{
		Namespace: "test",
		Limit:     10,
		UseVector: false,
		UseText:   true,
	})

	require.NoError(t, err)
	assert.NotNil(t, results)
}

func TestRetriever_Retrieve_FiltersNamespace(t *testing.T) {
	retriever, ts, cleanup := setupTestRetriever(t)
	defer cleanup()

	ctx := context.Background()

	// Create memories in different namespaces
	mem1, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "ns1",
		Content:   "Data in namespace 1",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	mem2, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "ns2",
		Content:   "Data in namespace 2",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	// Index both
	for _, mem := range []*storage.Memory{mem1, mem2} {
		emb, err := retriever.embedder.Embed(ctx, mem.Content)
		require.NoError(t, err)
		err = retriever.vectorIndex.Add(ctx, mem.ID, emb)
		require.NoError(t, err)
	}

	// Search in ns1 only
	results, err := retriever.Retrieve(ctx, "Data", &RetrieveOptions{
		Namespace: "ns1",
		Limit:     10,
		UseVector: true,
		UseText:   false,
	})

	require.NoError(t, err)
	for _, item := range results.Items {
		assert.Equal(t, "ns1", item.Memory.Namespace)
	}
}

func TestRetriever_Retrieve_FiltersTypes(t *testing.T) {
	retriever, ts, cleanup := setupTestRetriever(t)
	defer cleanup()

	ctx := context.Background()

	// Create memories with different types
	mem1, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Semantic memory content",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	mem2, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Episodic memory content",
		Type:      storage.MemoryTypeEpisodic,
	})
	require.NoError(t, err)

	// Index both
	for _, mem := range []*storage.Memory{mem1, mem2} {
		emb, err := retriever.embedder.Embed(ctx, mem.Content)
		require.NoError(t, err)
		err = retriever.vectorIndex.Add(ctx, mem.ID, emb)
		require.NoError(t, err)
	}

	// Search for semantic only
	results, err := retriever.Retrieve(ctx, "memory content", &RetrieveOptions{
		Namespace: "test",
		Types:     []storage.MemoryType{storage.MemoryTypeSemantic},
		Limit:     10,
		UseVector: true,
		UseText:   false,
	})

	require.NoError(t, err)
	for _, item := range results.Items {
		assert.Equal(t, storage.MemoryTypeSemantic, item.Memory.Type)
	}
}

func TestRetriever_Retrieve_FiltersTags(t *testing.T) {
	retriever, ts, cleanup := setupTestRetriever(t)
	defer cleanup()

	ctx := context.Background()

	// Create memories with different tags
	mem1, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Important data",
		Type:      storage.MemoryTypeSemantic,
		Tags:      []string{"important", "work"},
	})
	require.NoError(t, err)

	mem2, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Personal data",
		Type:      storage.MemoryTypeSemantic,
		Tags:      []string{"personal"},
	})
	require.NoError(t, err)

	// Index both
	for _, mem := range []*storage.Memory{mem1, mem2} {
		emb, err := retriever.embedder.Embed(ctx, mem.Content)
		require.NoError(t, err)
		err = retriever.vectorIndex.Add(ctx, mem.ID, emb)
		require.NoError(t, err)
	}

	// Search for important work items only
	results, err := retriever.Retrieve(ctx, "data", &RetrieveOptions{
		Namespace: "test",
		Tags:      []string{"important"},
		Limit:     10,
		UseVector: true,
		UseText:   false,
	})

	require.NoError(t, err)
	for _, item := range results.Items {
		assert.Contains(t, item.Memory.Tags, "important")
	}
}

func TestRetriever_Retrieve_MinScoreFilter(t *testing.T) {
	retriever, ts, cleanup := setupTestRetriever(t)
	defer cleanup()

	ctx := context.Background()

	// Create memories
	for i := 0; i < 5; i++ {
		mem, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
			Namespace: "test",
			Content:   "Test memory content for retrieval testing",
			Type:      storage.MemoryTypeSemantic,
		})
		require.NoError(t, err)

		emb, err := retriever.embedder.Embed(ctx, mem.Content)
		require.NoError(t, err)
		err = retriever.vectorIndex.Add(ctx, mem.ID, emb)
		require.NoError(t, err)
	}

	// Search with high min score - may filter out results
	results, err := retriever.Retrieve(ctx, "test memory", &RetrieveOptions{
		Namespace: "test",
		Limit:     10,
		MinScore:  0.9,
		UseVector: true,
		UseText:   false,
	})

	require.NoError(t, err)
	// All returned results should meet min score
	for _, item := range results.Items {
		assert.GreaterOrEqual(t, item.Score, 0.9)
	}
}

func TestRetriever_Retrieve_RespectsLimit(t *testing.T) {
	retriever, ts, cleanup := setupTestRetriever(t)
	defer cleanup()

	ctx := context.Background()

	// Create many memories
	for i := 0; i < 20; i++ {
		mem, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
			Namespace: "test",
			Content:   "Test content for limit testing",
			Type:      storage.MemoryTypeSemantic,
		})
		require.NoError(t, err)

		emb, err := retriever.embedder.Embed(ctx, mem.Content)
		require.NoError(t, err)
		err = retriever.vectorIndex.Add(ctx, mem.ID, emb)
		require.NoError(t, err)
	}

	// Search with limit
	results, err := retriever.Retrieve(ctx, "test content", &RetrieveOptions{
		Namespace: "test",
		Limit:     5,
		UseVector: true,
		UseText:   false,
	})

	require.NoError(t, err)
	assert.LessOrEqual(t, len(results.Items), 5)
}

func TestRetriever_Retrieve_ResultsSortedByScore(t *testing.T) {
	retriever, ts, cleanup := setupTestRetriever(t)
	defer cleanup()

	ctx := context.Background()

	// Create memories with varying relevance
	contents := []string{
		"Machine learning and artificial intelligence research",
		"Deep neural networks for computer vision",
		"Natural language processing with transformers",
	}

	for _, content := range contents {
		mem, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
			Namespace: "test",
			Content:   content,
			Type:      storage.MemoryTypeSemantic,
		})
		require.NoError(t, err)

		emb, err := retriever.embedder.Embed(ctx, content)
		require.NoError(t, err)
		err = retriever.vectorIndex.Add(ctx, mem.ID, emb)
		require.NoError(t, err)
	}

	results, err := retriever.Retrieve(ctx, "machine learning", &RetrieveOptions{
		Namespace: "test",
		Limit:     10,
		UseVector: true,
		UseText:   false,
	})

	require.NoError(t, err)

	// Results should be sorted by score descending
	for i := 1; i < len(results.Items); i++ {
		assert.GreaterOrEqual(t, results.Items[i-1].Score, results.Items[i].Score,
			"results should be sorted by score descending")
	}
}

func TestRetriever_Retrieve_IncludesHighlights(t *testing.T) {
	retriever, ts, cleanup := setupTestRetriever(t)
	defer cleanup()

	ctx := context.Background()

	// Create and index a memory
	mem, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "The quick brown fox jumps over the lazy dog",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	err = retriever.textIndex.Index(ctx, mem.ID, &fulltext.Document{
		ID:        mem.ID,
		Content:   mem.Content,
		Namespace: mem.Namespace,
		Type:      string(mem.Type),
	})
	require.NoError(t, err)

	// Search with text to get highlights
	results, err := retriever.Retrieve(ctx, "fox", &RetrieveOptions{
		Namespace: "test",
		Limit:     10,
		UseVector: false,
		UseText:   true,
	})

	require.NoError(t, err)
	assert.NotNil(t, results)
}

func TestRetriever_Retrieve_WithAnalysis(t *testing.T) {
	retriever, ts, cleanup := setupTestRetriever(t)
	defer cleanup()

	ctx := context.Background()

	// Create a memory
	mem, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "User preferences for dark mode",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	emb, err := retriever.embedder.Embed(ctx, mem.Content)
	require.NoError(t, err)
	err = retriever.vectorIndex.Add(ctx, mem.ID, emb)
	require.NoError(t, err)

	// Create query analysis
	analyzer := query.NewAnalyzer()
	analysis, err := analyzer.Analyze(ctx, "What are the user preferences?")
	require.NoError(t, err)

	// Search with analysis
	results, err := retriever.Retrieve(ctx, "user preferences", &RetrieveOptions{
		Namespace: "test",
		Limit:     10,
		UseVector: true,
		UseText:   false,
		Analysis:  analysis,
	})

	require.NoError(t, err)
	assert.NotNil(t, results)
}

func TestRetriever_Retrieve_FallbackToStorageSearch(t *testing.T) {
	// Create retriever without vector or text index
	ts := setupTestStore(t)
	defer ts.cleanup()

	retriever := NewRetriever(ts.Store, nil, nil, nil, DefaultConfig())

	ctx := context.Background()

	// Create memories
	_, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Test memory for fallback",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	// Should fall back to storage search
	results, err := retriever.Retrieve(ctx, "test", &RetrieveOptions{
		Namespace: "test",
		Limit:     10,
		UseVector: false,
		UseText:   false,
	})

	require.NoError(t, err)
	assert.NotNil(t, results)
}

func TestContainsType(t *testing.T) {
	tests := []struct {
		name     string
		types    []storage.MemoryType
		target   storage.MemoryType
		expected bool
	}{
		{
			name:     "contains type",
			types:    []storage.MemoryType{storage.MemoryTypeSemantic, storage.MemoryTypeEpisodic},
			target:   storage.MemoryTypeSemantic,
			expected: true,
		},
		{
			name:     "does not contain type",
			types:    []storage.MemoryType{storage.MemoryTypeEpisodic},
			target:   storage.MemoryTypeSemantic,
			expected: false,
		},
		{
			name:     "empty list",
			types:    []storage.MemoryType{},
			target:   storage.MemoryTypeSemantic,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsType(tt.types, tt.target)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContainsAllTags(t *testing.T) {
	tests := []struct {
		name         string
		memoryTags   []string
		requiredTags []string
		expected     bool
	}{
		{
			name:         "has all required tags",
			memoryTags:   []string{"a", "b", "c"},
			requiredTags: []string{"a", "b"},
			expected:     true,
		},
		{
			name:         "missing a required tag",
			memoryTags:   []string{"a", "c"},
			requiredTags: []string{"a", "b"},
			expected:     false,
		},
		{
			name:         "empty required tags",
			memoryTags:   []string{"a", "b"},
			requiredTags: []string{},
			expected:     true,
		},
		{
			name:         "empty memory tags",
			memoryTags:   []string{},
			requiredTags: []string{"a"},
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsAllTags(tt.memoryTags, tt.requiredTags)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// setupTestRetrieverWithGraph creates a retriever with graph index support
func setupTestRetrieverWithGraph(t *testing.T) (*Retriever, *testStore, graph.Index, func()) {
	// Create test store
	ts := setupTestStore(t)

	// Create mock embedder
	embedder := embedding.NewMockProvider(128)

	// Create vector index
	vectorIndex := vector.NewBruteForceIndex(128)

	// Create fulltext index (in-memory for testing)
	fulltextIndex, err := fulltext.NewBleveIndex(fulltext.Config{InMemory: true})
	require.NoError(t, err)

	// Create graph index
	graphIndex := graph.NewInMemoryIndex()

	// Create retriever with graph
	config := DefaultConfig()
	retriever := NewRetrieverWithGraph(ts.Store, vectorIndex, fulltextIndex, graphIndex, embedder, config)

	cleanup := func() {
		ts.cleanup()
		vectorIndex.Close()
		fulltextIndex.Close()
		graphIndex.Close()
	}

	return retriever, ts, graphIndex, cleanup
}

func TestNewRetrieverWithGraph(t *testing.T) {
	retriever, _, graphIndex, cleanup := setupTestRetrieverWithGraph(t)
	defer cleanup()

	assert.NotNil(t, retriever)
	assert.NotNil(t, retriever.scorer)
	assert.NotNil(t, retriever.graphIndex)
	assert.Same(t, graphIndex, retriever.graphIndex)
}

func TestRetriever_SetGraphIndex(t *testing.T) {
	retriever, _, cleanup := setupTestRetriever(t)
	defer cleanup()

	// Initially no graph index
	assert.Nil(t, retriever.graphIndex)

	// Set graph index
	graphIndex := graph.NewInMemoryIndex()
	defer graphIndex.Close()

	retriever.SetGraphIndex(graphIndex)

	assert.NotNil(t, retriever.graphIndex)
	assert.Same(t, graphIndex, retriever.graphIndex)
}

func TestRetriever_Retrieve_WithGraphSearch(t *testing.T) {
	retriever, ts, graphIndex, cleanup := setupTestRetrieverWithGraph(t)
	defer cleanup()

	ctx := context.Background()

	// Create memories with relationships
	mem1, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Machine learning fundamentals and basic concepts",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	mem2, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Deep learning neural networks advanced topics",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	mem3, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Natural language processing with transformers",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	// Index in vector index
	for _, mem := range []*storage.Memory{mem1, mem2, mem3} {
		emb, err := retriever.embedder.Embed(ctx, mem.Content)
		require.NoError(t, err)
		err = retriever.vectorIndex.Add(ctx, mem.ID, emb)
		require.NoError(t, err)
	}

	// Create graph relationships: mem1 -> mem2 -> mem3
	err = graphIndex.AddEdge(ctx, mem1.ID, mem2.ID, graph.RelationRelatedTo, 0.9)
	require.NoError(t, err)
	err = graphIndex.AddEdge(ctx, mem2.ID, mem3.ID, graph.RelationRelatedTo, 0.8)
	require.NoError(t, err)

	// Search with graph enabled and RelatedTo set
	results, err := retriever.Retrieve(ctx, "machine learning", &RetrieveOptions{
		Namespace:  "test",
		Limit:      10,
		UseVector:  true,
		UseText:    false,
		UseGraph:   true,
		RelatedTo:  []string{mem1.ID},
		GraphDepth: 2,
	})

	require.NoError(t, err)
	assert.NotNil(t, results)
	assert.Greater(t, len(results.Items), 0)

	// Results should include graph-related memories
	foundIDs := make(map[string]bool)
	for _, item := range results.Items {
		foundIDs[item.Memory.ID] = true
	}

	// mem2 should be found as it's directly related to mem1
	assert.True(t, foundIDs[mem2.ID], "mem2 should be found as directly related")
}

func TestRetriever_Retrieve_GraphWithRelationFilter(t *testing.T) {
	retriever, ts, graphIndex, cleanup := setupTestRetrieverWithGraph(t)
	defer cleanup()

	ctx := context.Background()

	// Create memories
	mem1, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Project documentation main page",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	mem2, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "API reference documentation",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	mem3, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Tutorial for beginners",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	// Index in vector index
	for _, mem := range []*storage.Memory{mem1, mem2, mem3} {
		emb, err := retriever.embedder.Embed(ctx, mem.Content)
		require.NoError(t, err)
		err = retriever.vectorIndex.Add(ctx, mem.ID, emb)
		require.NoError(t, err)
	}

	// Create different relationship types
	err = graphIndex.AddEdge(ctx, mem1.ID, mem2.ID, graph.RelationReferences, 0.9)
	require.NoError(t, err)
	err = graphIndex.AddEdge(ctx, mem1.ID, mem3.ID, graph.RelationContains, 0.7)
	require.NoError(t, err)

	// Search with specific relation filter
	results, err := retriever.Retrieve(ctx, "documentation", &RetrieveOptions{
		Namespace:      "test",
		Limit:          10,
		UseVector:      true,
		UseText:        false,
		UseGraph:       true,
		RelatedTo:      []string{mem1.ID},
		GraphRelations: []string{graph.RelationReferences},
		GraphDepth:     1,
	})

	require.NoError(t, err)
	assert.NotNil(t, results)
}

func TestRetriever_graphSearch(t *testing.T) {
	retriever, ts, graphIndex, cleanup := setupTestRetrieverWithGraph(t)
	defer cleanup()

	ctx := context.Background()

	// Create memories
	mem1, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Source memory",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	mem2, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Related memory 1",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	mem3, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Related memory 2",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	// Create graph relationships
	err = graphIndex.AddEdge(ctx, mem1.ID, mem2.ID, graph.RelationRelatedTo, 0.9)
	require.NoError(t, err)
	err = graphIndex.AddEdge(ctx, mem1.ID, mem3.ID, graph.RelationRelatedTo, 0.8)
	require.NoError(t, err)

	// Test graphSearch directly
	opts := &RetrieveOptions{
		RelatedTo:  []string{mem1.ID},
		GraphDepth: 2,
		Limit:      10,
	}

	scores, err := retriever.graphSearch(ctx, opts)
	require.NoError(t, err)
	assert.Len(t, scores, 2)

	// Check that mem2 has higher score (higher weight)
	assert.Greater(t, scores[mem2.ID], scores[mem3.ID])
}

func TestRetriever_graphSearch_DefaultDepth(t *testing.T) {
	retriever, ts, graphIndex, cleanup := setupTestRetrieverWithGraph(t)
	defer cleanup()

	ctx := context.Background()

	// Create memories
	mem1, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Source memory",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	mem2, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Related memory",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	// Create graph relationship
	err = graphIndex.AddEdge(ctx, mem1.ID, mem2.ID, graph.RelationRelatedTo, 0.9)
	require.NoError(t, err)

	// Test with GraphDepth = 0 (should default to 2)
	opts := &RetrieveOptions{
		RelatedTo:  []string{mem1.ID},
		GraphDepth: 0,
		Limit:      10,
	}

	scores, err := retriever.graphSearch(ctx, opts)
	require.NoError(t, err)
	assert.Greater(t, len(scores), 0)
}

func TestRetriever_graphSearch_MultipleRelatedTo(t *testing.T) {
	retriever, ts, graphIndex, cleanup := setupTestRetrieverWithGraph(t)
	defer cleanup()

	ctx := context.Background()

	// Create memories
	mem1, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Source memory 1",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	mem2, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Source memory 2",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	mem3, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Related to both",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	mem4, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Related to mem1 only",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	// Create graph relationships
	err = graphIndex.AddEdge(ctx, mem1.ID, mem3.ID, graph.RelationRelatedTo, 0.8)
	require.NoError(t, err)
	err = graphIndex.AddEdge(ctx, mem2.ID, mem3.ID, graph.RelationRelatedTo, 0.9)
	require.NoError(t, err)
	err = graphIndex.AddEdge(ctx, mem1.ID, mem4.ID, graph.RelationRelatedTo, 0.7)
	require.NoError(t, err)

	// Test with multiple RelatedTo
	opts := &RetrieveOptions{
		RelatedTo:  []string{mem1.ID, mem2.ID},
		GraphDepth: 1,
		Limit:      10,
	}

	scores, err := retriever.graphSearch(ctx, opts)
	require.NoError(t, err)

	// mem3 should have the highest score (related to both sources)
	// The highest score from either path should be kept
	assert.Contains(t, scores, mem3.ID)
	assert.Contains(t, scores, mem4.ID)
}

func TestRetriever_calculateGraphScore(t *testing.T) {
	retriever, ts, graphIndex, cleanup := setupTestRetrieverWithGraph(t)
	defer cleanup()

	ctx := context.Background()

	// Create memories
	mem1, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Source memory",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	mem2, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Directly connected",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	mem3, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Intermediate memory",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	mem4, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Two hops away",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	// Create edges: mem1 -> mem2 (direct), mem1 -> mem3 -> mem4 (2-hop)
	err = graphIndex.AddEdge(ctx, mem1.ID, mem2.ID, graph.RelationRelatedTo, 0.9)
	require.NoError(t, err)
	err = graphIndex.AddEdge(ctx, mem1.ID, mem3.ID, graph.RelationRelatedTo, 0.8)
	require.NoError(t, err)
	err = graphIndex.AddEdge(ctx, mem3.ID, mem4.ID, graph.RelationRelatedTo, 0.7)
	require.NoError(t, err)

	relatedTo := []string{mem1.ID}

	// Direct connection should return 1.0
	score := retriever.calculateGraphScore(ctx, mem2.ID, relatedTo)
	assert.Equal(t, 1.0, score, "direct connection should return 1.0")

	// 2-hop connection should return reduced score
	score = retriever.calculateGraphScore(ctx, mem4.ID, relatedTo)
	assert.Less(t, score, 1.0, "2-hop should have lower score")
	assert.Greater(t, score, 0.0, "2-hop should still have some score")
}

func TestRetriever_calculateGraphScore_ReverseEdge(t *testing.T) {
	retriever, ts, graphIndex, cleanup := setupTestRetrieverWithGraph(t)
	defer cleanup()

	ctx := context.Background()

	// Create memories
	mem1, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Source memory",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	mem2, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Points to source",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	// Create reverse edge: mem2 -> mem1
	err = graphIndex.AddEdge(ctx, mem2.ID, mem1.ID, graph.RelationRelatedTo, 0.9)
	require.NoError(t, err)

	relatedTo := []string{mem1.ID}

	// Should still find the connection in reverse
	score := retriever.calculateGraphScore(ctx, mem2.ID, relatedTo)
	assert.Equal(t, 1.0, score, "reverse edge should return 1.0")
}

func TestRetriever_calculateGraphScore_NoGraphIndex(t *testing.T) {
	retriever, _, cleanup := setupTestRetriever(t)
	defer cleanup()

	ctx := context.Background()

	// Without graph index, should return 0
	score := retriever.calculateGraphScore(ctx, "some-id", []string{"other-id"})
	assert.Equal(t, 0.0, score, "no graph index should return 0")
}

func TestRetriever_calculateGraphScore_EmptyRelatedTo(t *testing.T) {
	retriever, _, _, cleanup := setupTestRetrieverWithGraph(t)
	defer cleanup()

	ctx := context.Background()

	// Empty relatedTo should return 0
	score := retriever.calculateGraphScore(ctx, "some-id", []string{})
	assert.Equal(t, 0.0, score, "empty relatedTo should return 0")
}

func TestRetriever_calculateGraphScore_NoConnection(t *testing.T) {
	retriever, ts, _, cleanup := setupTestRetrieverWithGraph(t)
	defer cleanup()

	ctx := context.Background()

	// Create memories without any connection
	mem1, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Isolated memory 1",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	mem2, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Isolated memory 2",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	// No edges between them
	score := retriever.calculateGraphScore(ctx, mem2.ID, []string{mem1.ID})
	assert.Equal(t, 0.0, score, "no connection should return 0")
}

func TestRetriever_Retrieve_GraphScoreInResults(t *testing.T) {
	retriever, ts, graphIndex, cleanup := setupTestRetrieverWithGraph(t)
	defer cleanup()

	ctx := context.Background()

	// Create memories
	mem1, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Source memory for graph test",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	mem2, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Directly related memory for graph test",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	// Index in vector index
	for _, mem := range []*storage.Memory{mem1, mem2} {
		emb, err := retriever.embedder.Embed(ctx, mem.Content)
		require.NoError(t, err)
		err = retriever.vectorIndex.Add(ctx, mem.ID, emb)
		require.NoError(t, err)
	}

	// Create graph relationship
	err = graphIndex.AddEdge(ctx, mem1.ID, mem2.ID, graph.RelationRelatedTo, 0.9)
	require.NoError(t, err)

	// Search with graph
	results, err := retriever.Retrieve(ctx, "memory for graph test", &RetrieveOptions{
		Namespace: "test",
		Limit:     10,
		UseVector: true,
		UseText:   false,
		UseGraph:  true,
		RelatedTo: []string{mem1.ID},
	})

	require.NoError(t, err)
	assert.NotNil(t, results)

	// Find mem2 in results and check its graph score
	for _, item := range results.Items {
		if item.Memory.ID == mem2.ID {
			// GraphScore should be non-zero because it's directly related
			assert.Greater(t, item.GraphScore, 0.0, "related memory should have positive graph score")
		}
	}
}

func TestRetriever_Retrieve_GraphDisabled(t *testing.T) {
	retriever, ts, graphIndex, cleanup := setupTestRetrieverWithGraph(t)
	defer cleanup()

	ctx := context.Background()

	// Create memories
	mem1, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Source memory",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	mem2, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Related memory",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	// Index in vector index
	for _, mem := range []*storage.Memory{mem1, mem2} {
		emb, err := retriever.embedder.Embed(ctx, mem.Content)
		require.NoError(t, err)
		err = retriever.vectorIndex.Add(ctx, mem.ID, emb)
		require.NoError(t, err)
	}

	// Create graph relationship
	err = graphIndex.AddEdge(ctx, mem1.ID, mem2.ID, graph.RelationRelatedTo, 0.9)
	require.NoError(t, err)

	// Search with graph disabled
	results, err := retriever.Retrieve(ctx, "memory", &RetrieveOptions{
		Namespace: "test",
		Limit:     10,
		UseVector: true,
		UseText:   false,
		UseGraph:  false, // Explicitly disabled
		RelatedTo: []string{mem1.ID},
	})

	require.NoError(t, err)
	assert.NotNil(t, results)
}

func TestRetriever_Retrieve_GraphWithNoRelatedTo(t *testing.T) {
	retriever, ts, _, cleanup := setupTestRetrieverWithGraph(t)
	defer cleanup()

	ctx := context.Background()

	// Create memory
	mem1, err := ts.Store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace: "test",
		Content:   "Test memory content",
		Type:      storage.MemoryTypeSemantic,
	})
	require.NoError(t, err)

	// Index
	emb, err := retriever.embedder.Embed(ctx, mem1.Content)
	require.NoError(t, err)
	err = retriever.vectorIndex.Add(ctx, mem1.ID, emb)
	require.NoError(t, err)

	// Search with graph enabled but no RelatedTo - should not trigger graphSearch
	results, err := retriever.Retrieve(ctx, "test memory", &RetrieveOptions{
		Namespace: "test",
		Limit:     10,
		UseVector: true,
		UseText:   false,
		UseGraph:  true,
		RelatedTo: nil, // No RelatedTo
	})

	require.NoError(t, err)
	assert.NotNil(t, results)
}

func BenchmarkRetriever_Retrieve(b *testing.B) {
	// Create test store
	dir, err := os.MkdirTemp("", "maia-retrieval-bench-*")
	require.NoError(b, err)
	defer os.RemoveAll(dir)

	store, err := badger.NewWithPath(dir)
	require.NoError(b, err)
	defer store.Close()

	// Create mock embedder
	embedder := embedding.NewMockProvider(128)

	// Create vector index
	vectorIndex := vector.NewBruteForceIndex(128)
	defer vectorIndex.Close()

	ctx := context.Background()

	// Create and index 100 memories
	for i := 0; i < 100; i++ {
		mem, _ := store.CreateMemory(ctx, &storage.CreateMemoryInput{
			Namespace: "test",
			Content:   "Benchmark test memory content for retrieval performance testing",
			Type:      storage.MemoryTypeSemantic,
		})
		emb, _ := embedder.Embed(ctx, mem.Content)
		_ = vectorIndex.Add(ctx, mem.ID, emb)
	}

	retriever := NewRetriever(store, vectorIndex, nil, embedder, DefaultConfig())
	opts := &RetrieveOptions{
		Namespace: "test",
		Limit:     10,
		UseVector: true,
		UseText:   false,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = retriever.Retrieve(ctx, "test memory", opts)
	}
}

func BenchmarkRetriever_Retrieve_WithGraph(b *testing.B) {
	// Create test store
	dir, err := os.MkdirTemp("", "maia-retrieval-bench-graph-*")
	require.NoError(b, err)
	defer os.RemoveAll(dir)

	store, err := badger.NewWithPath(dir)
	require.NoError(b, err)
	defer store.Close()

	// Create mock embedder
	embedder := embedding.NewMockProvider(128)

	// Create vector index
	vectorIndex := vector.NewBruteForceIndex(128)
	defer vectorIndex.Close()

	// Create graph index
	graphIndex := graph.NewInMemoryIndex()
	defer graphIndex.Close()

	ctx := context.Background()

	// Create and index 100 memories with graph relationships
	var memIDs []string
	for i := 0; i < 100; i++ {
		mem, _ := store.CreateMemory(ctx, &storage.CreateMemoryInput{
			Namespace: "test",
			Content:   "Benchmark test memory content for retrieval performance testing",
			Type:      storage.MemoryTypeSemantic,
		})
		memIDs = append(memIDs, mem.ID)
		emb, _ := embedder.Embed(ctx, mem.Content)
		_ = vectorIndex.Add(ctx, mem.ID, emb)

		// Create some graph edges
		if i > 0 {
			_ = graphIndex.AddEdge(ctx, memIDs[i-1], mem.ID, graph.RelationRelatedTo, 0.8)
		}
	}

	retriever := NewRetrieverWithGraph(store, vectorIndex, nil, graphIndex, embedder, DefaultConfig())
	opts := &RetrieveOptions{
		Namespace: "test",
		Limit:     10,
		UseVector: true,
		UseText:   false,
		UseGraph:  true,
		RelatedTo: []string{memIDs[0]},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = retriever.Retrieve(ctx, "test memory", opts)
	}
}
