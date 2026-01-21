// Package audit provides comprehensive audit logging for MAIA operations.
package audit

import (
	"context"
	"encoding/json"
	"time"
)

// EventType represents the type of audit event.
type EventType string

const (
	// Memory operations
	EventMemoryCreate EventType = "memory.create"
	EventMemoryRead   EventType = "memory.read"
	EventMemoryUpdate EventType = "memory.update"
	EventMemoryDelete EventType = "memory.delete"
	EventMemorySearch EventType = "memory.search"

	// Namespace operations
	EventNamespaceCreate EventType = "namespace.create"
	EventNamespaceRead   EventType = "namespace.read"
	EventNamespaceUpdate EventType = "namespace.update"
	EventNamespaceDelete EventType = "namespace.delete"
	EventNamespaceList   EventType = "namespace.list"

	// Context operations
	EventContextAssemble EventType = "context.assemble"

	// Tenant operations
	EventTenantCreate  EventType = "tenant.create"
	EventTenantUpdate  EventType = "tenant.update"
	EventTenantDelete  EventType = "tenant.delete"
	EventTenantSuspend EventType = "tenant.suspend"
	EventTenantResume  EventType = "tenant.resume"

	// API Key operations
	EventAPIKeyCreate EventType = "apikey.create"
	EventAPIKeyRevoke EventType = "apikey.revoke"
	EventAPIKeyRotate EventType = "apikey.rotate"

	// Authentication/Authorization
	EventAuthSuccess    EventType = "auth.success"
	EventAuthFailure    EventType = "auth.failure"
	EventAuthScopeDenied EventType = "auth.scope_denied"

	// System events
	EventSystemStartup  EventType = "system.startup"
	EventSystemShutdown EventType = "system.shutdown"
	EventSystemBackup   EventType = "system.backup"
	EventSystemRestore  EventType = "system.restore"
)

// Outcome represents the result of an audited action.
type Outcome string

const (
	OutcomeSuccess Outcome = "success"
	OutcomeFailure Outcome = "failure"
	OutcomeDenied  Outcome = "denied"
)

// Event represents a single audit log entry.
type Event struct {
	// Unique identifier for this event
	ID string `json:"id"`

	// Timestamp when the event occurred
	Timestamp time.Time `json:"timestamp"`

	// Type of event
	Type EventType `json:"type"`

	// Outcome of the action
	Outcome Outcome `json:"outcome"`

	// Actor information
	Actor Actor `json:"actor"`

	// Resource being acted upon
	Resource Resource `json:"resource"`

	// Request details
	Request RequestInfo `json:"request"`

	// Action-specific details
	Details map[string]any `json:"details,omitempty"`

	// Error information if outcome is failure
	Error *ErrorInfo `json:"error,omitempty"`

	// Duration of the operation in milliseconds
	DurationMs int64 `json:"duration_ms,omitempty"`
}

// Actor represents who performed the action.
type Actor struct {
	// Type of actor (user, service, system)
	Type string `json:"type"`

	// Unique identifier for the actor
	ID string `json:"id"`

	// API key ID used (if applicable)
	APIKeyID string `json:"api_key_id,omitempty"`

	// Tenant ID the actor belongs to
	TenantID string `json:"tenant_id,omitempty"`

	// User agent string
	UserAgent string `json:"user_agent,omitempty"`
}

// Resource represents the resource being acted upon.
type Resource struct {
	// Type of resource (memory, namespace, tenant, etc.)
	Type string `json:"type"`

	// Unique identifier of the resource
	ID string `json:"id,omitempty"`

	// Namespace the resource belongs to
	Namespace string `json:"namespace,omitempty"`

	// Tenant ID the resource belongs to
	TenantID string `json:"tenant_id,omitempty"`
}

// RequestInfo contains HTTP request details.
type RequestInfo struct {
	// HTTP method
	Method string `json:"method"`

	// Request path
	Path string `json:"path"`

	// Client IP address
	ClientIP string `json:"client_ip"`

	// Request ID for correlation
	RequestID string `json:"request_id"`

	// Query parameters (sanitized)
	QueryParams map[string]string `json:"query_params,omitempty"`
}

// ErrorInfo contains error details for failed operations.
type ErrorInfo struct {
	// Error code
	Code string `json:"code"`

	// Error message (sanitized, no sensitive data)
	Message string `json:"message"`
}

// MarshalJSON implements custom JSON marshaling.
func (e *Event) MarshalJSON() ([]byte, error) {
	type Alias Event
	return json.Marshal(&struct {
		*Alias
		Timestamp string `json:"timestamp"`
	}{
		Alias:     (*Alias)(e),
		Timestamp: e.Timestamp.Format(time.RFC3339Nano),
	})
}

// Logger defines the interface for audit logging backends.
type Logger interface {
	// Log records an audit event
	Log(ctx context.Context, event *Event) error

	// Query retrieves audit events matching the filter
	Query(ctx context.Context, filter *QueryFilter) ([]*Event, error)

	// Close releases any resources
	Close() error
}

// QueryFilter defines criteria for querying audit logs.
type QueryFilter struct {
	// Time range
	StartTime *time.Time `json:"start_time,omitempty"`
	EndTime   *time.Time `json:"end_time,omitempty"`

	// Filter by event types
	EventTypes []EventType `json:"event_types,omitempty"`

	// Filter by outcome
	Outcomes []Outcome `json:"outcomes,omitempty"`

	// Filter by actor
	ActorID   string `json:"actor_id,omitempty"`
	ActorType string `json:"actor_type,omitempty"`
	TenantID  string `json:"tenant_id,omitempty"`
	APIKeyID  string `json:"api_key_id,omitempty"`

	// Filter by resource
	ResourceType string `json:"resource_type,omitempty"`
	ResourceID   string `json:"resource_id,omitempty"`
	Namespace    string `json:"namespace,omitempty"`

	// Filter by request
	RequestID string `json:"request_id,omitempty"`
	ClientIP  string `json:"client_ip,omitempty"`

	// Pagination
	Limit  int `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`

	// Ordering
	OrderBy   string `json:"order_by,omitempty"`
	OrderDesc bool   `json:"order_desc,omitempty"`
}

// Config holds audit logging configuration.
type Config struct {
	// Enable audit logging
	Enabled bool `mapstructure:"enabled"`

	// Log level for audit events (all, write, admin)
	Level string `mapstructure:"level"`

	// Backend configuration
	Backend BackendConfig `mapstructure:"backend"`

	// Retention period for audit logs
	RetentionDays int `mapstructure:"retention_days"`

	// Batch settings for async logging
	BatchSize    int           `mapstructure:"batch_size"`
	FlushTimeout time.Duration `mapstructure:"flush_timeout"`

	// Include request/response bodies (may contain sensitive data)
	IncludeBodies bool `mapstructure:"include_bodies"`

	// Fields to redact from logs
	RedactFields []string `mapstructure:"redact_fields"`
}

// BackendConfig configures the audit log storage backend.
type BackendConfig struct {
	// Type of backend (file, database, elasticsearch, etc.)
	Type string `mapstructure:"type"`

	// File backend settings
	FilePath       string `mapstructure:"file_path"`
	MaxSizeMB      int    `mapstructure:"max_size_mb"`
	MaxBackups     int    `mapstructure:"max_backups"`
	MaxAgeDays     int    `mapstructure:"max_age_days"`
	Compress       bool   `mapstructure:"compress"`

	// Database backend settings
	ConnectionString string `mapstructure:"connection_string"`
	TableName        string `mapstructure:"table_name"`

	// Elasticsearch backend settings
	ElasticsearchURL   string `mapstructure:"elasticsearch_url"`
	ElasticsearchIndex string `mapstructure:"elasticsearch_index"`
}

// DefaultConfig returns a default audit configuration.
func DefaultConfig() *Config {
	return &Config{
		Enabled:       false,
		Level:         "write",
		RetentionDays: 90,
		BatchSize:     100,
		FlushTimeout:  5 * time.Second,
		IncludeBodies: false,
		RedactFields: []string{
			"password",
			"api_key",
			"secret",
			"token",
			"authorization",
		},
		Backend: BackendConfig{
			Type:       "file",
			FilePath:   "./logs/audit.log",
			MaxSizeMB:  100,
			MaxBackups: 10,
			MaxAgeDays: 90,
			Compress:   true,
		},
	}
}
