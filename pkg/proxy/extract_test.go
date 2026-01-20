package proxy

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ar4mirez/maia/internal/storage"
)

func TestExtractor_Extract(t *testing.T) {
	extractor := NewExtractor(nil)

	tests := []struct {
		name             string
		assistantContent string
		userMessages     []string
		wantCount        int
		checkFn          func(t *testing.T, result *ExtractionResult)
	}{
		{
			name:             "empty content",
			assistantContent: "",
			userMessages:     nil,
			wantCount:        0,
		},
		{
			name:             "user preference pattern",
			assistantContent: "I understand that you prefer dark mode for your IDE.",
			userMessages:     nil,
			wantCount:        1,
			checkFn: func(t *testing.T, result *ExtractionResult) {
				assert.Equal(t, "preference", result.Memories[0].Category)
				assert.Contains(t, result.Memories[0].Content, "dark mode")
			},
		},
		{
			name:             "user fact pattern",
			assistantContent: "I see that you work at a tech company in San Francisco.",
			userMessages:     nil,
			wantCount:        1,
			checkFn: func(t *testing.T, result *ExtractionResult) {
				assert.Equal(t, "fact", result.Memories[0].Category)
			},
		},
		{
			name:             "explicit memory marker",
			assistantContent: "I'll remember that you need the reports by Friday.",
			userMessages:     nil,
			wantCount:        1,
			checkFn: func(t *testing.T, result *ExtractionResult) {
				assert.Equal(t, "explicit", result.Memories[0].Category)
			},
		},
		{
			name:             "instruction pattern",
			assistantContent: "You should always use TypeScript for new projects.",
			userMessages:     nil,
			wantCount:        1,
			checkFn: func(t *testing.T, result *ExtractionResult) {
				assert.Equal(t, "instruction", result.Memories[0].Category)
			},
		},
		{
			name:             "important info pattern",
			assistantContent: "Important: The deadline is next Monday.",
			userMessages:     nil,
			wantCount:        1,
			checkFn: func(t *testing.T, result *ExtractionResult) {
				assert.Equal(t, "important", result.Memories[0].Category)
			},
		},
		{
			name:             "multiple patterns",
			assistantContent: "You prefer dark mode. I'll remember that you work at Acme Corp.",
			userMessages:     nil,
			wantCount:        2,
		},
		{
			name:             "user message extraction",
			assistantContent: "Got it!",
			userMessages:     []string{"I prefer using Go for backend development."},
			wantCount:        1,
			checkFn: func(t *testing.T, result *ExtractionResult) {
				assert.Equal(t, "user_declaration", result.Memories[0].Category)
				assert.Equal(t, "user_message", result.Memories[0].Source)
			},
		},
		{
			name:             "identity extraction",
			assistantContent: "Nice to meet you!",
			userMessages:     []string{"My name is John"},
			wantCount:        1,
			checkFn: func(t *testing.T, result *ExtractionResult) {
				assert.Equal(t, "identity", result.Memories[0].Category)
			},
		},
		{
			name:             "no duplicates in single match",
			assistantContent: "I understand that you prefer dark mode for all your IDEs.",
			userMessages:     nil,
			wantCount:        1,
		},
		{
			name:             "too short content",
			assistantContent: "You like it.",
			userMessages:     nil,
			wantCount:        0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractor.Extract(context.Background(), tt.assistantContent, tt.userMessages)
			require.NoError(t, err)
			assert.Len(t, result.Memories, tt.wantCount)
			if tt.checkFn != nil && tt.wantCount > 0 {
				tt.checkFn(t, result)
			}
		})
	}
}

func TestCategoryToMemoryType(t *testing.T) {
	tests := []struct {
		category string
		expected storage.MemoryType
	}{
		{"preference", storage.MemoryTypeSemantic},
		{"instruction", storage.MemoryTypeSemantic},
		{"fact", storage.MemoryTypeEpisodic},
		{"identity", storage.MemoryTypeEpisodic},
		{"user_declaration", storage.MemoryTypeEpisodic},
		{"explicit", storage.MemoryTypeSemantic},
		{"important", storage.MemoryTypeSemantic},
		{"unknown", storage.MemoryTypeSemantic},
	}

	for _, tt := range tests {
		t.Run(tt.category, func(t *testing.T) {
			assert.Equal(t, tt.expected, categoryToMemoryType(tt.category))
		})
	}
}

func TestNormalizeContent(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "hello world"},
		{"  Multiple   Spaces  ", "multiple spaces"},
		{"Trailing punctuation.", "trailing punctuation"},
		{"UPPERCASE!", "uppercase"},
		{"Mixed CASE with SPACES", "mixed case with spaces"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, normalizeContent(tt.input))
		})
	}
}

func TestExtractor_Store_NilStore(t *testing.T) {
	extractor := NewExtractor(nil)

	memories := []*ExtractedMemory{
		{Content: "Test memory", Category: "preference"},
	}

	// Should not panic with nil store
	err := extractor.Store(context.Background(), "test", memories)
	assert.NoError(t, err)
}

func TestExtractor_Store_EmptyMemories(t *testing.T) {
	extractor := NewExtractor(nil)

	err := extractor.Store(context.Background(), "test", []*ExtractedMemory{})
	assert.NoError(t, err)
}

func TestDefaultExtractionPatterns(t *testing.T) {
	patterns := defaultExtractionPatterns()
	assert.NotEmpty(t, patterns)

	// Verify each pattern compiles and has required fields
	for _, p := range patterns {
		assert.NotNil(t, p.regex)
		assert.NotEmpty(t, p.category)
		assert.Greater(t, p.minLen, 0)
	}
}

// mockStore implements storage.Store interface for testing.
type mockStore struct {
	memories   []*storage.Memory
	createErr  error
	memCounter int
}

func (m *mockStore) CreateMemory(ctx context.Context, input *storage.CreateMemoryInput) (*storage.Memory, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	m.memCounter++
	mem := &storage.Memory{
		ID:        fmt.Sprintf("mem_%d", m.memCounter),
		Namespace: input.Namespace,
		Content:   input.Content,
		Type:      input.Type,
	}
	m.memories = append(m.memories, mem)
	return mem, nil
}

func (m *mockStore) GetMemory(ctx context.Context, id string) (*storage.Memory, error) {
	return nil, nil
}

func (m *mockStore) UpdateMemory(ctx context.Context, id string, input *storage.UpdateMemoryInput) (*storage.Memory, error) {
	return nil, nil
}

func (m *mockStore) DeleteMemory(ctx context.Context, id string) error {
	return nil
}

func (m *mockStore) ListMemories(ctx context.Context, namespace string, opts *storage.ListOptions) ([]*storage.Memory, error) {
	return nil, nil
}

func (m *mockStore) SearchMemories(ctx context.Context, opts *storage.SearchOptions) ([]*storage.SearchResult, error) {
	return nil, nil
}

func (m *mockStore) TouchMemory(ctx context.Context, id string) error {
	return nil
}

func (m *mockStore) CreateNamespace(ctx context.Context, input *storage.CreateNamespaceInput) (*storage.Namespace, error) {
	return nil, nil
}

func (m *mockStore) GetNamespace(ctx context.Context, id string) (*storage.Namespace, error) {
	return nil, nil
}

func (m *mockStore) GetNamespaceByName(ctx context.Context, name string) (*storage.Namespace, error) {
	return nil, nil
}

func (m *mockStore) UpdateNamespace(ctx context.Context, id string, config *storage.NamespaceConfig) (*storage.Namespace, error) {
	return nil, nil
}

func (m *mockStore) DeleteNamespace(ctx context.Context, id string) error {
	return nil
}

func (m *mockStore) ListNamespaces(ctx context.Context, opts *storage.ListOptions) ([]*storage.Namespace, error) {
	return nil, nil
}

func (m *mockStore) BatchCreateMemories(ctx context.Context, inputs []*storage.CreateMemoryInput) ([]*storage.Memory, error) {
	return nil, nil
}

func (m *mockStore) BatchDeleteMemories(ctx context.Context, ids []string) error {
	return nil
}

func (m *mockStore) Close() error {
	return nil
}

func (m *mockStore) Stats(ctx context.Context) (*storage.StoreStats, error) {
	return nil, nil
}

func TestExtractor_Store_WithMockStore(t *testing.T) {
	store := &mockStore{}
	extractor := NewExtractor(store)

	memories := []*ExtractedMemory{
		{Content: "User prefers dark mode", Category: "preference", Source: "assistant_response"},
		{Content: "User works at Acme Corp", Category: "fact", Source: "assistant_response"},
	}

	err := extractor.Store(context.Background(), "test-namespace", memories)
	assert.NoError(t, err)
	assert.Len(t, store.memories, 2)
	assert.Equal(t, "test-namespace", store.memories[0].Namespace)
	assert.Equal(t, "User prefers dark mode", store.memories[0].Content)
}

func TestExtractor_Store_WithCreateError(t *testing.T) {
	store := &mockStore{
		createErr: fmt.Errorf("database error"),
	}
	extractor := NewExtractor(store)

	memories := []*ExtractedMemory{
		{Content: "Test memory", Category: "preference"},
	}

	// Store should not return error even if create fails (it logs and continues)
	err := extractor.Store(context.Background(), "test", memories)
	assert.NoError(t, err)
	assert.Empty(t, store.memories) // No memories stored due to error
}

func TestExtractor_Extract_DeduplicatesFromUserMessages(t *testing.T) {
	extractor := NewExtractor(nil)

	// Extract from assistant content that contains the same info as user message
	result, err := extractor.Extract(
		context.Background(),
		"I understand you prefer dark mode!",
		[]string{"I prefer dark mode for everything."},
	)
	require.NoError(t, err)
	// Should have extractions but deduplicated
	assert.NotEmpty(t, result.Memories)
}

func TestExtractor_extractFromUserMessage_NoMatches(t *testing.T) {
	extractor := NewExtractor(nil)

	memories := extractor.extractFromUserMessage("Hello, how are you?")
	assert.Empty(t, memories)
}

func TestExtractor_extractFromUserMessage_CallMePattern(t *testing.T) {
	extractor := NewExtractor(nil)

	memories := extractor.extractFromUserMessage("Call me Alex")
	assert.NotEmpty(t, memories)
	assert.Equal(t, "identity", memories[0].Category)
}
