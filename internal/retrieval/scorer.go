package retrieval

import (
	"math"
	"time"
)

// Scorer calculates relevance scores for memories.
type Scorer struct {
	config Config
}

// NewScorer creates a new scorer.
func NewScorer(config Config) *Scorer {
	return &Scorer{config: config}
}

// CombinedScore calculates the final combined score (without graph).
func (s *Scorer) CombinedScore(vectorScore, textScore, recencyScore, accessCount float64) float64 {
	return s.CombinedScoreWithGraph(vectorScore, textScore, recencyScore, accessCount, 0)
}

// CombinedScoreWithGraph calculates the final combined score including graph connectivity.
func (s *Scorer) CombinedScoreWithGraph(vectorScore, textScore, recencyScore, accessCount, graphScore float64) float64 {
	// Normalize access count to 0-1 range using log scale
	frequencyScore := s.FrequencyScore(accessCount)

	score := s.config.VectorWeight*vectorScore +
		s.config.TextWeight*textScore +
		s.config.RecencyWeight*recencyScore +
		s.config.FrequencyWeight*frequencyScore +
		s.config.GraphWeight*graphScore

	// Ensure score is in 0-1 range
	if score > 1.0 {
		score = 1.0
	}
	if score < 0.0 {
		score = 0.0
	}

	return score
}

// RecencyScore calculates a recency score based on last access time.
// More recent accesses get higher scores.
func (s *Scorer) RecencyScore(accessedAt time.Time) float64 {
	if accessedAt.IsZero() {
		return 0.5 // Default score for never-accessed items
	}

	age := time.Since(accessedAt)

	// Use exponential decay
	// Half-life of 7 days
	halfLife := 7 * 24 * time.Hour
	decay := math.Pow(0.5, float64(age)/float64(halfLife))

	return decay
}

// FrequencyScore calculates a frequency score based on access count.
// Uses log scale to prevent very high access counts from dominating.
func (s *Scorer) FrequencyScore(accessCount float64) float64 {
	if accessCount <= 0 {
		return 0
	}

	// Log scale normalization
	// log(1 + count) / log(1 + maxExpectedCount)
	// Assume max expected count around 1000
	maxExpected := 1000.0
	score := math.Log1p(accessCount) / math.Log1p(maxExpected)

	if score > 1.0 {
		score = 1.0
	}

	return score
}

// VectorSimilarityScore normalizes a vector similarity score.
// Cosine similarity is already in [-1, 1], map to [0, 1].
func (s *Scorer) VectorSimilarityScore(cosineSimilarity float64) float64 {
	// Map [-1, 1] to [0, 1]
	return (cosineSimilarity + 1) / 2
}

// TextMatchScore normalizes a text match score.
func (s *Scorer) TextMatchScore(rawScore, maxScore float64) float64 {
	if maxScore <= 0 {
		return 0
	}
	score := rawScore / maxScore
	if score > 1.0 {
		score = 1.0
	}
	return score
}

// BoostScore applies a boost factor to a score.
func (s *Scorer) BoostScore(score, boost float64) float64 {
	boosted := score * boost
	if boosted > 1.0 {
		return 1.0
	}
	return boosted
}

// DecayScore applies time-based decay to a score.
func (s *Scorer) DecayScore(score float64, age time.Duration, halfLife time.Duration) float64 {
	if halfLife <= 0 {
		return score
	}
	decay := math.Pow(0.5, float64(age)/float64(halfLife))
	return score * decay
}
