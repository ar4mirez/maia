package proxy

import (
	"context"
	"regexp"
	"strings"

	"github.com/ar4mirez/maia/internal/storage"
)

// Extractor extracts memories from assistant responses.
type Extractor struct {
	store    storage.Store
	patterns []*extractionPattern
}

// extractionPattern defines a pattern for extracting memories.
type extractionPattern struct {
	regex    *regexp.Regexp
	category string
	minLen   int
}

// NewExtractor creates a new memory extractor.
func NewExtractor(store storage.Store) *Extractor {
	return &Extractor{
		store:    store,
		patterns: defaultExtractionPatterns(),
	}
}

// defaultExtractionPatterns returns the default extraction patterns.
func defaultExtractionPatterns() []*extractionPattern {
	return []*extractionPattern{
		// User preferences
		{
			regex:    regexp.MustCompile(`(?i)(?:you|user)\s+(?:prefer|like|want|enjoy|love)\s+(.+?)(?:\.|$)`),
			category: "preference",
			minLen:   10,
		},
		// User facts
		{
			regex:    regexp.MustCompile(`(?i)(?:you|user)\s+(?:are|work|live|have|use|study)\s+(.+?)(?:\.|$)`),
			category: "fact",
			minLen:   10,
		},
		// Explicit memory markers
		{
			regex:    regexp.MustCompile(`(?i)(?:I'll remember|I've noted|noted that|remembering that)\s+(.+?)(?:\.|$)`),
			category: "explicit",
			minLen:   5,
		},
		// Instructions or settings
		{
			regex:    regexp.MustCompile(`(?i)(?:always|never|don't|do not|please)\s+(.+?)(?:\.|$)`),
			category: "instruction",
			minLen:   15,
		},
		// Important information
		{
			regex:    regexp.MustCompile(`(?i)(?:important|note|remember):\s*(.+?)(?:\.|$)`),
			category: "important",
			minLen:   10,
		},
	}
}

// ExtractionResult contains extracted memories.
type ExtractionResult struct {
	Memories []*ExtractedMemory
}

// ExtractedMemory represents a memory extracted from a response.
type ExtractedMemory struct {
	Content  string
	Category string
	Source   string
}

// Extract extracts potential memories from an assistant response.
func (e *Extractor) Extract(
	ctx context.Context,
	assistantContent string,
	userMessages []string,
) (*ExtractionResult, error) {
	if assistantContent == "" {
		return &ExtractionResult{}, nil
	}

	memories := make([]*ExtractedMemory, 0)
	seen := make(map[string]bool)

	// Try each extraction pattern
	for _, pattern := range e.patterns {
		matches := pattern.regex.FindAllStringSubmatch(assistantContent, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}

			content := strings.TrimSpace(match[1])
			if len(content) < pattern.minLen {
				continue
			}

			// Normalize and dedupe
			normalized := normalizeContent(content)
			if seen[normalized] {
				continue
			}
			seen[normalized] = true

			memories = append(memories, &ExtractedMemory{
				Content:  content,
				Category: pattern.category,
				Source:   "assistant_response",
			})
		}
	}

	// Also extract from user messages for context
	for _, userMsg := range userMessages {
		extracted := e.extractFromUserMessage(userMsg)
		for _, mem := range extracted {
			normalized := normalizeContent(mem.Content)
			if seen[normalized] {
				continue
			}
			seen[normalized] = true
			memories = append(memories, mem)
		}
	}

	return &ExtractionResult{Memories: memories}, nil
}

// extractFromUserMessage extracts memories from user messages.
func (e *Extractor) extractFromUserMessage(content string) []*ExtractedMemory {
	memories := make([]*ExtractedMemory, 0)

	// Patterns for user self-declarations
	patterns := []*extractionPattern{
		{
			regex:    regexp.MustCompile(`(?i)(?:I|my)\s+(?:prefer|like|want|need|am|work|live|have)\s+(.+?)(?:\.|$)`),
			category: "user_declaration",
			minLen:   10,
		},
		{
			regex:    regexp.MustCompile(`(?i)(?:call me|my name is|I'm called)\s+(\w+)`),
			category: "identity",
			minLen:   2,
		},
	}

	for _, pattern := range patterns {
		matches := pattern.regex.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}

			extracted := strings.TrimSpace(match[0]) // Full match for context
			if len(extracted) < pattern.minLen {
				continue
			}

			memories = append(memories, &ExtractedMemory{
				Content:  extracted,
				Category: pattern.category,
				Source:   "user_message",
			})
		}
	}

	return memories
}

// Store stores extracted memories to the storage.
func (e *Extractor) Store(
	ctx context.Context,
	namespace string,
	memories []*ExtractedMemory,
) error {
	if e.store == nil || len(memories) == 0 {
		return nil
	}

	for _, mem := range memories {
		input := &storage.CreateMemoryInput{
			Namespace:  namespace,
			Content:    mem.Content,
			Type:       categoryToMemoryType(mem.Category),
			Confidence: 0.7,
			Source:     storage.MemorySourceExtracted,
			Metadata: map[string]interface{}{
				"extraction_category": mem.Category,
				"extraction_source":   mem.Source,
			},
		}

		_, err := e.store.CreateMemory(ctx, input)
		if err != nil {
			// Log error but continue with other memories
			continue
		}
	}

	return nil
}

// categoryToMemoryType maps extraction categories to memory types.
func categoryToMemoryType(category string) storage.MemoryType {
	switch category {
	case "preference", "instruction":
		return storage.MemoryTypeSemantic
	case "fact", "identity", "user_declaration":
		return storage.MemoryTypeEpisodic
	case "explicit", "important":
		return storage.MemoryTypeSemantic
	default:
		return storage.MemoryTypeSemantic
	}
}

// normalizeContent normalizes content for comparison.
func normalizeContent(content string) string {
	// Lowercase
	normalized := strings.ToLower(content)
	// Remove extra whitespace
	normalized = strings.Join(strings.Fields(normalized), " ")
	// Remove common punctuation for comparison
	normalized = strings.TrimRight(normalized, ".,!?;:")
	return normalized
}
