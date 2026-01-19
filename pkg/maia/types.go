// Package maia provides a Go SDK for interacting with the MAIA memory system.
package maia

import (
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
	MemorySourceUser      MemorySource = "user"      // Explicitly added by user
	MemorySourceExtracted MemorySource = "extracted" // Extracted from LLM response
	MemorySourceInferred  MemorySource = "inferred"  // Inferred from context
	MemorySourceImported  MemorySource = "imported"  // Imported from external source
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
	TokenBudget       int                `json:"token_budget,omitempty"`
	MaxMemories       int                `json:"max_memories,omitempty"`
	RetentionDays     int                `json:"retention_days,omitempty"`
	AllowedTypes      []MemoryType       `json:"allowed_types,omitempty"`
	InheritFromParent bool               `json:"inherit_from_parent"`
	CustomScoring     map[string]float64 `json:"custom_scoring,omitempty"`
}

// CreateMemoryInput represents input for creating a new memory.
type CreateMemoryInput struct {
	Namespace  string                 `json:"namespace"`
	Content    string                 `json:"content"`
	Type       MemoryType             `json:"type,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Tags       []string               `json:"tags,omitempty"`
	Confidence float64                `json:"confidence,omitempty"`
	Source     MemorySource           `json:"source,omitempty"`
}

// UpdateMemoryInput represents input for updating an existing memory.
type UpdateMemoryInput struct {
	Content    *string                `json:"content,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Tags       []string               `json:"tags,omitempty"`
	Confidence *float64               `json:"confidence,omitempty"`
}

// SearchMemoriesInput represents input for searching memories.
type SearchMemoriesInput struct {
	Query     string       `json:"query,omitempty"`
	Namespace string       `json:"namespace,omitempty"`
	Types     []MemoryType `json:"types,omitempty"`
	Tags      []string     `json:"tags,omitempty"`
	Limit     int          `json:"limit,omitempty"`
	Offset    int          `json:"offset,omitempty"`
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
	Config   NamespaceConfig `json:"config,omitempty"`
}

// UpdateNamespaceInput represents input for updating a namespace.
type UpdateNamespaceInput struct {
	Config NamespaceConfig `json:"config"`
}

// ListOptions represents options for listing items.
type ListOptions struct {
	Limit  int `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`
}

// ListResponse represents a paginated list response.
type ListResponse[T any] struct {
	Data   []T `json:"data"`
	Count  int `json:"count"`
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

// GetContextInput represents input for getting assembled context.
type GetContextInput struct {
	Query         string  `json:"query"`
	Namespace     string  `json:"namespace,omitempty"`
	TokenBudget   int     `json:"token_budget,omitempty"`
	SystemPrompt  string  `json:"system_prompt,omitempty"`
	IncludeScores bool    `json:"include_scores,omitempty"`
	MinScore      float64 `json:"min_score,omitempty"`
}

// ContextResponse represents the assembled context response.
type ContextResponse struct {
	Content     string            `json:"content"`
	Memories    []*ContextMemory  `json:"memories"`
	TokenCount  int               `json:"token_count"`
	TokenBudget int               `json:"token_budget"`
	Truncated   bool              `json:"truncated"`
	ZoneStats   *ContextZoneStats `json:"zone_stats,omitempty"`
	QueryTime   string            `json:"query_time"`
}

// ContextMemory represents a memory in the context response.
type ContextMemory struct {
	ID         string  `json:"id"`
	Content    string  `json:"content"`
	Type       string  `json:"type"`
	Score      float64 `json:"score,omitempty"`
	Position   string  `json:"position"`
	TokenCount int     `json:"token_count"`
	Truncated  bool    `json:"truncated"`
}

// ContextZoneStats represents zone statistics.
type ContextZoneStats struct {
	CriticalUsed   int `json:"critical_used"`
	CriticalBudget int `json:"critical_budget"`
	MiddleUsed     int `json:"middle_used"`
	MiddleBudget   int `json:"middle_budget"`
	RecencyUsed    int `json:"recency_used"`
	RecencyBudget  int `json:"recency_budget"`
}

// Stats represents storage statistics.
type Stats struct {
	TotalMemories    int64     `json:"total_memories"`
	TotalNamespaces  int64     `json:"total_namespaces"`
	StorageSizeBytes int64     `json:"storage_size_bytes"`
	LastCompaction   time.Time `json:"last_compaction"`
}

// HealthResponse represents the health check response.
type HealthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

// DeleteResponse represents a delete operation response.
type DeleteResponse struct {
	Deleted bool `json:"deleted"`
}
