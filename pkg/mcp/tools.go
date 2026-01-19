package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	ctxpkg "github.com/ar4mirez/maia/internal/context"
	"github.com/ar4mirez/maia/internal/retrieval"
	"github.com/ar4mirez/maia/internal/storage"
)

// registerTools registers all MAIA tools with the MCP server.
func (s *Server) registerTools() {
	// Remember tool - stores information in memory
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "remember",
		Description: "Store information in MAIA's memory for later retrieval. Use this to save facts, preferences, context, or any information that should be remembered.",
	}, s.handleRemember)

	// Recall tool - retrieves information from memory
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "recall",
		Description: "Retrieve relevant information from MAIA's memory based on a query. Returns contextually relevant memories with position-aware assembly.",
	}, s.handleRecall)

	// Forget tool - removes information from memory
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "forget",
		Description: "Remove specific memories from MAIA's storage. Use with caution as this permanently deletes information.",
	}, s.handleForget)

	// List tool - lists memories in a namespace
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "list_memories",
		Description: "List memories in a specific namespace with optional filtering.",
	}, s.handleListMemories)

	// Context tool - assembles context for LLM consumption
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_context",
		Description: "Assemble a position-aware context from relevant memories optimized for LLM consumption.",
	}, s.handleGetContext)
}

// RememberInput defines the input schema for the remember tool.
type RememberInput struct {
	Content   string            `json:"content" jsonschema:"The content to remember"`
	Namespace string            `json:"namespace,omitempty" jsonschema:"The namespace to store the memory in (default: 'default')"`
	Type      string            `json:"type,omitempty" jsonschema:"The type of memory: semantic, episodic, or working (default: 'semantic')"`
	Tags      []string          `json:"tags,omitempty" jsonschema:"Tags to categorize the memory"`
	Metadata  map[string]string `json:"metadata,omitempty" jsonschema:"Additional metadata for the memory"`
}

// RememberOutput defines the output for the remember tool.
type RememberOutput struct {
	ID        string    `json:"id"`
	Namespace string    `json:"namespace"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Server) handleRemember(ctx context.Context, req *mcp.CallToolRequest, input RememberInput) (*mcp.CallToolResult, RememberOutput, error) {
	if input.Content == "" {
		return nil, RememberOutput{}, fmt.Errorf("content is required")
	}

	namespace := input.Namespace
	if namespace == "" {
		namespace = "default"
	}

	memType := storage.MemoryTypeSemantic
	switch input.Type {
	case "episodic":
		memType = storage.MemoryTypeEpisodic
	case "working":
		memType = storage.MemoryTypeWorking
	}

	// Convert metadata
	var metadata map[string]interface{}
	if len(input.Metadata) > 0 {
		metadata = make(map[string]interface{}, len(input.Metadata))
		for k, v := range input.Metadata {
			metadata[k] = v
		}
	}

	// Generate embedding
	embedding, err := s.provider.Embed(ctx, input.Content)
	if err != nil {
		// Non-fatal: store without embedding
		embedding = nil
	}

	// Create memory
	mem, err := s.store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace:  namespace,
		Content:    input.Content,
		Type:       memType,
		Tags:       input.Tags,
		Metadata:   metadata,
		Embedding:  embedding,
		Confidence: 1.0,
		Source:     storage.MemorySourceUser,
	})
	if err != nil {
		return nil, RememberOutput{}, fmt.Errorf("failed to store memory: %w", err)
	}

	output := RememberOutput{
		ID:        mem.ID,
		Namespace: mem.Namespace,
		CreatedAt: mem.CreatedAt,
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Remembered: %s (ID: %s)", truncate(input.Content, 50), mem.ID)},
		},
	}, output, nil
}

// RecallInput defines the input schema for the recall tool.
type RecallInput struct {
	Query       string   `json:"query" jsonschema:"The query to search for relevant memories"`
	Namespace   string   `json:"namespace,omitempty" jsonschema:"The namespace to search in (default: 'default')"`
	Limit       int      `json:"limit,omitempty" jsonschema:"Maximum number of memories to return (default: 10)"`
	Tags        []string `json:"tags,omitempty" jsonschema:"Filter by tags"`
	Types       []string `json:"types,omitempty" jsonschema:"Filter by memory types"`
	TokenBudget int      `json:"token_budget,omitempty" jsonschema:"Maximum tokens for context assembly (default: 4000)"`
}

// RecallOutput defines the output for the recall tool.
type RecallOutput struct {
	Memories   []MemoryResult `json:"memories"`
	TotalFound int            `json:"total_found"`
}

// MemoryResult represents a single memory in recall results.
type MemoryResult struct {
	ID         string    `json:"id"`
	Content    string    `json:"content"`
	Type       string    `json:"type"`
	Score      float64   `json:"score"`
	Tags       []string  `json:"tags,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	AccessedAt time.Time `json:"accessed_at"`
}

func (s *Server) handleRecall(ctx context.Context, req *mcp.CallToolRequest, input RecallInput) (*mcp.CallToolResult, RecallOutput, error) {
	if input.Query == "" {
		return nil, RecallOutput{}, fmt.Errorf("query is required")
	}

	namespace := input.Namespace
	if namespace == "" {
		namespace = "default"
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}

	// Convert types
	var memTypes []storage.MemoryType
	for _, t := range input.Types {
		switch t {
		case "semantic":
			memTypes = append(memTypes, storage.MemoryTypeSemantic)
		case "episodic":
			memTypes = append(memTypes, storage.MemoryTypeEpisodic)
		case "working":
			memTypes = append(memTypes, storage.MemoryTypeWorking)
		}
	}

	// Search with retriever
	results, err := s.retriever.Retrieve(ctx, input.Query, &retrieval.RetrieveOptions{
		Namespace: namespace,
		Tags:      input.Tags,
		Types:     memTypes,
		Limit:     limit,
		UseVector: true,
		UseText:   true,
	})
	if err != nil {
		return nil, RecallOutput{}, fmt.Errorf("failed to search memories: %w", err)
	}

	// Convert to output format
	memories := make([]MemoryResult, 0, len(results.Items))
	for _, r := range results.Items {
		memories = append(memories, MemoryResult{
			ID:         r.Memory.ID,
			Content:    r.Memory.Content,
			Type:       string(r.Memory.Type),
			Score:      r.Score,
			Tags:       r.Memory.Tags,
			CreatedAt:  r.Memory.CreatedAt,
			AccessedAt: r.Memory.AccessedAt,
		})

		// Touch memory to update access stats
		_ = s.store.TouchMemory(ctx, r.Memory.ID)
	}

	output := RecallOutput{
		Memories:   memories,
		TotalFound: len(memories),
	}

	// Build text response
	var text string
	if len(memories) == 0 {
		text = "No relevant memories found."
	} else {
		text = fmt.Sprintf("Found %d relevant memories:\n\n", len(memories))
		for i, m := range memories {
			text += fmt.Sprintf("%d. [Score: %.2f] %s\n", i+1, m.Score, truncate(m.Content, 100))
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}, output, nil
}

// ForgetInput defines the input schema for the forget tool.
type ForgetInput struct {
	ID        string `json:"id,omitempty" jsonschema:"The ID of the specific memory to forget"`
	Namespace string `json:"namespace,omitempty" jsonschema:"The namespace to forget memories from"`
	Query     string `json:"query,omitempty" jsonschema:"A query to find and forget matching memories"`
	Confirm   bool   `json:"confirm" jsonschema:"Must be true to confirm deletion"`
}

// ForgetOutput defines the output for the forget tool.
type ForgetOutput struct {
	Deleted int      `json:"deleted"`
	IDs     []string `json:"ids"`
}

func (s *Server) handleForget(ctx context.Context, req *mcp.CallToolRequest, input ForgetInput) (*mcp.CallToolResult, ForgetOutput, error) {
	if !input.Confirm {
		return nil, ForgetOutput{}, fmt.Errorf("must set confirm=true to delete memories")
	}

	var deletedIDs []string

	if input.ID != "" {
		// Delete specific memory
		err := s.store.DeleteMemory(ctx, input.ID)
		if err != nil {
			return nil, ForgetOutput{}, fmt.Errorf("failed to delete memory: %w", err)
		}
		deletedIDs = append(deletedIDs, input.ID)
	} else if input.Query != "" {
		// Search and delete matching memories
		namespace := input.Namespace
		if namespace == "" {
			namespace = "default"
		}

		results, err := s.retriever.Retrieve(ctx, input.Query, &retrieval.RetrieveOptions{
			Namespace: namespace,
			Limit:     100,
			UseVector: true,
			UseText:   true,
		})
		if err != nil {
			return nil, ForgetOutput{}, fmt.Errorf("failed to search memories: %w", err)
		}

		for _, r := range results.Items {
			if err := s.store.DeleteMemory(ctx, r.Memory.ID); err == nil {
				deletedIDs = append(deletedIDs, r.Memory.ID)
			}
		}
	} else {
		return nil, ForgetOutput{}, fmt.Errorf("must provide either id or query")
	}

	output := ForgetOutput{
		Deleted: len(deletedIDs),
		IDs:     deletedIDs,
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Deleted %d memories", len(deletedIDs))},
		},
	}, output, nil
}

// ListMemoriesInput defines the input schema for the list_memories tool.
type ListMemoriesInput struct {
	Namespace string `json:"namespace,omitempty" jsonschema:"The namespace to list memories from (default: 'default')"`
	Limit     int    `json:"limit,omitempty" jsonschema:"Maximum number of memories to return (default: 20)"`
	Offset    int    `json:"offset,omitempty" jsonschema:"Number of memories to skip for pagination"`
}

// ListMemoriesOutput defines the output for the list_memories tool.
type ListMemoriesOutput struct {
	Memories []MemoryResult `json:"memories"`
	Total    int            `json:"total"`
}

func (s *Server) handleListMemories(ctx context.Context, req *mcp.CallToolRequest, input ListMemoriesInput) (*mcp.CallToolResult, ListMemoriesOutput, error) {
	namespace := input.Namespace
	if namespace == "" {
		namespace = "default"
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}

	memories, err := s.store.ListMemories(ctx, namespace, &storage.ListOptions{
		Limit:  limit,
		Offset: input.Offset,
	})
	if err != nil {
		return nil, ListMemoriesOutput{}, fmt.Errorf("failed to list memories: %w", err)
	}

	results := make([]MemoryResult, 0, len(memories))
	for _, m := range memories {
		results = append(results, MemoryResult{
			ID:         m.ID,
			Content:    m.Content,
			Type:       string(m.Type),
			Tags:       m.Tags,
			CreatedAt:  m.CreatedAt,
			AccessedAt: m.AccessedAt,
		})
	}

	output := ListMemoriesOutput{
		Memories: results,
		Total:    len(results),
	}

	var text string
	if len(results) == 0 {
		text = fmt.Sprintf("No memories in namespace '%s'", namespace)
	} else {
		text = fmt.Sprintf("Found %d memories in namespace '%s':\n\n", len(results), namespace)
		for i, m := range results {
			text += fmt.Sprintf("%d. [%s] %s\n", i+1, m.Type, truncate(m.Content, 80))
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}, output, nil
}

// GetContextInput defines the input schema for the get_context tool.
type GetContextInput struct {
	Query        string `json:"query" jsonschema:"The query or topic to build context for"`
	Namespace    string `json:"namespace,omitempty" jsonschema:"The namespace to search in (default: 'default')"`
	TokenBudget  int    `json:"token_budget,omitempty" jsonschema:"Maximum tokens for the context (default: 4000)"`
	SystemPrompt string `json:"system_prompt,omitempty" jsonschema:"Optional system prompt to include at the beginning"`
}

// GetContextOutput defines the output for the get_context tool.
type GetContextOutput struct {
	Context      string `json:"context"`
	TokensUsed   int    `json:"tokens_used"`
	MemoriesUsed int    `json:"memories_used"`
}

func (s *Server) handleGetContext(ctx context.Context, req *mcp.CallToolRequest, input GetContextInput) (*mcp.CallToolResult, GetContextOutput, error) {
	if input.Query == "" {
		return nil, GetContextOutput{}, fmt.Errorf("query is required")
	}

	namespace := input.Namespace
	if namespace == "" {
		namespace = "default"
	}

	tokenBudget := input.TokenBudget
	if tokenBudget <= 0 {
		tokenBudget = 4000
	}

	// Search for relevant memories
	results, err := s.retriever.Retrieve(ctx, input.Query, &retrieval.RetrieveOptions{
		Namespace: namespace,
		Limit:     50,
		UseVector: true,
		UseText:   true,
	})
	if err != nil {
		return nil, GetContextOutput{}, fmt.Errorf("failed to search memories: %w", err)
	}

	// Assemble context
	assembled, err := s.assembler.Assemble(ctx, results, &ctxpkg.AssembleOptions{
		TokenBudget:  tokenBudget,
		SystemPrompt: input.SystemPrompt,
	})
	if err != nil {
		return nil, GetContextOutput{}, fmt.Errorf("failed to assemble context: %w", err)
	}

	output := GetContextOutput{
		Context:      assembled.Content,
		TokensUsed:   assembled.TokenCount,
		MemoriesUsed: len(assembled.Memories),
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: assembled.Content},
		},
	}, output, nil
}

// truncate truncates a string to the specified length.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
