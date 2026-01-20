// Package graph provides graph-based memory relationship indexing for MAIA.
package graph

import (
	"context"
	"errors"
	"io"
	"time"
)

// Common errors for graph index operations.
var (
	ErrIndexClosed   = errors.New("graph index is closed")
	ErrEdgeNotFound  = errors.New("edge not found")
	ErrNodeNotFound  = errors.New("node not found")
	ErrInvalidEdge   = errors.New("invalid edge: source and target IDs required")
	ErrCycleDetected = errors.New("cycle detected during traversal")
	ErrMaxDepth      = errors.New("maximum traversal depth exceeded")
)

// Standard relationship types for consistency.
const (
	RelationRelatedTo  = "related_to"  // General relation
	RelationReferences = "references"  // Explicitly references
	RelationFollows    = "follows"     // Temporal sequence
	RelationCausedBy   = "caused_by"   // Causal chain
	RelationPartOf     = "part_of"     // Containment
	RelationSameAs     = "same_as"     // Equivalence
	RelationDerivedFrom = "derived_from" // Transformation/derivation
	RelationContains   = "contains"    // Inverse of part_of
)

// Direction specifies edge traversal direction.
type Direction int

const (
	// DirectionOutgoing traverses only outgoing edges.
	DirectionOutgoing Direction = iota
	// DirectionIncoming traverses only incoming edges.
	DirectionIncoming
	// DirectionBoth traverses both directions.
	DirectionBoth
)

// String returns the string representation of a Direction.
func (d Direction) String() string {
	switch d {
	case DirectionOutgoing:
		return "outgoing"
	case DirectionIncoming:
		return "incoming"
	case DirectionBoth:
		return "both"
	default:
		return "unknown"
	}
}

// Edge represents a directed relationship between memories.
type Edge struct {
	SourceID  string            // Source memory ID
	TargetID  string            // Target memory ID
	Relation  string            // Relationship type
	Weight    float32           // Edge weight (0-1, for scoring)
	Metadata  map[string]string // Optional metadata
	CreatedAt time.Time         // When the edge was created
}

// Validate checks if the edge has required fields.
func (e *Edge) Validate() error {
	if e.SourceID == "" || e.TargetID == "" {
		return ErrInvalidEdge
	}
	if e.Relation == "" {
		e.Relation = RelationRelatedTo
	}
	if e.Weight < 0 {
		e.Weight = 0
	}
	if e.Weight > 1 {
		e.Weight = 1
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	return nil
}

// TraversalOptions configures graph traversal.
type TraversalOptions struct {
	// Relations filters by relationship types (empty = all).
	Relations []string

	// Direction specifies traversal direction.
	Direction Direction

	// MaxDepth limits traversal depth (1 = direct neighbors only).
	// 0 means unlimited (use with caution).
	MaxDepth int

	// MaxResults limits total results.
	MaxResults int

	// MinWeight filters edges by minimum weight.
	MinWeight float32

	// IncludePath includes the full path to each result.
	IncludePath bool
}

// DefaultTraversalOptions returns sensible defaults.
func DefaultTraversalOptions() *TraversalOptions {
	return &TraversalOptions{
		Direction:   DirectionBoth,
		MaxDepth:    3,
		MaxResults:  100,
		MinWeight:   0,
		IncludePath: false,
	}
}

// TraversalResult represents a node found during traversal.
type TraversalResult struct {
	ID         string   // Memory ID
	Depth      int      // Distance from start node
	Relation   string   // Relationship that led here
	Weight     float32  // Edge weight
	Path       []string // Path from start to this node (if IncludePath)
	Cumulative float32  // Cumulative weight along path
}

// Index defines the interface for graph-based memory relationships.
type Index interface {
	// AddEdge creates a directed edge between two memories.
	AddEdge(ctx context.Context, sourceID, targetID, relation string, weight float32) error

	// AddEdgeWithMetadata creates an edge with additional metadata.
	AddEdgeWithMetadata(ctx context.Context, edge *Edge) error

	// RemoveEdge removes a specific edge.
	RemoveEdge(ctx context.Context, sourceID, targetID, relation string) error

	// RemoveNode removes all edges to/from a node.
	RemoveNode(ctx context.Context, id string) error

	// GetOutgoing returns all outgoing edges from a node.
	GetOutgoing(ctx context.Context, id string) ([]Edge, error)

	// GetIncoming returns all incoming edges to a node.
	GetIncoming(ctx context.Context, id string) ([]Edge, error)

	// GetRelated returns related nodes with optional relation filter.
	GetRelated(ctx context.Context, id string, opts *TraversalOptions) ([]TraversalResult, error)

	// Traverse performs multi-hop graph traversal.
	Traverse(ctx context.Context, startID string, opts *TraversalOptions) ([]TraversalResult, error)

	// HasEdge checks if a specific edge exists.
	HasEdge(ctx context.Context, sourceID, targetID, relation string) bool

	// Size returns the total number of edges.
	Size() int

	// NodeCount returns the number of nodes with edges.
	NodeCount() int

	// Save persists the index to a writer.
	Save(w io.Writer) error

	// Load restores the index from a reader.
	Load(r io.Reader) error

	// Close releases resources.
	Close() error
}
