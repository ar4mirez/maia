package context

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenCounter_Count(t *testing.T) {
	tc := NewTokenCounter()

	tests := []struct {
		name     string
		text     string
		minCount int
		maxCount int
	}{
		{
			name:     "empty string",
			text:     "",
			minCount: 0,
			maxCount: 0,
		},
		{
			name:     "single word",
			text:     "hello",
			minCount: 1,
			maxCount: 2,
		},
		{
			name:     "simple sentence",
			text:     "Hello, world!",
			minCount: 2,
			maxCount: 8,
		},
		{
			name:     "longer text",
			text:     "The quick brown fox jumps over the lazy dog.",
			minCount: 8,
			maxCount: 15,
		},
		{
			name:     "mixed alphanumeric",
			text:     "user123 has ID abc456xyz",
			minCount: 4,
			maxCount: 10,
		},
		{
			name:     "special characters",
			text:     "@user#channel $100 50% off!",
			minCount: 4,
			maxCount: 12,
		},
		{
			name:     "unicode text",
			text:     "こんにちは世界",
			minCount: 1,
			maxCount: 10,
		},
		{
			name:     "code snippet",
			text:     "func main() { fmt.Println(\"hello\") }",
			minCount: 5,
			maxCount: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := tc.Count(tt.text)
			assert.GreaterOrEqual(t, count, tt.minCount, "count should be at least %d", tt.minCount)
			assert.LessOrEqual(t, count, tt.maxCount, "count should be at most %d", tt.maxCount)
		})
	}
}

func TestTokenCounter_CountWord(t *testing.T) {
	tc := NewTokenCounter()

	tests := []struct {
		name     string
		word     string
		expected int
	}{
		{"empty", "", 0},
		{"short", "hi", 1},
		{"medium", "hello", 1},
		{"long", "extraordinary", 4},
		{"number", "12345", 2},
		{"mixed", "test123abc", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := tc.countWord(tt.word)
			// Allow some variance in estimation
			assert.InDelta(t, tt.expected, count, 2, "word '%s' token count", tt.word)
		})
	}
}

func TestTokenCounter_CountBatch(t *testing.T) {
	tc := NewTokenCounter()

	texts := []string{
		"Hello",
		"Hello world",
		"The quick brown fox",
	}

	counts := tc.CountBatch(texts)

	require.Len(t, counts, 3)
	assert.Greater(t, counts[1], counts[0], "more words should mean more tokens")
	assert.Greater(t, counts[2], counts[1], "more words should mean more tokens")
}

func TestTokenCounter_TotalCount(t *testing.T) {
	tc := NewTokenCounter()

	texts := []string{
		"Hello",
		"world",
	}

	total := tc.TotalCount(texts)
	individual := tc.Count(texts[0]) + tc.Count(texts[1])

	assert.Equal(t, individual, total)
}

func TestTokenCounter_EstimateFromLength(t *testing.T) {
	tc := NewTokenCounter()

	tests := []struct {
		length   int
		expected int
	}{
		{0, 0},
		{4, 1},
		{8, 2},
		{100, 25},
		{1000, 250},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			estimate := tc.EstimateFromLength(tt.length)
			assert.Equal(t, tt.expected, estimate)
		})
	}
}

func TestTokenCounter_FitsWithinBudget(t *testing.T) {
	tc := NewTokenCounter()

	tests := []struct {
		name   string
		text   string
		budget int
		fits   bool
	}{
		{
			name:   "empty text always fits",
			text:   "",
			budget: 10,
			fits:   true,
		},
		{
			name:   "small text fits in large budget",
			text:   "Hello world",
			budget: 100,
			fits:   true,
		},
		{
			name:   "large text does not fit in small budget",
			text:   "The quick brown fox jumps over the lazy dog. " +
				"This is a much longer text that should exceed the budget.",
			budget: 5,
			fits:   false,
		},
		{
			name:   "zero budget",
			text:   "hello",
			budget: 0,
			fits:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fits := tc.FitsWithinBudget(tt.text, tt.budget)
			assert.Equal(t, tt.fits, fits)
		})
	}
}

func TestTokenCounter_TruncateToFit(t *testing.T) {
	tc := NewTokenCounter()

	tests := []struct {
		name          string
		text          string
		budget        int
		wantTruncated bool
		wantEmpty     bool
	}{
		{
			name:          "no truncation needed",
			text:          "Hello world",
			budget:        100,
			wantTruncated: false,
			wantEmpty:     false,
		},
		{
			name:          "truncation needed",
			text:          "The quick brown fox jumps over the lazy dog. Pack my box with five dozen liquor jugs.",
			budget:        10,
			wantTruncated: true,
			wantEmpty:     false,
		},
		{
			name:          "zero budget returns empty",
			text:          "Hello world",
			budget:        0,
			wantTruncated: true,
			wantEmpty:     true,
		},
		{
			name:          "negative budget returns empty",
			text:          "Hello world",
			budget:        -5,
			wantTruncated: true,
			wantEmpty:     true,
		},
		{
			name:          "very tight budget",
			text:          "Hello world this is a test",
			budget:        2,
			wantTruncated: true,
			wantEmpty:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, wasTruncated := tc.TruncateToFit(tt.text, tt.budget)

			assert.Equal(t, tt.wantTruncated, wasTruncated, "truncation flag")

			if tt.wantEmpty {
				assert.Empty(t, result)
			} else if wasTruncated {
				// Truncated result should fit in budget
				assert.True(t, tc.FitsWithinBudget(result, tt.budget),
					"truncated result should fit in budget")
				// Truncated result should be shorter
				assert.Less(t, len(result), len(tt.text),
					"truncated result should be shorter")
			} else {
				assert.Equal(t, tt.text, result)
			}
		})
	}
}

func TestTokenCounter_Consistency(t *testing.T) {
	tc := NewTokenCounter()

	// Same text should always produce same count
	text := "The quick brown fox jumps over the lazy dog."

	count1 := tc.Count(text)
	count2 := tc.Count(text)
	count3 := tc.Count(text)

	assert.Equal(t, count1, count2)
	assert.Equal(t, count2, count3)
}

func BenchmarkTokenCounter_Count(b *testing.B) {
	tc := NewTokenCounter()
	text := "The quick brown fox jumps over the lazy dog. " +
		"Pack my box with five dozen liquor jugs. " +
		"How vexingly quick daft zebras jump!"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tc.Count(text)
	}
}

func BenchmarkTokenCounter_TruncateToFit(b *testing.B) {
	tc := NewTokenCounter()
	text := "The quick brown fox jumps over the lazy dog. " +
		"Pack my box with five dozen liquor jugs. " +
		"How vexingly quick daft zebras jump! " +
		"Sphinx of black quartz, judge my vow."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tc.TruncateToFit(text, 15)
	}
}
