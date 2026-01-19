package retrieval

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestScorer_RecencyScore(t *testing.T) {
	scorer := NewScorer(DefaultConfig())

	t.Run("zero time returns default score", func(t *testing.T) {
		score := scorer.RecencyScore(time.Time{})
		assert.Equal(t, 0.5, score)
	})

	t.Run("recent access has high score", func(t *testing.T) {
		recentTime := time.Now().Add(-1 * time.Hour)
		score := scorer.RecencyScore(recentTime)
		assert.Greater(t, score, 0.9)
	})

	t.Run("week-old access has medium score", func(t *testing.T) {
		weekAgo := time.Now().Add(-7 * 24 * time.Hour)
		score := scorer.RecencyScore(weekAgo)
		assert.InDelta(t, 0.5, score, 0.1) // Half-life is 7 days
	})

	t.Run("month-old access has low score", func(t *testing.T) {
		monthAgo := time.Now().Add(-30 * 24 * time.Hour)
		score := scorer.RecencyScore(monthAgo)
		assert.Less(t, score, 0.2)
	})

	t.Run("score decreases with age", func(t *testing.T) {
		recent := scorer.RecencyScore(time.Now().Add(-1 * time.Hour))
		older := scorer.RecencyScore(time.Now().Add(-24 * time.Hour))
		oldest := scorer.RecencyScore(time.Now().Add(-7 * 24 * time.Hour))

		assert.Greater(t, recent, older)
		assert.Greater(t, older, oldest)
	})
}

func TestScorer_FrequencyScore(t *testing.T) {
	scorer := NewScorer(DefaultConfig())

	t.Run("zero access returns zero", func(t *testing.T) {
		score := scorer.FrequencyScore(0)
		assert.Equal(t, 0.0, score)
	})

	t.Run("negative access returns zero", func(t *testing.T) {
		score := scorer.FrequencyScore(-5)
		assert.Equal(t, 0.0, score)
	})

	t.Run("low access has low score", func(t *testing.T) {
		score := scorer.FrequencyScore(1)
		assert.Less(t, score, 0.2)
	})

	t.Run("high access has high score", func(t *testing.T) {
		score := scorer.FrequencyScore(500)
		assert.Greater(t, score, 0.8)
	})

	t.Run("score increases with count", func(t *testing.T) {
		low := scorer.FrequencyScore(1)
		medium := scorer.FrequencyScore(50)
		high := scorer.FrequencyScore(500)

		assert.Less(t, low, medium)
		assert.Less(t, medium, high)
	})

	t.Run("score caps at 1.0", func(t *testing.T) {
		score := scorer.FrequencyScore(10000000)
		assert.LessOrEqual(t, score, 1.0)
	})
}

func TestScorer_CombinedScore(t *testing.T) {
	scorer := NewScorer(DefaultConfig())

	t.Run("combines scores with weights", func(t *testing.T) {
		score := scorer.CombinedScore(1.0, 1.0, 1.0, 100)
		assert.Greater(t, score, 0.9)
	})

	t.Run("high vector score dominates", func(t *testing.T) {
		highVector := scorer.CombinedScore(1.0, 0.0, 0.0, 0)

		// Vector has weight 0.4
		assert.Greater(t, highVector, 0.3)
		assert.Less(t, highVector, 0.5)
	})

	t.Run("score is bounded 0-1", func(t *testing.T) {
		score := scorer.CombinedScore(1.5, 1.5, 1.5, 10000)
		assert.LessOrEqual(t, score, 1.0)

		score = scorer.CombinedScore(-1.0, -1.0, -1.0, 0)
		assert.GreaterOrEqual(t, score, 0.0)
	})
}

func TestScorer_VectorSimilarityScore(t *testing.T) {
	scorer := NewScorer(DefaultConfig())

	t.Run("maps cosine similarity to 0-1", func(t *testing.T) {
		// Cosine similarity of 1 should map to 1
		score := scorer.VectorSimilarityScore(1.0)
		assert.Equal(t, 1.0, score)

		// Cosine similarity of -1 should map to 0
		score = scorer.VectorSimilarityScore(-1.0)
		assert.Equal(t, 0.0, score)

		// Cosine similarity of 0 should map to 0.5
		score = scorer.VectorSimilarityScore(0.0)
		assert.Equal(t, 0.5, score)
	})
}

func TestScorer_TextMatchScore(t *testing.T) {
	scorer := NewScorer(DefaultConfig())

	t.Run("normalizes score by max", func(t *testing.T) {
		score := scorer.TextMatchScore(5.0, 10.0)
		assert.Equal(t, 0.5, score)
	})

	t.Run("handles zero max score", func(t *testing.T) {
		score := scorer.TextMatchScore(5.0, 0.0)
		assert.Equal(t, 0.0, score)
	})

	t.Run("caps at 1.0", func(t *testing.T) {
		score := scorer.TextMatchScore(15.0, 10.0)
		assert.Equal(t, 1.0, score)
	})
}

func TestScorer_BoostScore(t *testing.T) {
	scorer := NewScorer(DefaultConfig())

	t.Run("applies boost factor", func(t *testing.T) {
		score := scorer.BoostScore(0.5, 1.5)
		assert.Equal(t, 0.75, score)
	})

	t.Run("caps at 1.0", func(t *testing.T) {
		score := scorer.BoostScore(0.8, 2.0)
		assert.Equal(t, 1.0, score)
	})
}

func TestScorer_DecayScore(t *testing.T) {
	scorer := NewScorer(DefaultConfig())

	t.Run("applies time decay", func(t *testing.T) {
		halfLife := 7 * 24 * time.Hour

		// At half-life, score should be halved
		score := scorer.DecayScore(1.0, halfLife, halfLife)
		assert.InDelta(t, 0.5, score, 0.01)

		// At 2x half-life, score should be quartered
		score = scorer.DecayScore(1.0, 2*halfLife, halfLife)
		assert.InDelta(t, 0.25, score, 0.01)
	})

	t.Run("handles zero half-life", func(t *testing.T) {
		score := scorer.DecayScore(0.8, time.Hour, 0)
		assert.Equal(t, 0.8, score)
	})
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, 10, cfg.DefaultLimit)
	assert.Equal(t, 0.4, cfg.VectorWeight)
	assert.Equal(t, 0.25, cfg.TextWeight)
	assert.Equal(t, 0.25, cfg.RecencyWeight)
	assert.Equal(t, 0.1, cfg.FrequencyWeight)

	// Weights should sum to 1.0
	totalWeight := cfg.VectorWeight + cfg.TextWeight + cfg.RecencyWeight + cfg.FrequencyWeight
	assert.Equal(t, 1.0, totalWeight)
}
