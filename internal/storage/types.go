// Package storage provides storage interfaces and implementations for MAIA.
package storage

import (
	"context"
	"time"
)

// MemoryType represents the type of memory stored.
type MemoryType string

const (
	MemoryTypeSemantic MemoryType = "semantic" // Facts, profiles, structured knowledge
	MemoryTypeEpisodic MemoryType = "episodic" // Conversations, experiences
	MemoryTypeWorking  MemoryType = "working"  // Current session state
)

// MemorySource indicates how the memory was created.
type MemorySource string

const (
	MemorySourceUser       MemorySource = "user"       // Explicitly added by user
	MemorySourceExtracted  MemorySource = "extracted"  // Extracted from LLM response
	MemorySourceInferred   MemorySource = "inferred"   // Inferred from context
	MemorySourceImported   MemorySource = "imported"   // Imported from external source
)

// RelationType represents the type of relation between memories.
type RelationType string

const (
	RelationTypeRelatedTo   RelationType = "related_to"
	RelationTypeContradicts RelationType = "contradicts"
	RelationTypeSupersedes  RelationType = "supersedes"
	RelationTypeDerivedFrom RelationType = "derived_from"
	RelationTypePartOf      RelationType = "part_of"
)

// Memory represents a single memory unit stored in MAIA.
type Memory struct {
	ID          string                 `json:"id"`
	Namespace   string                 `json:"namespace"`
	Content     string                 `json:"content"`
	Type        MemoryType             `json:"type"`
	Embedding   []float32              `json:"embedding,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	AccessedAt  time.Time              `json:"accessed_at"`
	AccessCount int64                  `json:"access_count"`
	Confidence  float64                `json:"confidence"`
	Source      MemorySource           `json:"source"`
	Relations   []Relation             `json:"relations,omitempty"`
}

// Relation represents a connection between memories.
type Relation struct {
	TargetID string                 `json:"target_id"`
	Type     RelationType           `json:"type"`
	Weight   float64                `json:"weight"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Namespace represents a memory namespace.
type Namespace struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Parent    string          `json:"parent,omitempty"`
	Template  string          `json:"template,omitempty"`
	Config    NamespaceConfig `json:"config"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// NamespaceConfig holds configuration for a namespace.
type NamespaceConfig struct {
	TokenBudget       int               `json:"token_budget,omitempty"`
	MaxMemories       int               `json:"max_memories,omitempty"`
	RetentionDays     int               `json:"retention_days,omitempty"`
	AllowedTypes      []MemoryType      `json:"allowed_types,omitempty"`
	InheritFromParent bool              `json:"inherit_from_parent"`
	CustomScoring     map[string]float64 `json:"custom_scoring,omitempty"`
}

// CreateMemoryInput represents input for creating a new memory.
type CreateMemoryInput struct {
	Namespace  string                 `json:"namespace"`
	Content    string                 `json:"content"`
	Type       MemoryType             `json:"type"`
	Embedding  []float32              `json:"embedding,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Tags       []string               `json:"tags,omitempty"`
	Confidence float64                `json:"confidence"`
	Source     MemorySource           `json:"source"`
	Relations  []Relation             `json:"relations,omitempty"`
}

// UpdateMemoryInput represents input for updating an existing memory.
type UpdateMemoryInput struct {
	Content    *string                `json:"content,omitempty"`
	Embedding  []float32              `json:"embedding,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Tags       []string               `json:"tags,omitempty"`
	Confidence *float64               `json:"confidence,omitempty"`
	Relations  []Relation             `json:"relations,omitempty"`
}

// SearchOptions represents options for searching memories.
type SearchOptions struct {
	Namespace   string                 `json:"namespace,omitempty"`
	Types       []MemoryType           `json:"types,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
	TimeRange   *TimeRange             `json:"time_range,omitempty"`
	Filters     map[string]interface{} `json:"filters,omitempty"`
	Limit       int                    `json:"limit,omitempty"`
	Offset      int                    `json:"offset,omitempty"`
	SortBy      string                 `json:"sort_by,omitempty"`
	SortOrder   string                 `json:"sort_order,omitempty"` // asc, desc
}

// TimeRange represents a time range for filtering.
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// SearchResult represents a memory search result with score.
type SearchResult struct {
	Memory *Memory `json:"memory"`
	Score  float64 `json:"score"`
}

// CreateNamespaceInput represents input for creating a namespace.
type CreateNamespaceInput struct {
	Name     string          `json:"name"`
	Parent   string          `json:"parent,omitempty"`
	Template string          `json:"template,omitempty"`
	Config   NamespaceConfig `json:"config"`
}

// ListOptions represents options for listing items.
type ListOptions struct {
	Limit  int `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`
}

// Store defines the interface for memory storage operations.
type Store interface {
	// Memory operations
	CreateMemory(ctx context.Context, input *CreateMemoryInput) (*Memory, error)
	GetMemory(ctx context.Context, id string) (*Memory, error)
	UpdateMemory(ctx context.Context, id string, input *UpdateMemoryInput) (*Memory, error)
	DeleteMemory(ctx context.Context, id string) error
	ListMemories(ctx context.Context, namespace string, opts *ListOptions) ([]*Memory, error)
	SearchMemories(ctx context.Context, opts *SearchOptions) ([]*SearchResult, error)
	TouchMemory(ctx context.Context, id string) error // Update access time and count

	// Namespace operations
	CreateNamespace(ctx context.Context, input *CreateNamespaceInput) (*Namespace, error)
	GetNamespace(ctx context.Context, id string) (*Namespace, error)
	GetNamespaceByName(ctx context.Context, name string) (*Namespace, error)
	UpdateNamespace(ctx context.Context, id string, config *NamespaceConfig) (*Namespace, error)
	DeleteNamespace(ctx context.Context, id string) error
	ListNamespaces(ctx context.Context, opts *ListOptions) ([]*Namespace, error)

	// Bulk operations
	BatchCreateMemories(ctx context.Context, inputs []*CreateMemoryInput) ([]*Memory, error)
	BatchDeleteMemories(ctx context.Context, ids []string) error

	// Maintenance
	Close() error
	Stats(ctx context.Context) (*StoreStats, error)
}

// StoreStats represents storage statistics.
type StoreStats struct {
	TotalMemories    int64 `json:"total_memories"`
	TotalNamespaces  int64 `json:"total_namespaces"`
	StorageSizeBytes int64 `json:"storage_size_bytes"`
	LastCompaction   time.Time `json:"last_compaction"`
}

// ErrNotFound is returned when a requested item is not found.
type ErrNotFound struct {
	Type string
	ID   string
}

func (e *ErrNotFound) Error() string {
	return e.Type + " not found: " + e.ID
}

// ErrAlreadyExists is returned when trying to create an item that already exists.
type ErrAlreadyExists struct {
	Type string
	ID   string
}

func (e *ErrAlreadyExists) Error() string {
	return e.Type + " already exists: " + e.ID
}

// ErrInvalidInput is returned when input validation fails.
type ErrInvalidInput struct {
	Field   string
	Message string
}

func (e *ErrInvalidInput) Error() string {
	return "invalid " + e.Field + ": " + e.Message
}
