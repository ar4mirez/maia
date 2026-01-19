package context

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/ar4mirez/maia/internal/retrieval"
	"github.com/ar4mirez/maia/internal/storage"
)

// Position represents a zone in the context window.
type Position int

const (
	// PositionCritical is the first zone (10-15% of budget).
	// Contains highest priority information.
	PositionCritical Position = iota

	// PositionMiddle is the middle zone (60-70% of budget).
	// Contains supporting context with decreasing relevance.
	PositionMiddle

	// PositionRecency is the last zone (15-20% of budget).
	// Contains recent/temporal information.
	PositionRecency
)

// ZoneAllocation defines how tokens are allocated across zones.
type ZoneAllocation struct {
	CriticalPercent float64
	MiddlePercent   float64
	RecencyPercent  float64
}

// DefaultZoneAllocation returns the default zone allocation.
func DefaultZoneAllocation() ZoneAllocation {
	return ZoneAllocation{
		CriticalPercent: 0.15, // 15% for critical
		MiddlePercent:   0.65, // 65% for middle
		RecencyPercent:  0.20, // 20% for recency
	}
}

// Assembler performs position-aware context assembly.
type Assembler struct {
	tokenCounter *TokenCounter
	config       AssemblerConfig
}

// AssemblerConfig holds assembler configuration.
type AssemblerConfig struct {
	// DefaultBudget is the default token budget.
	DefaultBudget int

	// ZoneAllocation defines token distribution across zones.
	ZoneAllocation ZoneAllocation

	// RecencyWindow defines how recent a memory must be for recency zone.
	RecencyWindow time.Duration

	// MinConfidence is the minimum confidence threshold.
	MinConfidence float64

	// IncludeMetadata includes memory metadata in assembled context.
	IncludeMetadata bool

	// Separator between assembled memories.
	Separator string
}

// DefaultAssemblerConfig returns the default configuration.
func DefaultAssemblerConfig() AssemblerConfig {
	return AssemblerConfig{
		DefaultBudget:   4000,
		ZoneAllocation:  DefaultZoneAllocation(),
		RecencyWindow:   24 * time.Hour,
		MinConfidence:   0.0,
		IncludeMetadata: false,
		Separator:       "\n\n---\n\n",
	}
}

// NewAssembler creates a new context assembler.
func NewAssembler(config AssemblerConfig) *Assembler {
	return &Assembler{
		tokenCounter: NewTokenCounter(),
		config:       config,
	}
}

// AssembleOptions configures context assembly.
type AssembleOptions struct {
	// TokenBudget is the maximum tokens for assembled context.
	TokenBudget int

	// ZoneAllocation overrides default zone allocation.
	ZoneAllocation *ZoneAllocation

	// SystemPrompt to prepend (counts against budget).
	SystemPrompt string

	// IncludeScores adds relevance scores to output.
	IncludeScores bool
}

// AssembledContext is the result of context assembly.
type AssembledContext struct {
	// Content is the assembled context string.
	Content string

	// Memories is the list of included memories in order.
	Memories []*IncludedMemory

	// TokenCount is the total token count.
	TokenCount int

	// TokenBudget is the budget that was used.
	TokenBudget int

	// Truncated indicates if memories were truncated.
	Truncated bool

	// ZoneStats shows how tokens were distributed.
	ZoneStats ZoneStats

	// AssemblyTime is how long assembly took.
	AssemblyTime time.Duration
}

// IncludedMemory represents a memory included in the context.
type IncludedMemory struct {
	Memory     *storage.Memory
	Position   Position
	Score      float64
	TokenCount int
	Truncated  bool
}

// ZoneStats tracks token usage per zone.
type ZoneStats struct {
	CriticalUsed   int
	CriticalBudget int
	MiddleUsed     int
	MiddleBudget   int
	RecencyUsed    int
	RecencyBudget  int
}

// Assemble creates a position-aware context from retrieval results.
func (a *Assembler) Assemble(
	ctx context.Context,
	results *retrieval.Results,
	opts *AssembleOptions,
) (*AssembledContext, error) {
	startTime := time.Now()

	if opts == nil {
		opts = &AssembleOptions{}
	}

	budget := opts.TokenBudget
	if budget <= 0 {
		budget = a.config.DefaultBudget
	}

	zones := a.config.ZoneAllocation
	if opts.ZoneAllocation != nil {
		zones = *opts.ZoneAllocation
	}

	// Reserve tokens for system prompt if provided
	systemTokens := 0
	if opts.SystemPrompt != "" {
		systemTokens = a.tokenCounter.Count(opts.SystemPrompt)
		budget -= systemTokens
	}

	// Calculate zone budgets
	criticalBudget := int(float64(budget) * zones.CriticalPercent)
	middleBudget := int(float64(budget) * zones.MiddlePercent)
	recencyBudget := int(float64(budget) * zones.RecencyPercent)

	// Categorize memories into zones
	critical, middle, recency := a.categorizeMemories(results.Items)

	// Fill each zone
	var included []*IncludedMemory
	zoneStats := ZoneStats{
		CriticalBudget: criticalBudget,
		MiddleBudget:   middleBudget,
		RecencyBudget:  recencyBudget,
	}

	// Fill critical zone (highest scores)
	criticalIncluded, criticalUsed := a.fillZone(critical, criticalBudget, PositionCritical)
	included = append(included, criticalIncluded...)
	zoneStats.CriticalUsed = criticalUsed

	// Fill middle zone (supporting context)
	middleIncluded, middleUsed := a.fillZone(middle, middleBudget, PositionMiddle)
	included = append(included, middleIncluded...)
	zoneStats.MiddleUsed = middleUsed

	// Fill recency zone (recent memories)
	recencyIncluded, recencyUsed := a.fillZone(recency, recencyBudget, PositionRecency)
	included = append(included, recencyIncluded...)
	zoneStats.RecencyUsed = recencyUsed

	// Build final content
	content := a.buildContent(opts.SystemPrompt, included, opts.IncludeScores)
	totalTokens := a.tokenCounter.Count(content)

	// Check if truncated
	truncated := len(included) < len(results.Items)

	return &AssembledContext{
		Content:      content,
		Memories:     included,
		TokenCount:   totalTokens,
		TokenBudget:  opts.TokenBudget,
		Truncated:    truncated,
		ZoneStats:    zoneStats,
		AssemblyTime: time.Since(startTime),
	}, nil
}

// categorizeMemories splits memories into zones based on characteristics.
func (a *Assembler) categorizeMemories(
	results []*retrieval.Result,
) (critical, middle, recency []*retrieval.Result) {
	now := time.Now()

	for _, r := range results {
		// Skip low confidence memories
		if r.Memory.Confidence < a.config.MinConfidence {
			continue
		}

		// Check if memory is recent (within recency window)
		isRecent := now.Sub(r.Memory.CreatedAt) < a.config.RecencyWindow ||
			now.Sub(r.Memory.AccessedAt) < a.config.RecencyWindow

		// Working memories always go to recency zone
		if r.Memory.Type == storage.MemoryTypeWorking {
			recency = append(recency, r)
			continue
		}

		// High-score memories go to critical zone
		if r.Score >= 0.7 {
			critical = append(critical, r)
			continue
		}

		// Recent memories go to recency zone
		if isRecent && r.Score >= 0.3 {
			recency = append(recency, r)
			continue
		}

		// Everything else goes to middle zone
		middle = append(middle, r)
	}

	return critical, middle, recency
}

// fillZone fills a zone with memories up to the budget.
func (a *Assembler) fillZone(
	results []*retrieval.Result,
	budget int,
	position Position,
) ([]*IncludedMemory, int) {
	if budget <= 0 || len(results) == 0 {
		return nil, 0
	}

	var included []*IncludedMemory
	usedTokens := 0

	// Sort by score descending
	sorted := make([]*retrieval.Result, len(results))
	copy(sorted, results)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Score > sorted[j].Score
	})

	for _, r := range sorted {
		content := r.Memory.Content
		tokens := a.tokenCounter.Count(content)

		// Check if memory fits
		if usedTokens+tokens <= budget {
			included = append(included, &IncludedMemory{
				Memory:     r.Memory,
				Position:   position,
				Score:      r.Score,
				TokenCount: tokens,
				Truncated:  false,
			})
			usedTokens += tokens
			continue
		}

		// Try to truncate to fit remaining budget
		remaining := budget - usedTokens
		if remaining > 50 { // Only truncate if meaningful space left
			truncated, wasTruncated := a.tokenCounter.TruncateToFit(content, remaining)
			if wasTruncated && len(truncated) > 0 {
				truncTokens := a.tokenCounter.Count(truncated)
				mem := *r.Memory // Copy memory
				mem.Content = truncated
				included = append(included, &IncludedMemory{
					Memory:     &mem,
					Position:   position,
					Score:      r.Score,
					TokenCount: truncTokens,
					Truncated:  true,
				})
				usedTokens += truncTokens
			}
		}
		break // No more room
	}

	return included, usedTokens
}

// buildContent assembles the final context string.
func (a *Assembler) buildContent(
	systemPrompt string,
	included []*IncludedMemory,
	includeScores bool,
) string {
	var builder strings.Builder

	// Add system prompt if provided
	if systemPrompt != "" {
		builder.WriteString(systemPrompt)
		builder.WriteString(a.config.Separator)
	}

	// Group by position for ordered output
	var critical, middle, recency []*IncludedMemory
	for _, m := range included {
		switch m.Position {
		case PositionCritical:
			critical = append(critical, m)
		case PositionMiddle:
			middle = append(middle, m)
		case PositionRecency:
			recency = append(recency, m)
		}
	}

	// Build in order: critical -> middle -> recency
	allGroups := [][]*IncludedMemory{critical, middle, recency}
	first := true

	for _, group := range allGroups {
		for _, m := range group {
			if !first {
				builder.WriteString(a.config.Separator)
			}
			first = false

			if includeScores {
				builder.WriteString("[relevance: ")
				builder.WriteString(formatScore(m.Score))
				builder.WriteString("] ")
			}

			builder.WriteString(m.Memory.Content)

			if m.Truncated {
				builder.WriteString(" [truncated]")
			}
		}
	}

	return builder.String()
}

// formatScore formats a score for display.
func formatScore(score float64) string {
	// Format as percentage
	pct := int(score * 100)
	if pct >= 100 {
		return "100%"
	}
	if pct < 10 {
		return string(rune('0'+pct)) + "%"
	}
	return string(rune('0'+pct/10)) + string(rune('0'+pct%10)) + "%"
}

// AssembleSimple provides a simpler interface for basic use cases.
func (a *Assembler) AssembleSimple(
	ctx context.Context,
	memories []*storage.Memory,
	budget int,
) (string, int, error) {
	// Convert memories to retrieval results
	results := make([]*retrieval.Result, len(memories))
	for i, m := range memories {
		results[i] = &retrieval.Result{
			Memory: m,
			Score:  1.0 - float64(i)*0.1, // Decreasing score by order
		}
	}

	assembled, err := a.Assemble(ctx, &retrieval.Results{
		Items: results,
		Total: len(results),
	}, &AssembleOptions{
		TokenBudget: budget,
	})
	if err != nil {
		return "", 0, err
	}

	return assembled.Content, assembled.TokenCount, nil
}
