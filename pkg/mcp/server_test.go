package mcp

import (
	"context"
	"fmt"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ar4mirez/maia/internal/config"
	"github.com/ar4mirez/maia/internal/embedding"
	"github.com/ar4mirez/maia/internal/index/fulltext"
	"github.com/ar4mirez/maia/internal/index/vector"
	"github.com/ar4mirez/maia/internal/storage"
	"github.com/ar4mirez/maia/internal/storage/badger"
)

func setupTestServer(t *testing.T) (*Server, func()) {
	t.Helper()

	// Create temporary store
	tmpDir := t.TempDir()
	store, err := badger.New(&badger.Options{DataDir: tmpDir})
	require.NoError(t, err)

	// Create mock provider
	provider := embedding.NewMockProvider(384)

	// Create vector index
	vectorIndex := vector.NewHNSWIndex(vector.DefaultConfig(384))

	// Create text index
	textIndex, err := fulltext.NewBleveIndex(fulltext.Config{InMemory: true})
	require.NoError(t, err)

	// Create server
	server, err := NewServer(&Options{
		Config:      &config.Config{},
		Store:       store,
		Provider:    provider,
		VectorIndex: vectorIndex,
		TextIndex:   textIndex,
	})
	require.NoError(t, err)

	cleanup := func() {
		server.Close()
		textIndex.Close()
	}

	return server, cleanup
}

func TestNewServer_WithNilOptions(t *testing.T) {
	_, err := NewServer(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "options cannot be nil")
}

func TestNewServer_WithValidOptions(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	assert.NotNil(t, server)
	assert.NotNil(t, server.MCPServer())
}

func TestServer_RememberAndRecall(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Create a memory directly using the store
	mem, err := server.store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace:  "default",
		Content:    "Test memory content for MCP server",
		Type:       storage.MemoryTypeSemantic,
		Confidence: 1.0,
		Source:     storage.MemorySourceUser,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, mem.ID)

	// List memories
	memories, err := server.store.ListMemories(ctx, "default", &storage.ListOptions{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, memories, 1)
	assert.Equal(t, "Test memory content for MCP server", memories[0].Content)
}

func TestServer_Close(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	err := server.Close()
	assert.NoError(t, err)

	// Close again should be safe (idempotent)
	err = server.Close()
	assert.NoError(t, err)
}

// Integration tests for MCP tools

func TestTool_Remember(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Test remembering content
	result, output, err := server.handleRemember(ctx, nil, RememberInput{
		Content:   "User prefers dark mode",
		Namespace: "test-ns",
		Type:      "semantic",
		Tags:      []string{"preferences", "ui"},
		Metadata:  map[string]string{"source": "settings"},
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, output.ID)
	assert.Equal(t, "test-ns", output.Namespace)
	assert.Contains(t, result.Content[0].(*mcp.TextContent).Text, "Remembered")

	// Verify memory was stored
	memories, err := server.store.ListMemories(ctx, "test-ns", &storage.ListOptions{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, memories, 1)
	assert.Equal(t, "User prefers dark mode", memories[0].Content)
}

func TestTool_Remember_DefaultNamespace(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	_, output, err := server.handleRemember(ctx, nil, RememberInput{
		Content: "Test content",
	})
	require.NoError(t, err)
	assert.Equal(t, "default", output.Namespace)
}

func TestTool_Remember_EmptyContent(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	_, _, err := server.handleRemember(ctx, nil, RememberInput{
		Content: "",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "content is required")
}

func TestTool_Remember_MemoryTypes(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		inputType    string
		expectedType storage.MemoryType
	}{
		{"semantic", storage.MemoryTypeSemantic},
		{"episodic", storage.MemoryTypeEpisodic},
		{"working", storage.MemoryTypeWorking},
		{"", storage.MemoryTypeSemantic}, // default
	}

	for _, tt := range tests {
		t.Run(tt.inputType, func(t *testing.T) {
			_, output, err := server.handleRemember(ctx, nil, RememberInput{
				Content:   "Test " + tt.inputType,
				Namespace: "type-test",
				Type:      tt.inputType,
			})
			require.NoError(t, err)

			mem, err := server.store.GetMemory(ctx, output.ID)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedType, mem.Type)
		})
	}
}

func TestTool_Recall(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Store some memories first
	_, _, err := server.handleRemember(ctx, nil, RememberInput{
		Content:   "User prefers dark mode theme",
		Namespace: "test-ns",
		Tags:      []string{"preferences"},
	})
	require.NoError(t, err)

	_, _, err = server.handleRemember(ctx, nil, RememberInput{
		Content:   "User lives in New York",
		Namespace: "test-ns",
		Tags:      []string{"location"},
	})
	require.NoError(t, err)

	// Recall memories
	result, output, err := server.handleRecall(ctx, nil, RecallInput{
		Query:     "user preferences",
		Namespace: "test-ns",
		Limit:     10,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.GreaterOrEqual(t, output.TotalFound, 1)
	assert.Contains(t, result.Content[0].(*mcp.TextContent).Text, "Found")
}

func TestTool_Recall_EmptyQuery(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	_, _, err := server.handleRecall(ctx, nil, RecallInput{
		Query: "",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "query is required")
}

func TestTool_Recall_NoResults(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	result, output, err := server.handleRecall(ctx, nil, RecallInput{
		Query:     "nonexistent topic xyz123",
		Namespace: "empty-ns",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, output.TotalFound)
	assert.Contains(t, result.Content[0].(*mcp.TextContent).Text, "No relevant memories")
}

func TestTool_Forget_ByID(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Store a memory
	_, output, err := server.handleRemember(ctx, nil, RememberInput{
		Content:   "Content to forget",
		Namespace: "test-ns",
	})
	require.NoError(t, err)
	memID := output.ID

	// Forget without confirm should fail
	_, _, err = server.handleForget(ctx, nil, ForgetInput{
		ID:      memID,
		Confirm: false,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must set confirm=true")

	// Forget with confirm
	result, forgetOutput, err := server.handleForget(ctx, nil, ForgetInput{
		ID:      memID,
		Confirm: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, forgetOutput.Deleted)
	assert.Contains(t, forgetOutput.IDs, memID)
	assert.Contains(t, result.Content[0].(*mcp.TextContent).Text, "Deleted 1")

	// Verify memory was deleted
	_, err = server.store.GetMemory(ctx, memID)
	require.Error(t, err)
}

func TestTool_Forget_NoIDOrQuery(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	_, _, err := server.handleForget(ctx, nil, ForgetInput{
		Confirm: true,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must provide either id or query")
}

func TestTool_ListMemories(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Store some memories
	for i := 0; i < 5; i++ {
		_, _, err := server.handleRemember(ctx, nil, RememberInput{
			Content:   fmt.Sprintf("Memory %d", i),
			Namespace: "list-test",
		})
		require.NoError(t, err)
	}

	// List memories
	result, output, err := server.handleListMemories(ctx, nil, ListMemoriesInput{
		Namespace: "list-test",
		Limit:     10,
	})
	require.NoError(t, err)
	assert.Equal(t, 5, output.Total)
	assert.Len(t, output.Memories, 5)
	assert.Contains(t, result.Content[0].(*mcp.TextContent).Text, "Found 5 memories")
}

func TestTool_ListMemories_EmptyNamespace(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	result, output, err := server.handleListMemories(ctx, nil, ListMemoriesInput{
		Namespace: "empty-ns",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, output.Total)
	assert.Contains(t, result.Content[0].(*mcp.TextContent).Text, "No memories")
}

func TestTool_ListMemories_Pagination(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Store 10 memories
	for i := 0; i < 10; i++ {
		_, _, err := server.handleRemember(ctx, nil, RememberInput{
			Content:   fmt.Sprintf("Memory %d", i),
			Namespace: "pagination-test",
		})
		require.NoError(t, err)
	}

	// First page
	_, output1, err := server.handleListMemories(ctx, nil, ListMemoriesInput{
		Namespace: "pagination-test",
		Limit:     5,
		Offset:    0,
	})
	require.NoError(t, err)
	assert.Equal(t, 5, output1.Total)

	// Second page
	_, output2, err := server.handleListMemories(ctx, nil, ListMemoriesInput{
		Namespace: "pagination-test",
		Limit:     5,
		Offset:    5,
	})
	require.NoError(t, err)
	assert.Equal(t, 5, output2.Total)

	// Verify different memories
	assert.NotEqual(t, output1.Memories[0].ID, output2.Memories[0].ID)
}

func TestTool_GetContext(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Store some memories
	_, _, err := server.handleRemember(ctx, nil, RememberInput{
		Content:   "The user's favorite color is blue",
		Namespace: "context-test",
	})
	require.NoError(t, err)

	_, _, err = server.handleRemember(ctx, nil, RememberInput{
		Content:   "The user works as a software engineer",
		Namespace: "context-test",
	})
	require.NoError(t, err)

	// Get context
	result, output, err := server.handleGetContext(ctx, nil, GetContextInput{
		Query:       "user information",
		Namespace:   "context-test",
		TokenBudget: 2000,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, output.Context)
	assert.GreaterOrEqual(t, output.MemoriesUsed, 1)
	assert.Greater(t, output.TokensUsed, 0)
	assert.NotNil(t, result)
}

func TestTool_GetContext_WithSystemPrompt(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Store a memory
	_, _, err := server.handleRemember(ctx, nil, RememberInput{
		Content:   "Test content",
		Namespace: "system-prompt-test",
	})
	require.NoError(t, err)

	// Get context with system prompt
	_, output, err := server.handleGetContext(ctx, nil, GetContextInput{
		Query:        "test",
		Namespace:    "system-prompt-test",
		TokenBudget:  2000,
		SystemPrompt: "You are a helpful assistant.",
	})
	require.NoError(t, err)
	assert.Contains(t, output.Context, "You are a helpful assistant.")
}

func TestTool_GetContext_EmptyQuery(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	_, _, err := server.handleGetContext(ctx, nil, GetContextInput{
		Query: "",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "query is required")
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is a longer string", 10, "this is..."},
		{"", 5, ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := truncate(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Resource handler tests

func TestResource_Namespaces(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Create some namespaces explicitly
	_, err := server.store.CreateNamespace(ctx, &storage.CreateNamespaceInput{
		Name: "ns1",
	})
	require.NoError(t, err)

	_, err = server.store.CreateNamespace(ctx, &storage.CreateNamespaceInput{
		Name: "ns2",
	})
	require.NoError(t, err)

	// Call the resource handler
	result, err := server.handleNamespacesResource(ctx, &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{
			URI: "maia://namespaces",
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Contents, 1)
	assert.Equal(t, "application/json", result.Contents[0].MIMEType)
	// Verify content contains JSON with namespaces
	assert.Contains(t, result.Contents[0].Text, "ns1")
	assert.Contains(t, result.Contents[0].Text, "ns2")
}

func TestResource_Memories(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Store some memories
	_, _, err := server.handleRemember(ctx, nil, RememberInput{
		Content:   "Memory one content",
		Namespace: "mem-test",
		Tags:      []string{"tag1"},
	})
	require.NoError(t, err)

	_, _, err = server.handleRemember(ctx, nil, RememberInput{
		Content:   "Memory two content",
		Namespace: "mem-test",
		Tags:      []string{"tag2"},
	})
	require.NoError(t, err)

	// Call the resource handler
	result, err := server.handleMemoriesResource(ctx, &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{
			URI: "maia://namespace/mem-test/memories",
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Contents, 1)
	assert.Equal(t, "application/json", result.Contents[0].MIMEType)
	assert.Contains(t, result.Contents[0].Text, "Memory one content")
	assert.Contains(t, result.Contents[0].Text, "Memory two content")
}

func TestResource_Memories_DefaultNamespace(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Call with empty namespace (should default to "default")
	result, err := server.handleMemoriesResource(ctx, &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{
			URI: "maia://namespace//memories", // Missing namespace
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestResource_Memory(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Store a memory
	_, output, err := server.handleRemember(ctx, nil, RememberInput{
		Content:   "Specific memory content",
		Namespace: "single-mem",
	})
	require.NoError(t, err)

	// Get specific memory
	result, err := server.handleMemoryResource(ctx, &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{
			URI: "maia://memory/" + output.ID,
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Contents, 1)
	assert.Contains(t, result.Contents[0].Text, "Specific memory content")
	assert.Contains(t, result.Contents[0].Text, output.ID)
}

func TestResource_Memory_NotFound(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Try to get non-existent memory
	_, err := server.handleMemoryResource(ctx, &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{
			URI: "maia://memory/nonexistent-id",
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get memory")
}

func TestResource_Memory_MissingID(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Try to get memory with empty ID
	_, err := server.handleMemoryResource(ctx, &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{
			URI: "maia://memory/",
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "memory ID is required")
}

func TestResource_Stats(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Store some memories to have stats
	_, _, err := server.handleRemember(ctx, nil, RememberInput{
		Content:   "Stats test memory",
		Namespace: "stats-test",
	})
	require.NoError(t, err)

	// Get stats
	result, err := server.handleStatsResource(ctx, &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{
			URI: "maia://stats",
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Contents, 1)
	assert.Equal(t, "application/json", result.Contents[0].MIMEType)
}

func TestExtractURIParam(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		param    string
		expected string
	}{
		{
			name:     "extract namespace",
			uri:      "maia://namespace/myns/memories",
			param:    "namespace",
			expected: "myns",
		},
		{
			name:     "extract namespace without trailing path",
			uri:      "maia://namespace/myns",
			param:    "namespace",
			expected: "myns",
		},
		{
			name:     "extract memory id",
			uri:      "maia://memory/abc123",
			param:    "id",
			expected: "abc123",
		},
		{
			name:     "empty namespace",
			uri:      "maia://namespace/",
			param:    "namespace",
			expected: "",
		},
		{
			name:     "short uri for namespace",
			uri:      "maia://namespace",
			param:    "namespace",
			expected: "",
		},
		{
			name:     "short uri for memory",
			uri:      "maia://memory",
			param:    "id",
			expected: "",
		},
		{
			name:     "unknown param",
			uri:      "maia://foo/bar",
			param:    "unknown",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractURIParam(tt.uri, tt.param)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Prompt handler tests

func TestPrompt_InjectContext(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Store some memories
	_, _, err := server.handleRemember(ctx, nil, RememberInput{
		Content:   "User likes coffee",
		Namespace: "prompt-test",
	})
	require.NoError(t, err)

	// Call inject context prompt
	result, err := server.handleInjectContextPrompt(ctx, &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{
			Name: "inject_context",
			Arguments: map[string]string{
				"topic":     "user preferences",
				"namespace": "prompt-test",
			},
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.Messages)
	assert.Contains(t, result.Description, "user preferences")
}

func TestPrompt_InjectContext_DefaultNamespace(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Call without namespace
	result, err := server.handleInjectContextPrompt(ctx, &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{
			Name: "inject_context",
			Arguments: map[string]string{
				"topic": "test topic",
			},
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Contains(t, result.Description, "default")
}

func TestPrompt_InjectContext_MissingTopic(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Call without topic
	_, err := server.handleInjectContextPrompt(ctx, &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{
			Name:      "inject_context",
			Arguments: map[string]string{},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "topic is required")
}

func TestPrompt_InjectContext_NoResults(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Call on empty namespace
	result, err := server.handleInjectContextPrompt(ctx, &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{
			Name: "inject_context",
			Arguments: map[string]string{
				"topic":     "unknown topic",
				"namespace": "empty-ns",
			},
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
	// Should still return valid result even with no memories
}

func TestPrompt_Summarize(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Store some memories
	_, _, err := server.handleRemember(ctx, nil, RememberInput{
		Content:   "First memory for summary",
		Namespace: "summary-test",
		Type:      "semantic",
	})
	require.NoError(t, err)

	_, _, err = server.handleRemember(ctx, nil, RememberInput{
		Content:   "Second memory for summary",
		Namespace: "summary-test",
		Type:      "episodic",
	})
	require.NoError(t, err)

	// Call summarize prompt
	result, err := server.handleSummarizePrompt(ctx, &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{
			Name: "summarize_memories",
			Arguments: map[string]string{
				"namespace": "summary-test",
			},
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.Messages)
	assert.Contains(t, result.Description, "summary-test")
	// Check message content contains memories
	content := result.Messages[0].Content.(*mcp.TextContent).Text
	assert.Contains(t, content, "First memory")
	assert.Contains(t, content, "Second memory")
}

func TestPrompt_Summarize_WithFocus(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Store a memory
	_, _, err := server.handleRemember(ctx, nil, RememberInput{
		Content:   "Memory content",
		Namespace: "focus-test",
	})
	require.NoError(t, err)

	// Call summarize prompt with focus
	result, err := server.handleSummarizePrompt(ctx, &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{
			Name: "summarize_memories",
			Arguments: map[string]string{
				"namespace": "focus-test",
				"focus":     "technical details",
			},
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
	content := result.Messages[0].Content.(*mcp.TextContent).Text
	assert.Contains(t, content, "technical details")
}

func TestPrompt_Summarize_MissingNamespace(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Call without namespace
	_, err := server.handleSummarizePrompt(ctx, &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{
			Name:      "summarize_memories",
			Arguments: map[string]string{},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "namespace is required")
}

func TestPrompt_Summarize_EmptyNamespace(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Call on empty namespace
	result, err := server.handleSummarizePrompt(ctx, &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{
			Name: "summarize_memories",
			Arguments: map[string]string{
				"namespace": "empty-namespace",
			},
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
	content := result.Messages[0].Content.(*mcp.TextContent).Text
	assert.Contains(t, content, "No memories")
}

func TestPrompt_Explore(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Store some memories of different types
	_, _, err := server.handleRemember(ctx, nil, RememberInput{
		Content:   "Semantic memory content",
		Namespace: "explore-test",
		Type:      "semantic",
	})
	require.NoError(t, err)

	_, _, err = server.handleRemember(ctx, nil, RememberInput{
		Content:   "Episodic memory content",
		Namespace: "explore-test",
		Type:      "episodic",
	})
	require.NoError(t, err)

	_, _, err = server.handleRemember(ctx, nil, RememberInput{
		Content:   "Working memory content",
		Namespace: "explore-test",
		Type:      "working",
	})
	require.NoError(t, err)

	// Call explore prompt
	result, err := server.handleExplorePrompt(ctx, &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{
			Name: "explore_memories",
			Arguments: map[string]string{
				"namespace": "explore-test",
			},
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.Messages)
	content := result.Messages[0].Content.(*mcp.TextContent).Text
	assert.Contains(t, content, "Memory Exploration")
	assert.Contains(t, content, "explore-test")
	assert.Contains(t, content, "Memory Types")
	assert.Contains(t, content, "semantic")
	assert.Contains(t, content, "episodic")
}

func TestPrompt_Explore_WithQuestion(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Store a memory
	_, _, err := server.handleRemember(ctx, nil, RememberInput{
		Content:   "Some memory",
		Namespace: "question-test",
	})
	require.NoError(t, err)

	// Call explore prompt with question
	result, err := server.handleExplorePrompt(ctx, &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{
			Name: "explore_memories",
			Arguments: map[string]string{
				"namespace": "question-test",
				"question":  "What patterns exist?",
			},
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
	content := result.Messages[0].Content.(*mcp.TextContent).Text
	assert.Contains(t, content, "What patterns exist?")
}

func TestPrompt_Explore_MissingNamespace(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Call without namespace
	_, err := server.handleExplorePrompt(ctx, &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{
			Name:      "explore_memories",
			Arguments: map[string]string{},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "namespace is required")
}

// Integration test for full workflow
func TestIntegration_FullWorkflow(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()
	namespace := "integration-test"

	// 1. Remember some facts
	_, mem1, err := server.handleRemember(ctx, nil, RememberInput{
		Content:   "User's name is Alice",
		Namespace: namespace,
		Type:      "semantic",
		Tags:      []string{"identity"},
	})
	require.NoError(t, err)

	_, _, err = server.handleRemember(ctx, nil, RememberInput{
		Content:   "Alice is 30 years old",
		Namespace: namespace,
		Type:      "semantic",
		Tags:      []string{"identity"},
	})
	require.NoError(t, err)

	_, _, err = server.handleRemember(ctx, nil, RememberInput{
		Content:   "Alice prefers Python for programming",
		Namespace: namespace,
		Type:      "semantic",
		Tags:      []string{"preferences", "programming"},
	})
	require.NoError(t, err)

	// 2. List all memories
	_, listOutput, err := server.handleListMemories(ctx, nil, ListMemoriesInput{
		Namespace: namespace,
	})
	require.NoError(t, err)
	assert.Equal(t, 3, listOutput.Total)

	// 3. Recall with a query
	_, recallOutput, err := server.handleRecall(ctx, nil, RecallInput{
		Query:     "What is Alice's name and age?",
		Namespace: namespace,
		Limit:     10,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, recallOutput.TotalFound, 1)

	// 4. Get assembled context
	_, contextOutput, err := server.handleGetContext(ctx, nil, GetContextInput{
		Query:       "Tell me about Alice",
		Namespace:   namespace,
		TokenBudget: 1000,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, contextOutput.MemoriesUsed, 1)

	// 5. Forget a specific memory
	_, forgetOutput, err := server.handleForget(ctx, nil, ForgetInput{
		ID:      mem1.ID,
		Confirm: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, forgetOutput.Deleted)

	// 6. Verify it's gone
	_, listOutput2, err := server.handleListMemories(ctx, nil, ListMemoriesInput{
		Namespace: namespace,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, listOutput2.Total)
}
