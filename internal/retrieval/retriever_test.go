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
		vectorIndex.Add(ctx, mem.ID, emb)
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
