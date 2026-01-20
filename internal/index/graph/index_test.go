package graph

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewInMemoryIndex(t *testing.T) {
	idx := NewInMemoryIndex()
	require.NotNil(t, idx)
	assert.Equal(t, 0, idx.Size())
	assert.Equal(t, 0, idx.NodeCount())
}

func TestInMemoryIndex_AddEdge(t *testing.T) {
	tests := []struct {
		name     string
		sourceID string
		targetID string
		relation string
		weight   float32
		wantErr  bool
	}{
		{
			name:     "valid edge",
			sourceID: "mem1",
			targetID: "mem2",
			relation: RelationRelatedTo,
			weight:   0.8,
			wantErr:  false,
		},
		{
			name:     "edge with default relation",
			sourceID: "mem1",
			targetID: "mem3",
			relation: "",
			weight:   0.5,
			wantErr:  false,
		},
		{
			name:     "empty source ID",
			sourceID: "",
			targetID: "mem2",
			relation: RelationRelatedTo,
			weight:   0.8,
			wantErr:  true,
		},
		{
			name:     "empty target ID",
			sourceID: "mem1",
			targetID: "",
			relation: RelationRelatedTo,
			weight:   0.8,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := NewInMemoryIndex()
			ctx := context.Background()

			err := idx.AddEdge(ctx, tt.sourceID, tt.targetID, tt.relation, tt.weight)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Verify edge was added
			if tt.sourceID != "" && tt.targetID != "" {
				relation := tt.relation
				if relation == "" {
					relation = RelationRelatedTo
				}
				assert.True(t, idx.HasEdge(ctx, tt.sourceID, tt.targetID, relation))
				assert.Equal(t, 1, idx.Size())
				assert.Equal(t, 2, idx.NodeCount())
			}
		})
	}
}

func TestInMemoryIndex_AddEdgeWithMetadata(t *testing.T) {
	idx := NewInMemoryIndex()
	ctx := context.Background()

	edge := &Edge{
		SourceID:  "mem1",
		TargetID:  "mem2",
		Relation:  RelationReferences,
		Weight:    0.9,
		Metadata:  map[string]string{"context": "test"},
		CreatedAt: time.Now(),
	}

	err := idx.AddEdgeWithMetadata(ctx, edge)
	require.NoError(t, err)

	// Verify edge was added
	edges, err := idx.GetOutgoing(ctx, "mem1")
	require.NoError(t, err)
	require.Len(t, edges, 1)
	assert.Equal(t, "test", edges[0].Metadata["context"])
}

func TestInMemoryIndex_AddEdge_UpdateExisting(t *testing.T) {
	idx := NewInMemoryIndex()
	ctx := context.Background()

	// Add initial edge
	err := idx.AddEdge(ctx, "mem1", "mem2", RelationRelatedTo, 0.5)
	require.NoError(t, err)
	assert.Equal(t, 1, idx.Size())

	// Add same edge with different weight (should update)
	err = idx.AddEdge(ctx, "mem1", "mem2", RelationRelatedTo, 0.9)
	require.NoError(t, err)
	assert.Equal(t, 1, idx.Size())

	// Verify weight was updated
	edges, err := idx.GetOutgoing(ctx, "mem1")
	require.NoError(t, err)
	require.Len(t, edges, 1)
	assert.Equal(t, float32(0.9), edges[0].Weight)
}

func TestInMemoryIndex_RemoveEdge(t *testing.T) {
	idx := NewInMemoryIndex()
	ctx := context.Background()

	// Add edge
	err := idx.AddEdge(ctx, "mem1", "mem2", RelationRelatedTo, 0.8)
	require.NoError(t, err)
	assert.Equal(t, 1, idx.Size())

	// Remove edge
	err = idx.RemoveEdge(ctx, "mem1", "mem2", RelationRelatedTo)
	require.NoError(t, err)
	assert.Equal(t, 0, idx.Size())

	// Verify edge is gone
	assert.False(t, idx.HasEdge(ctx, "mem1", "mem2", RelationRelatedTo))

	// Try to remove non-existent edge
	err = idx.RemoveEdge(ctx, "mem1", "mem2", RelationRelatedTo)
	assert.ErrorIs(t, err, ErrEdgeNotFound)
}

func TestInMemoryIndex_RemoveNode(t *testing.T) {
	idx := NewInMemoryIndex()
	ctx := context.Background()

	// Create a graph: mem1 -> mem2 -> mem3
	//                 mem1 -> mem3
	err := idx.AddEdge(ctx, "mem1", "mem2", RelationFollows, 1.0)
	require.NoError(t, err)
	err = idx.AddEdge(ctx, "mem2", "mem3", RelationFollows, 1.0)
	require.NoError(t, err)
	err = idx.AddEdge(ctx, "mem1", "mem3", RelationRelatedTo, 0.5)
	require.NoError(t, err)
	assert.Equal(t, 3, idx.Size())
	assert.Equal(t, 3, idx.NodeCount())

	// Remove mem2 (middle node)
	err = idx.RemoveNode(ctx, "mem2")
	require.NoError(t, err)

	// Should have only mem1 -> mem3 edge left
	assert.Equal(t, 1, idx.Size())
	assert.Equal(t, 2, idx.NodeCount())
	assert.False(t, idx.HasEdge(ctx, "mem1", "mem2", RelationFollows))
	assert.False(t, idx.HasEdge(ctx, "mem2", "mem3", RelationFollows))
	assert.True(t, idx.HasEdge(ctx, "mem1", "mem3", RelationRelatedTo))
}

func TestInMemoryIndex_GetOutgoing(t *testing.T) {
	idx := NewInMemoryIndex()
	ctx := context.Background()

	// Add multiple outgoing edges from mem1
	require.NoError(t, idx.AddEdge(ctx, "mem1", "mem2", RelationRelatedTo, 0.8))
	require.NoError(t, idx.AddEdge(ctx, "mem1", "mem3", RelationFollows, 0.6))
	require.NoError(t, idx.AddEdge(ctx, "mem2", "mem3", RelationRelatedTo, 0.5))

	edges, err := idx.GetOutgoing(ctx, "mem1")
	require.NoError(t, err)
	assert.Len(t, edges, 2)

	// Check edges for non-existent node
	edges, err = idx.GetOutgoing(ctx, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, edges)
}

func TestInMemoryIndex_GetIncoming(t *testing.T) {
	idx := NewInMemoryIndex()
	ctx := context.Background()

	// Add multiple incoming edges to mem3
	require.NoError(t, idx.AddEdge(ctx, "mem1", "mem3", RelationRelatedTo, 0.8))
	require.NoError(t, idx.AddEdge(ctx, "mem2", "mem3", RelationFollows, 0.6))

	edges, err := idx.GetIncoming(ctx, "mem3")
	require.NoError(t, err)
	assert.Len(t, edges, 2)

	// Check edges for non-existent node
	edges, err = idx.GetIncoming(ctx, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, edges)
}

func TestInMemoryIndex_GetRelated(t *testing.T) {
	idx := NewInMemoryIndex()
	ctx := context.Background()

	// Build graph
	require.NoError(t, idx.AddEdge(ctx, "mem1", "mem2", RelationRelatedTo, 0.8))
	require.NoError(t, idx.AddEdge(ctx, "mem1", "mem3", RelationReferences, 0.6))
	require.NoError(t, idx.AddEdge(ctx, "mem4", "mem1", RelationFollows, 0.9))

	// Get all related (single hop, both directions)
	results, err := idx.GetRelated(ctx, "mem1", &TraversalOptions{
		Direction: DirectionBoth,
	})
	require.NoError(t, err)
	assert.Len(t, results, 3) // mem2, mem3, mem4

	// Get only outgoing relations
	results, err = idx.GetRelated(ctx, "mem1", &TraversalOptions{
		Direction: DirectionOutgoing,
	})
	require.NoError(t, err)
	assert.Len(t, results, 2) // mem2, mem3

	// Get only specific relation
	results, err = idx.GetRelated(ctx, "mem1", &TraversalOptions{
		Direction: DirectionOutgoing,
		Relations: []string{RelationRelatedTo},
	})
	require.NoError(t, err)
	assert.Len(t, results, 1) // only mem2
	assert.Equal(t, "mem2", results[0].ID)
}

func TestInMemoryIndex_Traverse(t *testing.T) {
	idx := NewInMemoryIndex()
	ctx := context.Background()

	// Build a longer chain: A -> B -> C -> D
	require.NoError(t, idx.AddEdge(ctx, "A", "B", RelationFollows, 1.0))
	require.NoError(t, idx.AddEdge(ctx, "B", "C", RelationFollows, 0.9))
	require.NoError(t, idx.AddEdge(ctx, "C", "D", RelationFollows, 0.8))

	// Traverse with depth 1 (single hop)
	results, err := idx.Traverse(ctx, "A", &TraversalOptions{
		Direction:  DirectionOutgoing,
		MaxDepth:   1,
		MaxResults: 10,
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "B", results[0].ID)
	assert.Equal(t, 1, results[0].Depth)

	// Traverse with depth 3 (full chain)
	results, err = idx.Traverse(ctx, "A", &TraversalOptions{
		Direction:   DirectionOutgoing,
		MaxDepth:    3,
		MaxResults:  10,
		IncludePath: true,
	})
	require.NoError(t, err)
	assert.Len(t, results, 3) // B, C, D

	// Verify depths
	depths := make(map[string]int)
	for _, r := range results {
		depths[r.ID] = r.Depth
	}
	assert.Equal(t, 1, depths["B"])
	assert.Equal(t, 2, depths["C"])
	assert.Equal(t, 3, depths["D"])

	// Verify path for deepest node
	for _, r := range results {
		if r.ID == "D" {
			assert.Equal(t, []string{"A", "B", "C", "D"}, r.Path)
		}
	}
}

func TestInMemoryIndex_Traverse_WithFilters(t *testing.T) {
	idx := NewInMemoryIndex()
	ctx := context.Background()

	// Build mixed graph
	require.NoError(t, idx.AddEdge(ctx, "A", "B", RelationFollows, 0.9))
	require.NoError(t, idx.AddEdge(ctx, "A", "C", RelationRelatedTo, 0.3))
	require.NoError(t, idx.AddEdge(ctx, "B", "D", RelationFollows, 0.8))

	// Filter by relation
	results, err := idx.Traverse(ctx, "A", &TraversalOptions{
		Direction:  DirectionOutgoing,
		MaxDepth:   2,
		Relations:  []string{RelationFollows},
		MaxResults: 10,
	})
	require.NoError(t, err)
	assert.Len(t, results, 2) // B and D only

	// Filter by minimum weight
	results, err = idx.Traverse(ctx, "A", &TraversalOptions{
		Direction:  DirectionOutgoing,
		MaxDepth:   2,
		MinWeight:  0.5,
		MaxResults: 10,
	})
	require.NoError(t, err)
	assert.Len(t, results, 2) // B and D (C has weight 0.3)
}

func TestInMemoryIndex_Traverse_MaxResults(t *testing.T) {
	idx := NewInMemoryIndex()
	ctx := context.Background()

	// Build wide graph: A -> B1, B2, B3, B4, B5
	for i := 1; i <= 5; i++ {
		require.NoError(t, idx.AddEdge(ctx, "A", "B"+string(rune('0'+i)), RelationRelatedTo, 0.8))
	}

	// Limit results
	results, err := idx.Traverse(ctx, "A", &TraversalOptions{
		Direction:  DirectionOutgoing,
		MaxDepth:   1,
		MaxResults: 3,
	})
	require.NoError(t, err)
	assert.Len(t, results, 3)
}

func TestInMemoryIndex_Traverse_Bidirectional(t *testing.T) {
	idx := NewInMemoryIndex()
	ctx := context.Background()

	// Build graph: A -> B -> C
	//              D -> B
	require.NoError(t, idx.AddEdge(ctx, "A", "B", RelationFollows, 1.0))
	require.NoError(t, idx.AddEdge(ctx, "B", "C", RelationFollows, 1.0))
	require.NoError(t, idx.AddEdge(ctx, "D", "B", RelationRelatedTo, 0.8))

	// Traverse from B in both directions
	results, err := idx.Traverse(ctx, "B", &TraversalOptions{
		Direction:  DirectionBoth,
		MaxDepth:   1,
		MaxResults: 10,
	})
	require.NoError(t, err)
	assert.Len(t, results, 3) // A, C, D

	// Verify we found nodes in both directions
	ids := make(map[string]bool)
	for _, r := range results {
		ids[r.ID] = true
	}
	assert.True(t, ids["A"]) // incoming from A
	assert.True(t, ids["C"]) // outgoing to C
	assert.True(t, ids["D"]) // incoming from D
}

func TestInMemoryIndex_HasEdge(t *testing.T) {
	idx := NewInMemoryIndex()
	ctx := context.Background()

	require.NoError(t, idx.AddEdge(ctx, "mem1", "mem2", RelationRelatedTo, 0.8))

	// Exact match
	assert.True(t, idx.HasEdge(ctx, "mem1", "mem2", RelationRelatedTo))

	// Wrong relation
	assert.False(t, idx.HasEdge(ctx, "mem1", "mem2", RelationFollows))

	// Empty relation (matches any)
	assert.True(t, idx.HasEdge(ctx, "mem1", "mem2", ""))

	// Non-existent edge
	assert.False(t, idx.HasEdge(ctx, "mem2", "mem1", RelationRelatedTo))
}

func TestInMemoryIndex_ContextCancellation(t *testing.T) {
	idx := NewInMemoryIndex()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// All operations should return context error
	err := idx.AddEdge(ctx, "mem1", "mem2", RelationRelatedTo, 0.8)
	assert.ErrorIs(t, err, context.Canceled)

	err = idx.RemoveEdge(ctx, "mem1", "mem2", RelationRelatedTo)
	assert.ErrorIs(t, err, context.Canceled)

	err = idx.RemoveNode(ctx, "mem1")
	assert.ErrorIs(t, err, context.Canceled)

	_, err = idx.GetOutgoing(ctx, "mem1")
	assert.ErrorIs(t, err, context.Canceled)

	_, err = idx.GetIncoming(ctx, "mem1")
	assert.ErrorIs(t, err, context.Canceled)

	_, err = idx.Traverse(ctx, "mem1", nil)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestInMemoryIndex_ClosedIndex(t *testing.T) {
	idx := NewInMemoryIndex()
	ctx := context.Background()

	require.NoError(t, idx.Close())

	// All operations should return closed error
	err := idx.AddEdge(ctx, "mem1", "mem2", RelationRelatedTo, 0.8)
	assert.ErrorIs(t, err, ErrIndexClosed)

	err = idx.RemoveEdge(ctx, "mem1", "mem2", RelationRelatedTo)
	assert.ErrorIs(t, err, ErrIndexClosed)

	err = idx.RemoveNode(ctx, "mem1")
	assert.ErrorIs(t, err, ErrIndexClosed)

	_, err = idx.GetOutgoing(ctx, "mem1")
	assert.ErrorIs(t, err, ErrIndexClosed)

	_, err = idx.GetIncoming(ctx, "mem1")
	assert.ErrorIs(t, err, ErrIndexClosed)

	_, err = idx.Traverse(ctx, "mem1", nil)
	assert.ErrorIs(t, err, ErrIndexClosed)

	assert.False(t, idx.HasEdge(ctx, "mem1", "mem2", ""))

	// Save should also fail
	var buf bytes.Buffer
	err = idx.Save(&buf)
	assert.ErrorIs(t, err, ErrIndexClosed)

	// Load should also fail
	err = idx.Load(&buf)
	assert.ErrorIs(t, err, ErrIndexClosed)
}

func TestInMemoryIndex_SaveLoad(t *testing.T) {
	idx := NewInMemoryIndex()
	ctx := context.Background()

	// Add some edges
	require.NoError(t, idx.AddEdge(ctx, "mem1", "mem2", RelationRelatedTo, 0.8))
	require.NoError(t, idx.AddEdge(ctx, "mem1", "mem3", RelationFollows, 0.6))
	require.NoError(t, idx.AddEdgeWithMetadata(ctx, &Edge{
		SourceID:  "mem2",
		TargetID:  "mem3",
		Relation:  RelationReferences,
		Weight:    0.9,
		Metadata:  map[string]string{"key": "value"},
		CreatedAt: time.Now(),
	}))

	// Save to buffer
	var buf bytes.Buffer
	err := idx.Save(&buf)
	require.NoError(t, err)

	// Create new index and load
	idx2 := NewInMemoryIndex()
	err = idx2.Load(&buf)
	require.NoError(t, err)

	// Verify data was restored
	assert.Equal(t, idx.Size(), idx2.Size())
	assert.Equal(t, idx.NodeCount(), idx2.NodeCount())
	assert.True(t, idx2.HasEdge(ctx, "mem1", "mem2", RelationRelatedTo))
	assert.True(t, idx2.HasEdge(ctx, "mem1", "mem3", RelationFollows))
	assert.True(t, idx2.HasEdge(ctx, "mem2", "mem3", RelationReferences))

	// Verify metadata was preserved
	edges, err := idx2.GetOutgoing(ctx, "mem2")
	require.NoError(t, err)
	require.Len(t, edges, 1)
	assert.Equal(t, "value", edges[0].Metadata["key"])
}

func TestInMemoryIndex_SaveLoad_EmptyIndex(t *testing.T) {
	idx := NewInMemoryIndex()

	var buf bytes.Buffer
	err := idx.Save(&buf)
	require.NoError(t, err)

	idx2 := NewInMemoryIndex()
	err = idx2.Load(&buf)
	require.NoError(t, err)

	assert.Equal(t, 0, idx2.Size())
}

func TestInMemoryIndex_Load_InvalidFormat(t *testing.T) {
	idx := NewInMemoryIndex()

	// Invalid magic number
	buf := bytes.NewBuffer([]byte{0x00, 0x00, 0x00, 0x00})
	err := idx.Load(buf)
	assert.ErrorIs(t, err, ErrInvalidFormat)
}

func TestEdge_Validate(t *testing.T) {
	tests := []struct {
		name    string
		edge    *Edge
		wantErr bool
	}{
		{
			name: "valid edge",
			edge: &Edge{
				SourceID: "src",
				TargetID: "tgt",
				Relation: RelationRelatedTo,
				Weight:   0.5,
			},
			wantErr: false,
		},
		{
			name: "empty source",
			edge: &Edge{
				SourceID: "",
				TargetID: "tgt",
			},
			wantErr: true,
		},
		{
			name: "empty target",
			edge: &Edge{
				SourceID: "src",
				TargetID: "",
			},
			wantErr: true,
		},
		{
			name: "empty relation gets default",
			edge: &Edge{
				SourceID: "src",
				TargetID: "tgt",
				Relation: "",
			},
			wantErr: false,
		},
		{
			name: "negative weight clamped to 0",
			edge: &Edge{
				SourceID: "src",
				TargetID: "tgt",
				Weight:   -0.5,
			},
			wantErr: false,
		},
		{
			name: "weight > 1 clamped to 1",
			edge: &Edge{
				SourceID: "src",
				TargetID: "tgt",
				Weight:   1.5,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.edge.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				// Check clamping
				if tt.edge.Weight < 0 {
					assert.Equal(t, float32(0), tt.edge.Weight)
				}
				if tt.edge.Weight > 1 {
					assert.Equal(t, float32(1), tt.edge.Weight)
				}
				// Check default relation
				if tt.edge.Relation == "" {
					assert.Equal(t, RelationRelatedTo, tt.edge.Relation)
				}
			}
		})
	}
}

func TestDirection_String(t *testing.T) {
	assert.Equal(t, "outgoing", DirectionOutgoing.String())
	assert.Equal(t, "incoming", DirectionIncoming.String())
	assert.Equal(t, "both", DirectionBoth.String())
	assert.Equal(t, "unknown", Direction(99).String())
}

func TestDefaultTraversalOptions(t *testing.T) {
	opts := DefaultTraversalOptions()
	require.NotNil(t, opts)
	assert.Equal(t, DirectionBoth, opts.Direction)
	assert.Equal(t, 3, opts.MaxDepth)
	assert.Equal(t, 100, opts.MaxResults)
	assert.Equal(t, float32(0), opts.MinWeight)
	assert.False(t, opts.IncludePath)
}

func TestInMemoryIndex_Traverse_CycleHandling(t *testing.T) {
	idx := NewInMemoryIndex()
	ctx := context.Background()

	// Create a cycle: A -> B -> C -> A
	require.NoError(t, idx.AddEdge(ctx, "A", "B", RelationFollows, 1.0))
	require.NoError(t, idx.AddEdge(ctx, "B", "C", RelationFollows, 1.0))
	require.NoError(t, idx.AddEdge(ctx, "C", "A", RelationFollows, 1.0))

	// Traversal should not loop forever
	results, err := idx.Traverse(ctx, "A", &TraversalOptions{
		Direction:  DirectionOutgoing,
		MaxDepth:   10,
		MaxResults: 100,
	})
	require.NoError(t, err)
	// Should find B and C, but not revisit A
	assert.Len(t, results, 2)
}

func TestInMemoryIndex_CumulativeWeight(t *testing.T) {
	idx := NewInMemoryIndex()
	ctx := context.Background()

	// Build chain: A -0.8-> B -0.5-> C
	require.NoError(t, idx.AddEdge(ctx, "A", "B", RelationFollows, 0.8))
	require.NoError(t, idx.AddEdge(ctx, "B", "C", RelationFollows, 0.5))

	results, err := idx.Traverse(ctx, "A", &TraversalOptions{
		Direction:  DirectionOutgoing,
		MaxDepth:   2,
		MaxResults: 10,
	})
	require.NoError(t, err)
	require.Len(t, results, 2)

	// Find C and check cumulative weight
	for _, r := range results {
		if r.ID == "C" {
			// Cumulative should be 0.8 * 0.5 = 0.4
			assert.InDelta(t, 0.4, r.Cumulative, 0.001)
		}
	}
}
