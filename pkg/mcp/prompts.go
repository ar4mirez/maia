package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ar4mirez/maia/internal/storage"
)

// registerPrompts registers all MAIA prompts with the MCP server.
func (s *Server) registerPrompts() {
	// Context injection prompt
	s.server.AddPrompt(&mcp.Prompt{
		Name:        "inject_context",
		Description: "Inject relevant MAIA memories as context for a conversation",
		Arguments: []*mcp.PromptArgument{
			{
				Name:        "topic",
				Description: "The topic or query to build context for",
				Required:    true,
			},
			{
				Name:        "namespace",
				Description: "The namespace to search in (default: 'default')",
				Required:    false,
			},
			{
				Name:        "max_tokens",
				Description: "Maximum tokens for the context (default: 2000)",
				Required:    false,
			},
		},
	}, s.handleInjectContextPrompt)

	// Summarize memories prompt
	s.server.AddPrompt(&mcp.Prompt{
		Name:        "summarize_memories",
		Description: "Generate a summary of memories in a namespace",
		Arguments: []*mcp.PromptArgument{
			{
				Name:        "namespace",
				Description: "The namespace to summarize",
				Required:    true,
			},
			{
				Name:        "focus",
				Description: "Optional focus area for the summary",
				Required:    false,
			},
		},
	}, s.handleSummarizePrompt)

	// Memory exploration prompt
	s.server.AddPrompt(&mcp.Prompt{
		Name:        "explore_memories",
		Description: "Explore and analyze stored memories to find patterns or insights",
		Arguments: []*mcp.PromptArgument{
			{
				Name:        "namespace",
				Description: "The namespace to explore",
				Required:    true,
			},
			{
				Name:        "question",
				Description: "A question to guide the exploration",
				Required:    false,
			},
		},
	}, s.handleExplorePrompt)
}

func (s *Server) handleInjectContextPrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	topic := req.Params.Arguments["topic"]
	if topic == "" {
		return nil, fmt.Errorf("topic is required")
	}

	namespace := req.Params.Arguments["namespace"]
	if namespace == "" {
		namespace = "default"
	}

	// Search for relevant memories
	results, err := s.store.SearchMemories(ctx, &storage.SearchOptions{
		Namespace: namespace,
		Limit:     20,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search memories: %w", err)
	}

	// Build context block
	var contextParts []string
	contextParts = append(contextParts, fmt.Sprintf("# Relevant Context for: %s\n", topic))
	contextParts = append(contextParts, "The following information has been retrieved from MAIA memory:\n")

	for i, r := range results {
		contextParts = append(contextParts, fmt.Sprintf("%d. %s", i+1, r.Memory.Content))
	}

	if len(results) == 0 {
		contextParts = append(contextParts, "(No relevant memories found)")
	}

	contextText := strings.Join(contextParts, "\n")

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("Context for '%s' from namespace '%s'", topic, namespace),
		Messages: []*mcp.PromptMessage{
			{
				Role: "user",
				Content: &mcp.TextContent{Text: fmt.Sprintf(
					"I have the following context from my memory system:\n\n%s\n\nNow, regarding: %s",
					contextText, topic,
				)},
			},
		},
	}, nil
}

func (s *Server) handleSummarizePrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	namespace := req.Params.Arguments["namespace"]
	if namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}

	focus := req.Params.Arguments["focus"]

	// Get memories from namespace
	memories, err := s.store.ListMemories(ctx, namespace, &storage.ListOptions{Limit: 50})
	if err != nil {
		return nil, fmt.Errorf("failed to list memories: %w", err)
	}

	// Build memory list
	var memoryList []string
	for i, m := range memories {
		memoryList = append(memoryList, fmt.Sprintf("%d. [%s] %s", i+1, m.Type, m.Content))
	}

	memoriesText := strings.Join(memoryList, "\n")
	if len(memories) == 0 {
		memoriesText = "(No memories in this namespace)"
	}

	var promptText string
	if focus != "" {
		promptText = fmt.Sprintf(
			"Please summarize the following memories from namespace '%s', focusing on '%s':\n\n%s",
			namespace, focus, memoriesText,
		)
	} else {
		promptText = fmt.Sprintf(
			"Please provide a comprehensive summary of the following memories from namespace '%s':\n\n%s",
			namespace, memoriesText,
		)
	}

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("Summarize memories in namespace '%s'", namespace),
		Messages: []*mcp.PromptMessage{
			{
				Role:    "user",
				Content: &mcp.TextContent{Text: promptText},
			},
		},
	}, nil
}

func (s *Server) handleExplorePrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	namespace := req.Params.Arguments["namespace"]
	if namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}

	question := req.Params.Arguments["question"]

	// Get memories and stats
	memories, err := s.store.ListMemories(ctx, namespace, &storage.ListOptions{Limit: 50})
	if err != nil {
		return nil, fmt.Errorf("failed to list memories: %w", err)
	}

	// Categorize memories
	typeCounts := make(map[string]int)
	var recentMemories []*storage.Memory

	for _, m := range memories {
		typeCounts[string(m.Type)]++
		if len(recentMemories) < 5 {
			recentMemories = append(recentMemories, m)
		}
	}

	// Build exploration prompt
	var parts []string
	parts = append(parts, fmt.Sprintf("# Memory Exploration: Namespace '%s'\n", namespace))
	parts = append(parts, fmt.Sprintf("Total memories: %d\n", len(memories)))

	parts = append(parts, "\n## Memory Types:")
	for t, count := range typeCounts {
		parts = append(parts, fmt.Sprintf("- %s: %d", t, count))
	}

	parts = append(parts, "\n## Sample Memories:")
	for i, m := range recentMemories {
		parts = append(parts, fmt.Sprintf("%d. [%s] %s", i+1, m.Type, truncate(m.Content, 150)))
	}

	parts = append(parts, "\n## All Memory Contents:")
	for i, m := range memories {
		parts = append(parts, fmt.Sprintf("%d. %s", i+1, m.Content))
	}

	var promptText string
	if question != "" {
		promptText = fmt.Sprintf(
			"%s\n\nBased on these memories, please answer: %s\n\nAlso identify any patterns, connections, or insights.",
			strings.Join(parts, "\n"), question,
		)
	} else {
		promptText = fmt.Sprintf(
			"%s\n\nPlease analyze these memories and identify:\n1. Key themes and patterns\n2. Potential connections between memories\n3. Any gaps or areas that could benefit from more information\n4. Actionable insights",
			strings.Join(parts, "\n"),
		)
	}

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("Explore memories in namespace '%s'", namespace),
		Messages: []*mcp.PromptMessage{
			{
				Role:    "user",
				Content: &mcp.TextContent{Text: promptText},
			},
		},
	}, nil
}
