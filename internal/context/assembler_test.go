package context

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ar4mirez/maia/internal/retrieval"
	"github.com/ar4mirez/maia/internal/storage"
)

func TestDefaultZoneAllocation(t *testing.T) {
	za := DefaultZoneAllocation()

	// Verify allocations sum to ~1.0
	total := za.CriticalPercent + za.MiddlePercent + za.RecencyPercent
	assert.InDelta(t, 1.0, total, 0.01, "zone allocations should sum to 1.0")

	// Verify each zone has reasonable allocation
	assert.Greater(t, za.CriticalPercent, 0.0)
	assert.Greater(t, za.MiddlePercent, 0.0)
	assert.Greater(t, za.RecencyPercent, 0.0)

	// Middle should be largest
	assert.Greater(t, za.MiddlePercent, za.CriticalPercent)
	assert.Greater(t, za.MiddlePercent, za.RecencyPercent)
}

func TestDefaultAssemblerConfig(t *testing.T) {
	cfg := DefaultAssemblerConfig()

	assert.Greater(t, cfg.DefaultBudget, 0)
	assert.NotEmpty(t, cfg.Separator)
	assert.Greater(t, cfg.RecencyWindow, time.Duration(0))
}

func TestNewAssembler(t *testing.T) {
	cfg := DefaultAssemblerConfig()
	a := NewAssembler(cfg)

	assert.NotNil(t, a)
	assert.NotNil(t, a.tokenCounter)
	assert.Equal(t, cfg, a.config)
}

func TestAssembler_Assemble(t *testing.T) {
	a := NewAssembler(DefaultAssemblerConfig())
	ctx := context.Background()

	tests := []struct {
		name    string
		results *retrieval.Results
		opts    *AssembleOptions
		check   func(t *testing.T, ac *AssembledContext)
	}{
		{
			name:    "empty results",
			results: &retrieval.Results{Items: nil, Total: 0},
			opts:    nil,
			check: func(t *testing.T, ac *AssembledContext) {
				assert.Empty(t, ac.Content)
				assert.Empty(t, ac.Memories)
				assert.Equal(t, 0, ac.TokenCount)
			},
		},
		{
			name: "single high-score memory goes to critical zone",
			results: &retrieval.Results{
				Items: []*retrieval.Result{
					{
						Memory: &storage.Memory{
							ID:        "1",
							Content:   "Important fact",
							Type:      storage.MemoryTypeSemantic,
							CreatedAt: time.Now().Add(-48 * time.Hour),
						},
						Score: 0.9,
					},
				},
				Total: 1,
			},
			opts: &AssembleOptions{TokenBudget: 1000},
			check: func(t *testing.T, ac *AssembledContext) {
				require.Len(t, ac.Memories, 1)
				assert.Equal(t, PositionCritical, ac.Memories[0].Position)
				assert.Contains(t, ac.Content, "Important fact")
			},
		},
		{
			name: "working memory goes to recency zone",
			results: &retrieval.Results{
				Items: []*retrieval.Result{
					{
						Memory: &storage.Memory{
							ID:        "1",
							Content:   "Current session data",
							Type:      storage.MemoryTypeWorking,
							CreatedAt: time.Now(),
						},
						Score: 0.5,
					},
				},
				Total: 1,
			},
			opts: &AssembleOptions{TokenBudget: 1000},
			check: func(t *testing.T, ac *AssembledContext) {
				require.Len(t, ac.Memories, 1)
				assert.Equal(t, PositionRecency, ac.Memories[0].Position)
			},
		},
		{
			name: "recent memory goes to recency zone",
			results: &retrieval.Results{
				Items: []*retrieval.Result{
					{
						Memory: &storage.Memory{
							ID:         "1",
							Content:    "Recent info",
							Type:       storage.MemoryTypeSemantic,
							CreatedAt:  time.Now().Add(-1 * time.Hour),
							AccessedAt: time.Now().Add(-30 * time.Minute),
						},
						Score: 0.5,
					},
				},
				Total: 1,
			},
			opts: &AssembleOptions{TokenBudget: 1000},
			check: func(t *testing.T, ac *AssembledContext) {
				require.Len(t, ac.Memories, 1)
				assert.Equal(t, PositionRecency, ac.Memories[0].Position)
			},
		},
		{
			name: "medium score old memory goes to middle zone",
			results: &retrieval.Results{
				Items: []*retrieval.Result{
					{
						Memory: &storage.Memory{
							ID:         "1",
							Content:    "Supporting context",
							Type:       storage.MemoryTypeSemantic,
							CreatedAt:  time.Now().Add(-72 * time.Hour),
							AccessedAt: time.Now().Add(-48 * time.Hour),
						},
						Score: 0.5,
					},
				},
				Total: 1,
			},
			opts: &AssembleOptions{TokenBudget: 1000},
			check: func(t *testing.T, ac *AssembledContext) {
				require.Len(t, ac.Memories, 1)
				assert.Equal(t, PositionMiddle, ac.Memories[0].Position)
			},
		},
		{
			name: "system prompt included",
			results: &retrieval.Results{
				Items: []*retrieval.Result{
					{
						Memory: &storage.Memory{
							ID:        "1",
							Content:   "Memory content",
							Type:      storage.MemoryTypeSemantic,
							CreatedAt: time.Now().Add(-48 * time.Hour),
						},
						Score: 0.8,
					},
				},
				Total: 1,
			},
			opts: &AssembleOptions{
				TokenBudget:  1000,
				SystemPrompt: "You are a helpful assistant.",
			},
			check: func(t *testing.T, ac *AssembledContext) {
				assert.Contains(t, ac.Content, "You are a helpful assistant.")
				assert.Contains(t, ac.Content, "Memory content")
			},
		},
		{
			name: "include scores option",
			results: &retrieval.Results{
				Items: []*retrieval.Result{
					{
						Memory: &storage.Memory{
							ID:        "1",
							Content:   "Test content",
							Type:      storage.MemoryTypeSemantic,
							CreatedAt: time.Now().Add(-48 * time.Hour),
						},
						Score: 0.85,
					},
				},
				Total: 1,
			},
			opts: &AssembleOptions{
				TokenBudget:   1000,
				IncludeScores: true,
			},
			check: func(t *testing.T, ac *AssembledContext) {
				assert.Contains(t, ac.Content, "[relevance:")
			},
		},
		{
			name: "respects token budget",
			results: &retrieval.Results{
				Items: []*retrieval.Result{
					{
						Memory: &storage.Memory{
							ID:        "1",
							Content:   "First memory with some content that takes up tokens",
							Type:      storage.MemoryTypeSemantic,
							CreatedAt: time.Now().Add(-48 * time.Hour),
						},
						Score: 0.9,
					},
					{
						Memory: &storage.Memory{
							ID:        "2",
							Content:   "Second memory with more content",
							Type:      storage.MemoryTypeSemantic,
							CreatedAt: time.Now().Add(-48 * time.Hour),
						},
						Score: 0.8,
					},
					{
						Memory: &storage.Memory{
							ID:        "3",
							Content:   "Third memory with even more content to fill budget",
							Type:      storage.MemoryTypeSemantic,
							CreatedAt: time.Now().Add(-48 * time.Hour),
						},
						Score: 0.7,
					},
				},
				Total: 3,
			},
			opts: &AssembleOptions{TokenBudget: 200},
			check: func(t *testing.T, ac *AssembledContext) {
				// Should include at least some memories
				assert.NotEmpty(t, ac.Memories)
				// Token count should be reasonable
				assert.Greater(t, ac.TokenCount, 0)
			},
		},
		{
			name: "multiple zones filled correctly",
			results: &retrieval.Results{
				Items: []*retrieval.Result{
					{
						Memory: &storage.Memory{
							ID:         "critical",
							Content:    "Critical information",
							Type:       storage.MemoryTypeSemantic,
							CreatedAt:  time.Now().Add(-72 * time.Hour),
							AccessedAt: time.Now().Add(-72 * time.Hour),
						},
						Score: 0.9,
					},
					{
						Memory: &storage.Memory{
							ID:         "middle",
							Content:    "Middle zone content",
							Type:       storage.MemoryTypeSemantic,
							CreatedAt:  time.Now().Add(-72 * time.Hour),
							AccessedAt: time.Now().Add(-72 * time.Hour),
						},
						Score: 0.5,
					},
					{
						Memory: &storage.Memory{
							ID:        "working",
							Content:   "Working memory",
							Type:      storage.MemoryTypeWorking,
							CreatedAt: time.Now(),
						},
						Score: 0.4,
					},
				},
				Total: 3,
			},
			opts: &AssembleOptions{TokenBudget: 1000},
			check: func(t *testing.T, ac *AssembledContext) {
				require.Len(t, ac.Memories, 3)

				// Find memories by ID and check positions
				positions := make(map[string]Position)
				for _, m := range ac.Memories {
					positions[m.Memory.ID] = m.Position
				}

				assert.Equal(t, PositionCritical, positions["critical"])
				assert.Equal(t, PositionMiddle, positions["middle"])
				assert.Equal(t, PositionRecency, positions["working"])

				// Content should be in order: critical, middle, recency
				criticalIdx := indexOfSubstring(ac.Content, "Critical information")
				middleIdx := indexOfSubstring(ac.Content, "Middle zone content")
				workingIdx := indexOfSubstring(ac.Content, "Working memory")

				assert.Less(t, criticalIdx, middleIdx, "critical should come before middle")
				assert.Less(t, middleIdx, workingIdx, "middle should come before recency")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := a.Assemble(ctx, tt.results, tt.opts)
			require.NoError(t, err)
			require.NotNil(t, result)

			// Basic checks
			assert.GreaterOrEqual(t, result.TokenCount, 0)
			assert.Greater(t, result.AssemblyTime, time.Duration(0))

			// Custom checks
			tt.check(t, result)
		})
	}
}

func TestAssembler_Assemble_ZoneStats(t *testing.T) {
	a := NewAssembler(DefaultAssemblerConfig())
	ctx := context.Background()

	results := &retrieval.Results{
		Items: []*retrieval.Result{
			{
				Memory: &storage.Memory{
					ID:        "1",
					Content:   "High score content for critical zone",
					Type:      storage.MemoryTypeSemantic,
					CreatedAt: time.Now().Add(-72 * time.Hour),
				},
				Score: 0.9,
			},
			{
				Memory: &storage.Memory{
					ID:        "2",
					Content:   "Medium score content for middle zone",
					Type:      storage.MemoryTypeSemantic,
					CreatedAt: time.Now().Add(-72 * time.Hour),
				},
				Score: 0.5,
			},
			{
				Memory: &storage.Memory{
					ID:        "3",
					Content:   "Recent content for recency zone",
					Type:      storage.MemoryTypeWorking,
					CreatedAt: time.Now(),
				},
				Score: 0.4,
			},
		},
		Total: 3,
	}

	result, err := a.Assemble(ctx, results, &AssembleOptions{TokenBudget: 1000})
	require.NoError(t, err)

	// Zone stats should have budgets calculated
	assert.Greater(t, result.ZoneStats.CriticalBudget, 0)
	assert.Greater(t, result.ZoneStats.MiddleBudget, 0)
	assert.Greater(t, result.ZoneStats.RecencyBudget, 0)

	// Middle budget should be largest
	assert.Greater(t, result.ZoneStats.MiddleBudget, result.ZoneStats.CriticalBudget)
	assert.Greater(t, result.ZoneStats.MiddleBudget, result.ZoneStats.RecencyBudget)
}

func TestAssembler_Assemble_CustomZoneAllocation(t *testing.T) {
	a := NewAssembler(DefaultAssemblerConfig())
	ctx := context.Background()

	results := &retrieval.Results{
		Items: []*retrieval.Result{
			{
				Memory: &storage.Memory{
					ID:        "1",
					Content:   "Test content",
					Type:      storage.MemoryTypeSemantic,
					CreatedAt: time.Now().Add(-72 * time.Hour),
				},
				Score: 0.9,
			},
		},
		Total: 1,
	}

	// Custom allocation with larger critical zone
	customAlloc := &ZoneAllocation{
		CriticalPercent: 0.50,
		MiddlePercent:   0.30,
		RecencyPercent:  0.20,
	}

	result, err := a.Assemble(ctx, results, &AssembleOptions{
		TokenBudget:    1000,
		ZoneAllocation: customAlloc,
	})
	require.NoError(t, err)

	// Critical budget should now be larger than middle
	assert.Greater(t, result.ZoneStats.CriticalBudget, result.ZoneStats.MiddleBudget)
}

func TestAssembler_Assemble_Truncation(t *testing.T) {
	a := NewAssembler(DefaultAssemblerConfig())
	ctx := context.Background()

	// Create many memories that won't all fit
	items := make([]*retrieval.Result, 20)
	for i := 0; i < 20; i++ {
		items[i] = &retrieval.Result{
			Memory: &storage.Memory{
				ID:        string(rune('a' + i)),
				Content:   "This is memory content that takes up some space in the context window.",
				Type:      storage.MemoryTypeSemantic,
				CreatedAt: time.Now().Add(-72 * time.Hour),
			},
			Score: 0.9 - float64(i)*0.02,
		}
	}

	results := &retrieval.Results{Items: items, Total: len(items)}

	result, err := a.Assemble(ctx, results, &AssembleOptions{TokenBudget: 100})
	require.NoError(t, err)

	// Not all memories should fit
	assert.Less(t, len(result.Memories), len(items))
	assert.True(t, result.Truncated)
}

func TestAssembler_AssembleSimple(t *testing.T) {
	a := NewAssembler(DefaultAssemblerConfig())
	ctx := context.Background()

	memories := []*storage.Memory{
		{ID: "1", Content: "First memory", Type: storage.MemoryTypeSemantic},
		{ID: "2", Content: "Second memory", Type: storage.MemoryTypeSemantic},
	}

	content, tokenCount, err := a.AssembleSimple(ctx, memories, 1000)
	require.NoError(t, err)

	assert.Contains(t, content, "First memory")
	assert.Contains(t, content, "Second memory")
	assert.Greater(t, tokenCount, 0)
}

func TestAssembler_Assemble_LowConfidenceFiltered(t *testing.T) {
	cfg := DefaultAssemblerConfig()
	cfg.MinConfidence = 0.5
	a := NewAssembler(cfg)
	ctx := context.Background()

	results := &retrieval.Results{
		Items: []*retrieval.Result{
			{
				Memory: &storage.Memory{
					ID:         "high",
					Content:    "High confidence",
					Type:       storage.MemoryTypeSemantic,
					Confidence: 0.8,
					CreatedAt:  time.Now().Add(-72 * time.Hour),
				},
				Score: 0.9,
			},
			{
				Memory: &storage.Memory{
					ID:         "low",
					Content:    "Low confidence",
					Type:       storage.MemoryTypeSemantic,
					Confidence: 0.3,
					CreatedAt:  time.Now().Add(-72 * time.Hour),
				},
				Score: 0.9,
			},
		},
		Total: 2,
	}

	result, err := a.Assemble(ctx, results, &AssembleOptions{TokenBudget: 1000})
	require.NoError(t, err)

	// Only high confidence memory should be included
	require.Len(t, result.Memories, 1)
	assert.Equal(t, "high", result.Memories[0].Memory.ID)
}

func TestFormatScore(t *testing.T) {
	tests := []struct {
		score    float64
		expected string
	}{
		{0.0, "0%"},
		{0.05, "5%"},
		{0.5, "50%"},
		{0.85, "85%"},
		{1.0, "100%"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatScore(tt.score)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper function
func indexOfSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func BenchmarkAssembler_Assemble(b *testing.B) {
	a := NewAssembler(DefaultAssemblerConfig())
	ctx := context.Background()

	// Create 100 test memories
	items := make([]*retrieval.Result, 100)
	for i := 0; i < 100; i++ {
		items[i] = &retrieval.Result{
			Memory: &storage.Memory{
				ID:        string(rune(i)),
				Content:   "This is test content for benchmarking the context assembler performance.",
				Type:      storage.MemoryTypeSemantic,
				CreatedAt: time.Now().Add(-time.Duration(i) * time.Hour),
			},
			Score: 0.9 - float64(i)*0.005,
		}
	}
	results := &retrieval.Results{Items: items, Total: len(items)}
	opts := &AssembleOptions{TokenBudget: 4000}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = a.Assemble(ctx, results, opts)
	}
}
