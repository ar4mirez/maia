package proxy

import (
	"context"
	"fmt"
	"strings"
	"time"

	mcontext "github.com/ar4mirez/maia/internal/context"
	"github.com/ar4mirez/maia/internal/retrieval"
)

// ContextPosition defines where to inject context in the messages.
type ContextPosition string

const (
	// PositionSystem prepends context to the system message.
	PositionSystem ContextPosition = "system"
	// PositionFirstUser prepends context to the first user message.
	PositionFirstUser ContextPosition = "first_user"
	// PositionBeforeLast inserts context before the last user message.
	PositionBeforeLast ContextPosition = "before_last"
)

// Injector handles context injection into chat messages.
type Injector struct {
	retriever *retrieval.Retriever
	assembler *mcontext.Assembler
}

// NewInjector creates a new context injector.
func NewInjector(retriever *retrieval.Retriever, assembler *mcontext.Assembler) *Injector {
	return &Injector{
		retriever: retriever,
		assembler: assembler,
	}
}

// InjectionResult contains the result of context injection.
type InjectionResult struct {
	Messages       []ChatMessage
	MemoriesUsed   int
	TokensInjected int
	QueryTime      time.Duration
}

// InjectContext retrieves and injects relevant context into the messages.
func (i *Injector) InjectContext(
	ctx context.Context,
	messages []ChatMessage,
	opts *InjectionOptions,
) (*InjectionResult, error) {
	if i.retriever == nil || i.assembler == nil {
		return &InjectionResult{Messages: messages}, nil
	}

	// Extract query from messages
	query := i.buildQuery(messages)
	if query == "" {
		return &InjectionResult{Messages: messages}, nil
	}

	start := time.Now()

	// Retrieve relevant memories
	retrievalOpts := &retrieval.RetrieveOptions{
		Namespace: opts.Namespace,
		Limit:     50,
		MinScore:  0.3,
		UseVector: true,
		UseText:   true,
	}

	results, err := i.retriever.Retrieve(ctx, query, retrievalOpts)
	if err != nil {
		return nil, fmt.Errorf("retrieve memories: %w", err)
	}

	if results.Total == 0 {
		return &InjectionResult{
			Messages:  messages,
			QueryTime: time.Since(start),
		}, nil
	}

	// Assemble context
	assembleOpts := &mcontext.AssembleOptions{
		TokenBudget:   opts.TokenBudget,
		IncludeScores: false,
	}

	assembled, err := i.assembler.Assemble(ctx, results, assembleOpts)
	if err != nil {
		return nil, fmt.Errorf("assemble context: %w", err)
	}

	if assembled.Content == "" {
		return &InjectionResult{
			Messages:  messages,
			QueryTime: time.Since(start),
		}, nil
	}

	// Inject context into messages
	injectedMessages := i.injectIntoMessages(messages, assembled.Content, opts.Position)

	return &InjectionResult{
		Messages:       injectedMessages,
		MemoriesUsed:   len(assembled.Memories),
		TokensInjected: assembled.TokenCount,
		QueryTime:      time.Since(start),
	}, nil
}

// InjectionOptions configures context injection.
type InjectionOptions struct {
	Namespace   string
	TokenBudget int
	Position    ContextPosition
}

// buildQuery extracts a query from the messages for retrieval.
func (i *Injector) buildQuery(messages []ChatMessage) string {
	var parts []string

	// Include last few user messages for context
	userMsgCount := 0
	for j := len(messages) - 1; j >= 0 && userMsgCount < 3; j-- {
		if messages[j].Role == "user" {
			content := messages[j].GetContentString()
			if content != "" {
				parts = append([]string{content}, parts...)
				userMsgCount++
			}
		}
	}

	return strings.Join(parts, " ")
}

// injectIntoMessages injects context into the messages at the specified position.
func (i *Injector) injectIntoMessages(
	messages []ChatMessage,
	contextContent string,
	position ContextPosition,
) []ChatMessage {
	if contextContent == "" {
		return messages
	}

	// Format the context
	formattedContext := formatContext(contextContent)

	// Make a copy of messages
	result := make([]ChatMessage, len(messages))
	copy(result, messages)

	switch position {
	case PositionSystem:
		result = injectIntoSystem(result, formattedContext)
	case PositionFirstUser:
		result = injectIntoFirstUser(result, formattedContext)
	case PositionBeforeLast:
		result = injectBeforeLastUser(result, formattedContext)
	default:
		result = injectIntoSystem(result, formattedContext)
	}

	return result
}

// formatContext formats the context for injection.
func formatContext(content string) string {
	return fmt.Sprintf(`[Relevant context from memory]

%s

[End of context]`, content)
}

// injectIntoSystem injects context into the system message.
func injectIntoSystem(messages []ChatMessage, context string) []ChatMessage {
	// Find system message
	for i := range messages {
		if messages[i].Role == "system" {
			// Prepend context to existing system message
			existingContent := messages[i].GetContentString()
			newContent := context
			if existingContent != "" {
				newContent = context + "\n\n" + existingContent
			}
			messages[i].SetContent(newContent)
			return messages
		}
	}

	// No system message found, create one at the beginning
	systemMsg := ChatMessage{
		Role:    "system",
		Content: context,
	}
	return append([]ChatMessage{systemMsg}, messages...)
}

// injectIntoFirstUser injects context into the first user message.
func injectIntoFirstUser(messages []ChatMessage, context string) []ChatMessage {
	for i := range messages {
		if messages[i].Role == "user" {
			existingContent := messages[i].GetContentString()
			newContent := context + "\n\n" + existingContent
			messages[i].SetContent(newContent)
			return messages
		}
	}
	return messages
}

// injectBeforeLastUser inserts a system message before the last user message.
func injectBeforeLastUser(messages []ChatMessage, context string) []ChatMessage {
	// Find last user message
	lastUserIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			lastUserIdx = i
			break
		}
	}

	if lastUserIdx == -1 {
		return messages
	}

	// Insert system message before last user message
	contextMsg := ChatMessage{
		Role:    "system",
		Content: context,
	}

	result := make([]ChatMessage, 0, len(messages)+1)
	result = append(result, messages[:lastUserIdx]...)
	result = append(result, contextMsg)
	result = append(result, messages[lastUserIdx:]...)

	return result
}
