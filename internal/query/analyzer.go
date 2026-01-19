// Package query provides query understanding and analysis for MAIA.
package query

import (
	"context"
	"regexp"
	"strings"
	"unicode"
)

// IntentType represents the type of user intent.
type IntentType string

const (
	IntentQuestion     IntentType = "question"     // User is asking a question
	IntentCommand      IntentType = "command"      // User wants to do something
	IntentConversation IntentType = "conversation" // User is having a conversation
	IntentSearch       IntentType = "search"       // User is searching for information
	IntentUnknown      IntentType = "unknown"      // Intent could not be determined
)

// ContextType represents the type of context needed.
type ContextType string

const (
	ContextSemantic ContextType = "semantic" // Facts, knowledge, profiles
	ContextEpisodic ContextType = "episodic" // Past conversations, experiences
	ContextWorking  ContextType = "working"  // Current session state
)

// TemporalScope represents the time scope for retrieval.
type TemporalScope string

const (
	TemporalRecent     TemporalScope = "recent"     // Last few interactions
	TemporalHistorical TemporalScope = "historical" // Past interactions
	TemporalAllTime    TemporalScope = "all_time"   // All available history
)

// Analysis represents the result of query analysis.
type Analysis struct {
	// Original query text
	Query string

	// Detected intent
	Intent IntentType

	// Extracted keywords (ordered by importance)
	Keywords []string

	// Extracted entities (people, places, projects, etc.)
	Entities []Entity

	// Required context types
	ContextTypes []ContextType

	// Temporal scope for retrieval
	TemporalScope TemporalScope

	// Suggested token budget allocation
	TokenBudget TokenBudget

	// Confidence in the analysis (0-1)
	Confidence float64
}

// Entity represents an extracted entity.
type Entity struct {
	Text  string
	Type  EntityType
	Start int
	End   int
}

// EntityType represents the type of entity.
type EntityType string

const (
	EntityPerson   EntityType = "person"
	EntityPlace    EntityType = "place"
	EntityProject  EntityType = "project"
	EntityDate     EntityType = "date"
	EntityConcept  EntityType = "concept"
	EntityUnknown  EntityType = "unknown"
)

// TokenBudget suggests how to allocate tokens.
type TokenBudget struct {
	Semantic int // Tokens for semantic memory
	Episodic int // Tokens for episodic memory
	Working  int // Tokens for working memory
}

// Analyzer analyzes queries to understand intent and extract information.
type Analyzer struct {
	// Stopwords to filter out
	stopwords map[string]bool

	// Question word patterns
	questionWords *regexp.Regexp

	// Command word patterns
	commandWords *regexp.Regexp

	// Entity patterns
	entityPatterns map[EntityType]*regexp.Regexp
}

// NewAnalyzer creates a new query analyzer.
func NewAnalyzer() *Analyzer {
	return &Analyzer{
		stopwords:      buildStopwords(),
		questionWords:  regexp.MustCompile(`(?i)^(what|who|where|when|why|how|which|is|are|was|were|do|does|did|can|could|would|should|will)`),
		commandWords:   regexp.MustCompile(`(?i)^(find|get|show|tell|give|list|search|fetch|retrieve|remember|recall|look)`),
		entityPatterns: buildEntityPatterns(),
	}
}

// Analyze performs analysis on the query.
func (a *Analyzer) Analyze(ctx context.Context, query string) (*Analysis, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return &Analysis{
			Query:         query,
			Intent:        IntentUnknown,
			ContextTypes:  []ContextType{ContextSemantic},
			TemporalScope: TemporalAllTime,
			Confidence:    0,
		}, nil
	}

	analysis := &Analysis{
		Query:        query,
		Keywords:     a.extractKeywords(query),
		Entities:     a.extractEntities(query),
		Confidence:   0.7, // Default confidence for rule-based analysis
	}

	// Detect intent
	analysis.Intent = a.detectIntent(query)

	// Determine context types based on query
	analysis.ContextTypes = a.determineContextTypes(query, analysis.Intent)

	// Determine temporal scope
	analysis.TemporalScope = a.determineTemporalScope(query)

	// Suggest token budget
	analysis.TokenBudget = a.suggestTokenBudget(analysis)

	// Adjust confidence based on analysis quality
	analysis.Confidence = a.calculateConfidence(analysis)

	return analysis, nil
}

// extractKeywords extracts important keywords from the query.
func (a *Analyzer) extractKeywords(query string) []string {
	// Tokenize
	words := tokenize(query)

	// Filter and score
	keywords := make([]string, 0)
	seen := make(map[string]bool)

	for _, word := range words {
		lower := strings.ToLower(word)

		// Skip stopwords
		if a.stopwords[lower] {
			continue
		}

		// Skip very short words
		if len(word) < 2 {
			continue
		}

		// Skip duplicates
		if seen[lower] {
			continue
		}
		seen[lower] = true

		keywords = append(keywords, word)
	}

	return keywords
}

// extractEntities extracts entities from the query.
func (a *Analyzer) extractEntities(query string) []Entity {
	entities := make([]Entity, 0)

	for entityType, pattern := range a.entityPatterns {
		matches := pattern.FindAllStringSubmatchIndex(query, -1)
		for _, match := range matches {
			if len(match) >= 2 {
				entities = append(entities, Entity{
					Text:  query[match[0]:match[1]],
					Type:  entityType,
					Start: match[0],
					End:   match[1],
				})
			}
		}
	}

	return entities
}

// detectIntent detects the user's intent.
func (a *Analyzer) detectIntent(query string) IntentType {
	query = strings.TrimSpace(query)

	// Check for question patterns
	if a.questionWords.MatchString(query) || strings.HasSuffix(query, "?") {
		return IntentQuestion
	}

	// Check for command patterns
	if a.commandWords.MatchString(query) {
		return IntentCommand
	}

	// Check for search-like patterns
	if isSearchLike(query) {
		return IntentSearch
	}

	// Default to conversation
	return IntentConversation
}

// determineContextTypes determines what types of context are needed.
func (a *Analyzer) determineContextTypes(query string, intent IntentType) []ContextType {
	types := make([]ContextType, 0)
	lower := strings.ToLower(query)

	// Always include semantic for factual queries
	if intent == IntentQuestion || intent == IntentSearch {
		types = append(types, ContextSemantic)
	}

	// Include episodic for conversation/history queries
	if strings.Contains(lower, "remember") ||
		strings.Contains(lower, "last time") ||
		strings.Contains(lower, "previous") ||
		strings.Contains(lower, "before") ||
		strings.Contains(lower, "history") ||
		strings.Contains(lower, "conversation") {
		types = append(types, ContextEpisodic)
	}

	// Include working for session-related queries
	if strings.Contains(lower, "current") ||
		strings.Contains(lower, "now") ||
		strings.Contains(lower, "this session") ||
		strings.Contains(lower, "just") {
		types = append(types, ContextWorking)
	}

	// Default to semantic if no types detected
	if len(types) == 0 {
		types = append(types, ContextSemantic)
	}

	return types
}

// determineTemporalScope determines the time scope for retrieval.
func (a *Analyzer) determineTemporalScope(query string) TemporalScope {
	lower := strings.ToLower(query)

	// Recent indicators
	if strings.Contains(lower, "recent") ||
		strings.Contains(lower, "latest") ||
		strings.Contains(lower, "just") ||
		strings.Contains(lower, "today") ||
		strings.Contains(lower, "yesterday") ||
		strings.Contains(lower, "this week") {
		return TemporalRecent
	}

	// Historical indicators
	if strings.Contains(lower, "history") ||
		strings.Contains(lower, "all time") ||
		strings.Contains(lower, "ever") ||
		strings.Contains(lower, "always") {
		return TemporalAllTime
	}

	// Default based on context
	return TemporalHistorical
}

// suggestTokenBudget suggests token allocation.
func (a *Analyzer) suggestTokenBudget(analysis *Analysis) TokenBudget {
	// Default allocation (total 4000)
	budget := TokenBudget{
		Semantic: 2000,
		Episodic: 1500,
		Working:  500,
	}

	// Adjust based on context types
	hasSemantic := false
	hasEpisodic := false
	hasWorking := false

	for _, ct := range analysis.ContextTypes {
		switch ct {
		case ContextSemantic:
			hasSemantic = true
		case ContextEpisodic:
			hasEpisodic = true
		case ContextWorking:
			hasWorking = true
		}
	}

	// Reallocate based on needs
	if hasSemantic && !hasEpisodic && !hasWorking {
		budget.Semantic = 3500
		budget.Episodic = 0
		budget.Working = 500
	} else if hasEpisodic && !hasSemantic && !hasWorking {
		budget.Semantic = 500
		budget.Episodic = 3000
		budget.Working = 500
	} else if hasWorking {
		budget.Working = 1000
		budget.Semantic = 1500
		budget.Episodic = 1500
	}

	return budget
}

// calculateConfidence calculates confidence in the analysis.
func (a *Analyzer) calculateConfidence(analysis *Analysis) float64 {
	confidence := 0.5 // Base confidence

	// More keywords = more confidence
	if len(analysis.Keywords) > 0 {
		confidence += 0.1 * float64(min(len(analysis.Keywords), 5)) / 5
	}

	// Detected entities increase confidence
	if len(analysis.Entities) > 0 {
		confidence += 0.1
	}

	// Clear intent increases confidence
	if analysis.Intent != IntentUnknown {
		confidence += 0.2
	}

	// Cap at 0.95 for rule-based analysis
	if confidence > 0.95 {
		confidence = 0.95
	}

	return confidence
}

// Helper functions

func tokenize(text string) []string {
	var words []string
	var current strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\'' {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
		}
	}

	if current.Len() > 0 {
		words = append(words, current.String())
	}

	return words
}

func isSearchLike(query string) bool {
	// Short queries are likely searches
	words := strings.Fields(query)
	if len(words) <= 3 && !strings.Contains(query, "?") {
		return true
	}
	return false
}

func buildStopwords() map[string]bool {
	words := []string{
		"a", "an", "the", "and", "or", "but", "in", "on", "at", "to", "for",
		"of", "with", "by", "from", "as", "is", "was", "are", "were", "been",
		"be", "have", "has", "had", "do", "does", "did", "will", "would",
		"could", "should", "may", "might", "must", "shall", "can", "need",
		"dare", "ought", "used", "it", "its", "this", "that", "these", "those",
		"i", "you", "he", "she", "we", "they", "me", "him", "her", "us", "them",
		"my", "your", "his", "our", "their", "mine", "yours", "hers", "ours",
		"theirs", "what", "which", "who", "whom", "whose", "where", "when",
		"why", "how", "all", "each", "every", "both", "few", "more", "most",
		"other", "some", "such", "no", "nor", "not", "only", "own", "same",
		"so", "than", "too", "very", "just", "also", "now", "here", "there",
	}

	stopwords := make(map[string]bool)
	for _, w := range words {
		stopwords[w] = true
	}
	return stopwords
}

func buildEntityPatterns() map[EntityType]*regexp.Regexp {
	patterns := make(map[EntityType]*regexp.Regexp)

	// Date patterns (simple)
	patterns[EntityDate] = regexp.MustCompile(`(?i)\b(today|yesterday|tomorrow|last week|next week|monday|tuesday|wednesday|thursday|friday|saturday|sunday|\d{1,2}/\d{1,2}/\d{2,4}|\d{4}-\d{2}-\d{2})\b`)

	// Person patterns (capitalized names - simplified)
	patterns[EntityPerson] = regexp.MustCompile(`\b([A-Z][a-z]+(?:\s+[A-Z][a-z]+)+)\b`)

	return patterns
}
