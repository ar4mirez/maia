package query

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyzer_Analyze(t *testing.T) {
	analyzer := NewAnalyzer()
	ctx := context.Background()

	t.Run("handles empty query", func(t *testing.T) {
		analysis, err := analyzer.Analyze(ctx, "")
		require.NoError(t, err)
		assert.Equal(t, IntentUnknown, analysis.Intent)
		assert.Equal(t, float64(0), analysis.Confidence)
	})

	t.Run("detects question intent", func(t *testing.T) {
		queries := []string{
			"What is the capital of France?",
			"Who wrote this code?",
			"Where are the config files?",
			"When was the last deployment?",
			"Why did the test fail?",
			"How do I configure logging?",
			"Is this the correct approach?",
		}

		for _, query := range queries {
			analysis, err := analyzer.Analyze(ctx, query)
			require.NoError(t, err)
			assert.Equal(t, IntentQuestion, analysis.Intent, "Query: %s", query)
		}
	})

	t.Run("detects command intent", func(t *testing.T) {
		queries := []string{
			"Find all errors in the log",
			"Get the user preferences",
			"Show me the recent conversations",
			"List all namespaces",
			"Search for memory related issues",
			"Remember this for later",
		}

		for _, query := range queries {
			analysis, err := analyzer.Analyze(ctx, query)
			require.NoError(t, err)
			assert.Equal(t, IntentCommand, analysis.Intent, "Query: %s", query)
		}
	})

	t.Run("detects search intent for short queries", func(t *testing.T) {
		queries := []string{
			"user preferences",
			"error handling",
			"database connection",
		}

		for _, query := range queries {
			analysis, err := analyzer.Analyze(ctx, query)
			require.NoError(t, err)
			assert.Equal(t, IntentSearch, analysis.Intent, "Query: %s", query)
		}
	})

	t.Run("extracts keywords", func(t *testing.T) {
		analysis, err := analyzer.Analyze(ctx, "How do I configure the database connection for production?")
		require.NoError(t, err)

		// Should contain meaningful keywords
		assert.Contains(t, analysis.Keywords, "configure")
		assert.Contains(t, analysis.Keywords, "database")
		assert.Contains(t, analysis.Keywords, "connection")
		assert.Contains(t, analysis.Keywords, "production")

		// Should not contain stopwords
		for _, kw := range analysis.Keywords {
			assert.NotEqual(t, "the", kw)
			assert.NotEqual(t, "for", kw)
			assert.NotEqual(t, "do", kw)
		}
	})

	t.Run("extracts date entities", func(t *testing.T) {
		queries := []string{
			"What happened yesterday?",
			"Show me events from last week",
			"Schedule meeting for tomorrow",
		}

		for _, query := range queries {
			analysis, err := analyzer.Analyze(ctx, query)
			require.NoError(t, err)

			hasDateEntity := false
			for _, entity := range analysis.Entities {
				if entity.Type == EntityDate {
					hasDateEntity = true
					break
				}
			}
			assert.True(t, hasDateEntity, "Query: %s", query)
		}
	})

	t.Run("determines context types for questions", func(t *testing.T) {
		analysis, err := analyzer.Analyze(ctx, "What is the API rate limit?")
		require.NoError(t, err)
		assert.Contains(t, analysis.ContextTypes, ContextSemantic)
	})

	t.Run("includes episodic context for history queries", func(t *testing.T) {
		queries := []string{
			"What did we discuss last time?",
			"Remember when we talked about this?",
			"Show me the conversation history",
			"What happened in previous sessions?",
		}

		for _, query := range queries {
			analysis, err := analyzer.Analyze(ctx, query)
			require.NoError(t, err)
			assert.Contains(t, analysis.ContextTypes, ContextEpisodic, "Query: %s", query)
		}
	})

	t.Run("includes working context for session queries", func(t *testing.T) {
		queries := []string{
			"What are we currently working on?",
			"Show me the current task",
			"What just happened?",
		}

		for _, query := range queries {
			analysis, err := analyzer.Analyze(ctx, query)
			require.NoError(t, err)
			assert.Contains(t, analysis.ContextTypes, ContextWorking, "Query: %s", query)
		}
	})

	t.Run("determines temporal scope", func(t *testing.T) {
		recentQueries := []string{
			"Show me recent changes",
			"What happened today?",
			"Latest updates please",
		}

		for _, query := range recentQueries {
			analysis, err := analyzer.Analyze(ctx, query)
			require.NoError(t, err)
			assert.Equal(t, TemporalRecent, analysis.TemporalScope, "Query: %s", query)
		}

		allTimeQueries := []string{
			"Show me all history",
			"Have we ever discussed this?",
		}

		for _, query := range allTimeQueries {
			analysis, err := analyzer.Analyze(ctx, query)
			require.NoError(t, err)
			assert.Equal(t, TemporalAllTime, analysis.TemporalScope, "Query: %s", query)
		}
	})

	t.Run("suggests token budget", func(t *testing.T) {
		analysis, err := analyzer.Analyze(ctx, "What are the user preferences?")
		require.NoError(t, err)

		// Token budget should be non-zero
		total := analysis.TokenBudget.Semantic + analysis.TokenBudget.Episodic + analysis.TokenBudget.Working
		assert.Greater(t, total, 0)

		// Semantic query should favor semantic memory
		assert.Greater(t, analysis.TokenBudget.Semantic, analysis.TokenBudget.Episodic)
	})

	t.Run("calculates confidence", func(t *testing.T) {
		// Clear query should have higher confidence
		clearAnalysis, err := analyzer.Analyze(ctx, "What is the database connection string?")
		require.NoError(t, err)

		// Ambiguous query
		ambiguousAnalysis, err := analyzer.Analyze(ctx, "stuff")
		require.NoError(t, err)

		assert.Greater(t, clearAnalysis.Confidence, ambiguousAnalysis.Confidence)
		assert.LessOrEqual(t, clearAnalysis.Confidence, 1.0)
		assert.GreaterOrEqual(t, ambiguousAnalysis.Confidence, 0.0)
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := analyzer.Analyze(ctx, "test query")
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{
			input:    "Hello, world!",
			expected: []string{"Hello", "world"},
		},
		{
			input:    "user's preference",
			expected: []string{"user's", "preference"},
		},
		{
			input:    "test123 foo456",
			expected: []string{"test123", "foo456"},
		},
		{
			input:    "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := tokenize(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsSearchLike(t *testing.T) {
	tests := []struct {
		query    string
		expected bool
	}{
		{"user preferences", true},
		{"database config", true},
		{"a b c", true},
		{"What is the database configuration?", false},
		{"single", true},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			result := isSearchLike(tt.query)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func BenchmarkAnalyzer_Analyze(b *testing.B) {
	analyzer := NewAnalyzer()
	ctx := context.Background()
	query := "How do I configure the database connection for production deployment?"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = analyzer.Analyze(ctx, query)
	}
}
