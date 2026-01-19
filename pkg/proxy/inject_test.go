package proxy

import (
	"context"
	"testing"

	mcontext "github.com/ar4mirez/maia/internal/context"
	"github.com/ar4mirez/maia/internal/embedding"
	"github.com/ar4mirez/maia/internal/index/fulltext"
	"github.com/ar4mirez/maia/internal/index/vector"
	"github.com/ar4mirez/maia/internal/retrieval"
	"github.com/ar4mirez/maia/internal/storage"
	"github.com/ar4mirez/maia/internal/storage/badger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInjector_buildQuery(t *testing.T) {
	injector := &Injector{}

	tests := []struct {
		name     string
		messages []ChatMessage
		want     string
	}{
		{
			name: "single user message",
			messages: []ChatMessage{
				{Role: "user", Content: "What is the weather today?"},
			},
			want: "What is the weather today?",
		},
		{
			name: "multiple user messages",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello"},
				{Role: "assistant", Content: "Hi there!"},
				{Role: "user", Content: "What is the weather?"},
			},
			want: "Hello What is the weather?",
		},
		{
			name: "system and user messages",
			messages: []ChatMessage{
				{Role: "system", Content: "You are helpful."},
				{Role: "user", Content: "Tell me about Go"},
			},
			want: "Tell me about Go",
		},
		{
			name:     "empty messages",
			messages: []ChatMessage{},
			want:     "",
		},
		{
			name: "only system message",
			messages: []ChatMessage{
				{Role: "system", Content: "You are helpful."},
			},
			want: "",
		},
		{
			name: "three user messages - takes last 3",
			messages: []ChatMessage{
				{Role: "user", Content: "First"},
				{Role: "assistant", Content: "Response 1"},
				{Role: "user", Content: "Second"},
				{Role: "assistant", Content: "Response 2"},
				{Role: "user", Content: "Third"},
				{Role: "assistant", Content: "Response 3"},
				{Role: "user", Content: "Fourth"},
			},
			want: "Second Third Fourth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := injector.buildQuery(tt.messages)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormatContext(t *testing.T) {
	content := "User prefers dark mode."
	formatted := formatContext(content)

	assert.Contains(t, formatted, "[Relevant context from memory]")
	assert.Contains(t, formatted, "User prefers dark mode.")
	assert.Contains(t, formatted, "[End of context]")
}

func TestInjectIntoSystem(t *testing.T) {
	tests := []struct {
		name     string
		messages []ChatMessage
		context  string
		checkFn  func(t *testing.T, result []ChatMessage)
	}{
		{
			name: "existing system message",
			messages: []ChatMessage{
				{Role: "system", Content: "You are helpful."},
				{Role: "user", Content: "Hello"},
			},
			context: "Context here",
			checkFn: func(t *testing.T, result []ChatMessage) {
				assert.Len(t, result, 2)
				assert.Equal(t, "system", result[0].Role)
				content := result[0].GetContentString()
				assert.Contains(t, content, "Context here")
				assert.Contains(t, content, "You are helpful.")
			},
		},
		{
			name: "no system message",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello"},
			},
			context: "Context here",
			checkFn: func(t *testing.T, result []ChatMessage) {
				assert.Len(t, result, 2)
				assert.Equal(t, "system", result[0].Role)
				assert.Equal(t, "Context here", result[0].GetContentString())
				assert.Equal(t, "user", result[1].Role)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := injectIntoSystem(tt.messages, tt.context)
			tt.checkFn(t, result)
		})
	}
}

func TestInjectIntoFirstUser(t *testing.T) {
	tests := []struct {
		name     string
		messages []ChatMessage
		context  string
		checkFn  func(t *testing.T, result []ChatMessage)
	}{
		{
			name: "prepend to first user",
			messages: []ChatMessage{
				{Role: "system", Content: "You are helpful."},
				{Role: "user", Content: "Hello"},
			},
			context: "Context here",
			checkFn: func(t *testing.T, result []ChatMessage) {
				assert.Len(t, result, 2)
				content := result[1].GetContentString()
				assert.Contains(t, content, "Context here")
				assert.Contains(t, content, "Hello")
			},
		},
		{
			name: "no user message",
			messages: []ChatMessage{
				{Role: "system", Content: "You are helpful."},
			},
			context: "Context here",
			checkFn: func(t *testing.T, result []ChatMessage) {
				assert.Len(t, result, 1)
				assert.Equal(t, "You are helpful.", result[0].GetContentString())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := injectIntoFirstUser(tt.messages, tt.context)
			tt.checkFn(t, result)
		})
	}
}

func TestInjectBeforeLastUser(t *testing.T) {
	tests := []struct {
		name     string
		messages []ChatMessage
		context  string
		checkFn  func(t *testing.T, result []ChatMessage)
	}{
		{
			name: "insert before last user",
			messages: []ChatMessage{
				{Role: "system", Content: "You are helpful."},
				{Role: "user", Content: "First question"},
				{Role: "assistant", Content: "First response"},
				{Role: "user", Content: "Second question"},
			},
			context: "Context here",
			checkFn: func(t *testing.T, result []ChatMessage) {
				assert.Len(t, result, 5)
				assert.Equal(t, "system", result[0].Role)
				assert.Equal(t, "user", result[1].Role)
				assert.Equal(t, "assistant", result[2].Role)
				assert.Equal(t, "system", result[3].Role)
				assert.Equal(t, "Context here", result[3].GetContentString())
				assert.Equal(t, "user", result[4].Role)
				assert.Equal(t, "Second question", result[4].GetContentString())
			},
		},
		{
			name: "no user message",
			messages: []ChatMessage{
				{Role: "system", Content: "You are helpful."},
			},
			context: "Context here",
			checkFn: func(t *testing.T, result []ChatMessage) {
				assert.Len(t, result, 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := injectBeforeLastUser(tt.messages, tt.context)
			tt.checkFn(t, result)
		})
	}
}

func TestInjector_injectIntoMessages(t *testing.T) {
	injector := &Injector{}

	messages := []ChatMessage{
		{Role: "user", Content: "Hello"},
	}

	// Test with empty context
	result := injector.injectIntoMessages(messages, "", PositionSystem)
	assert.Len(t, result, 1)

	// Test with context
	result = injector.injectIntoMessages(messages, "Context", PositionSystem)
	assert.Len(t, result, 2)
	assert.Equal(t, "system", result[0].Role)

	// Test different positions
	result = injector.injectIntoMessages(messages, "Context", PositionFirstUser)
	assert.Contains(t, result[0].GetContentString(), "Context")

	// Test default position
	result = injector.injectIntoMessages(messages, "Context", "unknown")
	assert.Equal(t, "system", result[0].Role)
}

func setupInjectorTestDeps(t *testing.T) (*retrieval.Retriever, *mcontext.Assembler, storage.Store, func()) {
	t.Helper()

	// Create temporary store
	tmpDir := t.TempDir()
	store, err := badger.New(&badger.Options{DataDir: tmpDir})
	require.NoError(t, err)

	// Create mock embedding provider
	provider := embedding.NewMockProvider(384)

	// Create vector index
	vectorIndex := vector.NewHNSWIndex(vector.DefaultConfig(384))

	// Create text index
	textIndex, err := fulltext.NewBleveIndex(fulltext.Config{InMemory: true})
	require.NoError(t, err)

	// Create retriever
	retriever := retrieval.NewRetriever(store, vectorIndex, textIndex, provider, retrieval.DefaultConfig())

	// Create assembler
	assembler := mcontext.NewAssembler(mcontext.DefaultAssemblerConfig())

	cleanup := func() {
		textIndex.Close()
	}

	return retriever, assembler, store, cleanup
}

func TestNewInjector(t *testing.T) {
	retriever, assembler, _, cleanup := setupInjectorTestDeps(t)
	defer cleanup()

	injector := NewInjector(retriever, assembler)

	assert.NotNil(t, injector)
	assert.Equal(t, retriever, injector.retriever)
	assert.Equal(t, assembler, injector.assembler)
}

func TestInjector_InjectContext_NilDependencies(t *testing.T) {
	// Test with nil retriever and assembler
	injector := &Injector{
		retriever: nil,
		assembler: nil,
	}

	ctx := context.Background()
	messages := []ChatMessage{
		{Role: "user", Content: "Hello"},
	}

	result, err := injector.InjectContext(ctx, messages, &InjectionOptions{
		Namespace:   "default",
		TokenBudget: 2000,
		Position:    PositionSystem,
	})

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, messages, result.Messages)
	assert.Equal(t, 0, result.MemoriesUsed)
	assert.Equal(t, 0, result.TokensInjected)
}

func TestInjector_InjectContext_EmptyQuery(t *testing.T) {
	retriever, assembler, _, cleanup := setupInjectorTestDeps(t)
	defer cleanup()

	injector := NewInjector(retriever, assembler)

	ctx := context.Background()
	// Only system message - no user messages to build query from
	messages := []ChatMessage{
		{Role: "system", Content: "You are helpful."},
	}

	result, err := injector.InjectContext(ctx, messages, &InjectionOptions{
		Namespace:   "default",
		TokenBudget: 2000,
		Position:    PositionSystem,
	})

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, messages, result.Messages)
}

func TestInjector_InjectContext_NoResults(t *testing.T) {
	retriever, assembler, _, cleanup := setupInjectorTestDeps(t)
	defer cleanup()

	injector := NewInjector(retriever, assembler)

	ctx := context.Background()
	messages := []ChatMessage{
		{Role: "user", Content: "What is the weather in Antarctica?"},
	}

	result, err := injector.InjectContext(ctx, messages, &InjectionOptions{
		Namespace:   "empty-namespace",
		TokenBudget: 2000,
		Position:    PositionSystem,
	})

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, messages, result.Messages)
	assert.Equal(t, 0, result.MemoriesUsed)
	assert.Greater(t, result.QueryTime.Nanoseconds(), int64(0))
}

func TestInjector_InjectContext_WithMemories(t *testing.T) {
	retriever, assembler, store, cleanup := setupInjectorTestDeps(t)
	defer cleanup()

	ctx := context.Background()

	// Create some memories
	_, err := store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace:  "test-ns",
		Content:    "User prefers dark mode for the application interface",
		Type:       storage.MemoryTypeSemantic,
		Confidence: 1.0,
		Source:     storage.MemorySourceUser,
	})
	require.NoError(t, err)

	_, err = store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace:  "test-ns",
		Content:    "User's favorite programming language is Go",
		Type:       storage.MemoryTypeSemantic,
		Confidence: 1.0,
		Source:     storage.MemorySourceUser,
	})
	require.NoError(t, err)

	injector := NewInjector(retriever, assembler)

	messages := []ChatMessage{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "What are my preferences?"},
	}

	result, err := injector.InjectContext(ctx, messages, &InjectionOptions{
		Namespace:   "test-ns",
		TokenBudget: 2000,
		Position:    PositionSystem,
	})

	require.NoError(t, err)
	assert.NotNil(t, result)
	// The result should have memories injected
	assert.GreaterOrEqual(t, result.MemoriesUsed, 1)
	assert.Greater(t, result.TokensInjected, 0)
	// Check that context was injected into system message
	assert.Contains(t, result.Messages[0].GetContentString(), "[Relevant context from memory]")
}

func TestInjector_InjectContext_DifferentPositions(t *testing.T) {
	retriever, assembler, store, cleanup := setupInjectorTestDeps(t)
	defer cleanup()

	ctx := context.Background()

	// Create a memory
	_, err := store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace:  "pos-test",
		Content:    "Important context information",
		Type:       storage.MemoryTypeSemantic,
		Confidence: 1.0,
		Source:     storage.MemorySourceUser,
	})
	require.NoError(t, err)

	injector := NewInjector(retriever, assembler)

	messages := []ChatMessage{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "First question"},
		{Role: "assistant", Content: "First answer"},
		{Role: "user", Content: "What context do you have?"},
	}

	tests := []struct {
		name     string
		position ContextPosition
	}{
		{"system position", PositionSystem},
		{"first user position", PositionFirstUser},
		{"before last position", PositionBeforeLast},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := injector.InjectContext(ctx, messages, &InjectionOptions{
				Namespace:   "pos-test",
				TokenBudget: 2000,
				Position:    tt.position,
			})

			require.NoError(t, err)
			assert.NotNil(t, result)
			assert.GreaterOrEqual(t, result.MemoriesUsed, 1)
		})
	}
}
