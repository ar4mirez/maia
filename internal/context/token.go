// Package context provides position-aware context assembly for MAIA.
package context

import (
	"strings"
	"unicode"
)

// TokenCounter estimates token counts for text.
// Uses a simple heuristic based on common tokenization patterns.
type TokenCounter struct {
	// avgCharsPerToken is the average characters per token.
	// GPT models average ~4 chars/token for English text.
	avgCharsPerToken float64
}

// NewTokenCounter creates a new token counter.
func NewTokenCounter() *TokenCounter {
	return &TokenCounter{
		avgCharsPerToken: 4.0,
	}
}

// Count estimates the token count for the given text.
// Uses a heuristic that accounts for:
// - Words (split on whitespace)
// - Special characters and punctuation
// - Numbers and mixed alphanumeric
func (tc *TokenCounter) Count(text string) int {
	if text == "" {
		return 0
	}

	// Count tokens using multiple heuristics
	tokens := 0

	// Split into words
	words := strings.Fields(text)

	for _, word := range words {
		tokens += tc.countWord(word)
	}

	return tokens
}

// countWord estimates tokens for a single word.
func (tc *TokenCounter) countWord(word string) int {
	if word == "" {
		return 0
	}

	// Short words are typically 1 token
	if len(word) <= 4 {
		return 1
	}

	// Count based on character composition
	tokens := 0
	currentRun := 0
	lastType := runeType(0)

	for _, r := range word {
		rt := runeType(r)

		// Type change often indicates token boundary
		if rt != lastType && lastType != 0 {
			tokens += (currentRun + 3) / 4 // ceil(currentRun / 4)
			currentRun = 0
		}

		currentRun++
		lastType = rt
	}

	// Add remaining characters
	if currentRun > 0 {
		tokens += (currentRun + 3) / 4
	}

	// Ensure at least 1 token per word
	if tokens == 0 {
		tokens = 1
	}

	return tokens
}

// runeType categorizes a rune for token boundary detection.
func runeType(r rune) int {
	switch {
	case unicode.IsLetter(r):
		return 1
	case unicode.IsDigit(r):
		return 2
	case unicode.IsPunct(r):
		return 3
	case unicode.IsSpace(r):
		return 4
	default:
		return 5
	}
}

// CountBatch estimates token counts for multiple texts.
func (tc *TokenCounter) CountBatch(texts []string) []int {
	counts := make([]int, len(texts))
	for i, text := range texts {
		counts[i] = tc.Count(text)
	}
	return counts
}

// TotalCount returns the total token count for multiple texts.
func (tc *TokenCounter) TotalCount(texts []string) int {
	total := 0
	for _, text := range texts {
		total += tc.Count(text)
	}
	return total
}

// EstimateFromLength provides a quick estimate based on text length.
// Less accurate but faster for very long texts.
func (tc *TokenCounter) EstimateFromLength(length int) int {
	return int(float64(length)/tc.avgCharsPerToken + 0.5)
}

// FitsWithinBudget checks if the text fits within the token budget.
func (tc *TokenCounter) FitsWithinBudget(text string, budget int) bool {
	return tc.Count(text) <= budget
}

// TruncateToFit truncates text to fit within the token budget.
// Returns the truncated text and whether truncation occurred.
func (tc *TokenCounter) TruncateToFit(text string, budget int) (string, bool) {
	if budget <= 0 {
		return "", true
	}

	count := tc.Count(text)
	if count <= budget {
		return text, false
	}

	// Binary search for the right length
	words := strings.Fields(text)
	lo, hi := 0, len(words)

	for lo < hi {
		mid := (lo + hi + 1) / 2
		truncated := strings.Join(words[:mid], " ")
		if tc.Count(truncated) <= budget {
			lo = mid
		} else {
			hi = mid - 1
		}
	}

	if lo == 0 {
		return "", true
	}

	return strings.Join(words[:lo], " "), true
}
