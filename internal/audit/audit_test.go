package audit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewFileLogger(t *testing.T) {
	tempDir := t.TempDir()
	config := &Config{
		Enabled:   true,
		BatchSize: 1,
		Backend: BackendConfig{
			FilePath: filepath.Join(tempDir, "audit.log"),
		},
		RedactFields: []string{"password", "secret"},
	}

	logger, err := NewFileLogger(config, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, logger)

	defer logger.Close()

	// Verify file was created
	_, err = os.Stat(config.Backend.FilePath)
	assert.NoError(t, err)
}

func TestFileLogger_Log(t *testing.T) {
	tempDir := t.TempDir()
	config := &Config{
		Enabled:   true,
		BatchSize: 1,
		Backend: BackendConfig{
			FilePath: filepath.Join(tempDir, "audit.log"),
		},
		RedactFields: []string{"password", "secret"},
	}

	logger, err := NewFileLogger(config, zap.NewNop())
	require.NoError(t, err)
	defer logger.Close()

	// Log an event
	event := &Event{
		Type:    EventMemoryCreate,
		Outcome: OutcomeSuccess,
		Actor: Actor{
			Type:     "user",
			ID:       "user-123",
			TenantID: "tenant-abc",
		},
		Resource: Resource{
			Type:      "memory",
			ID:        "mem-456",
			Namespace: "default",
		},
		Request: RequestInfo{
			Method:    "POST",
			Path:      "/api/v1/memories",
			ClientIP:  "192.168.1.1",
			RequestID: "req-789",
		},
		Details: map[string]any{
			"content_length": 100,
		},
		DurationMs: 50,
	}

	err = logger.Log(context.Background(), event)
	require.NoError(t, err)

	// Read back the log file
	data, err := os.ReadFile(config.Backend.FilePath)
	require.NoError(t, err)

	var logged Event
	err = json.Unmarshal(data, &logged)
	require.NoError(t, err)

	assert.Equal(t, EventMemoryCreate, logged.Type)
	assert.Equal(t, OutcomeSuccess, logged.Outcome)
	assert.Equal(t, "user-123", logged.Actor.ID)
	assert.Equal(t, "mem-456", logged.Resource.ID)
	assert.NotEmpty(t, logged.ID)
	assert.NotZero(t, logged.Timestamp)
}

func TestFileLogger_Redaction(t *testing.T) {
	tempDir := t.TempDir()
	config := &Config{
		Enabled:      true,
		BatchSize:    1,
		RedactFields: []string{"password", "secret", "api_key"},
		Backend: BackendConfig{
			FilePath: filepath.Join(tempDir, "audit.log"),
		},
	}

	logger, err := NewFileLogger(config, zap.NewNop())
	require.NoError(t, err)
	defer logger.Close()

	// Log an event with sensitive data
	event := &Event{
		Type:    EventAPIKeyCreate,
		Outcome: OutcomeSuccess,
		Details: map[string]any{
			"name":     "my-key",
			"password": "super-secret-password",
			"secret":   "another-secret",
			"api_key":  "maia_abc123",
		},
	}

	err = logger.Log(context.Background(), event)
	require.NoError(t, err)

	// Read back and verify redaction
	data, err := os.ReadFile(config.Backend.FilePath)
	require.NoError(t, err)

	var logged Event
	err = json.Unmarshal(data, &logged)
	require.NoError(t, err)

	assert.Equal(t, "my-key", logged.Details["name"])
	assert.Equal(t, "[REDACTED]", logged.Details["password"])
	assert.Equal(t, "[REDACTED]", logged.Details["secret"])
	assert.Equal(t, "[REDACTED]", logged.Details["api_key"])
}

func TestFileLogger_BatchedLogging(t *testing.T) {
	tempDir := t.TempDir()
	config := &Config{
		Enabled:      true,
		BatchSize:    5,
		FlushTimeout: 100 * time.Millisecond,
		Backend: BackendConfig{
			FilePath: filepath.Join(tempDir, "audit.log"),
		},
	}

	logger, err := NewFileLogger(config, zap.NewNop())
	require.NoError(t, err)

	// Log 3 events (less than batch size)
	for i := 0; i < 3; i++ {
		event := &Event{
			Type:    EventMemoryRead,
			Outcome: OutcomeSuccess,
		}
		err = logger.Log(context.Background(), event)
		require.NoError(t, err)
	}

	// File should be empty or minimal (batched)
	data, err := os.ReadFile(config.Backend.FilePath)
	require.NoError(t, err)
	assert.Empty(t, data, "events should be batched")

	// Wait for flush timeout
	time.Sleep(200 * time.Millisecond)

	// Now file should have content
	data, err = os.ReadFile(config.Backend.FilePath)
	require.NoError(t, err)
	assert.NotEmpty(t, data, "events should be flushed after timeout")

	logger.Close()
}

func TestFileLogger_Query(t *testing.T) {
	tempDir := t.TempDir()
	config := &Config{
		Enabled:   true,
		BatchSize: 1,
		Backend: BackendConfig{
			FilePath: filepath.Join(tempDir, "audit.log"),
		},
	}

	logger, err := NewFileLogger(config, zap.NewNop())
	require.NoError(t, err)
	defer logger.Close()

	// Log multiple events
	events := []*Event{
		{Type: EventMemoryCreate, Outcome: OutcomeSuccess, Actor: Actor{TenantID: "tenant-a"}},
		{Type: EventMemoryRead, Outcome: OutcomeSuccess, Actor: Actor{TenantID: "tenant-a"}},
		{Type: EventMemoryDelete, Outcome: OutcomeFailure, Actor: Actor{TenantID: "tenant-b"}},
		{Type: EventTenantCreate, Outcome: OutcomeSuccess, Actor: Actor{TenantID: "tenant-a"}},
	}

	for _, e := range events {
		err = logger.Log(context.Background(), e)
		require.NoError(t, err)
	}

	// Query all events
	results, err := logger.Query(context.Background(), nil)
	require.NoError(t, err)
	assert.Len(t, results, 4)

	// Query by tenant
	results, err = logger.Query(context.Background(), &QueryFilter{
		TenantID: "tenant-a",
	})
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Query by event type
	results, err = logger.Query(context.Background(), &QueryFilter{
		EventTypes: []EventType{EventMemoryCreate},
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)

	// Query by outcome
	results, err = logger.Query(context.Background(), &QueryFilter{
		Outcomes: []Outcome{OutcomeFailure},
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, EventMemoryDelete, results[0].Type)

	// Query with limit
	results, err = logger.Query(context.Background(), &QueryFilter{
		Limit: 2,
	})
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestEventTypes(t *testing.T) {
	// Verify event type constants
	tests := []struct {
		eventType EventType
		expected  string
	}{
		{EventMemoryCreate, "memory.create"},
		{EventMemoryRead, "memory.read"},
		{EventMemoryUpdate, "memory.update"},
		{EventMemoryDelete, "memory.delete"},
		{EventMemorySearch, "memory.search"},
		{EventNamespaceCreate, "namespace.create"},
		{EventContextAssemble, "context.assemble"},
		{EventTenantCreate, "tenant.create"},
		{EventAPIKeyCreate, "apikey.create"},
		{EventAuthSuccess, "auth.success"},
		{EventAuthFailure, "auth.failure"},
		{EventSystemStartup, "system.startup"},
	}

	for _, tt := range tests {
		t.Run(string(tt.eventType), func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.eventType))
		})
	}
}

func TestOutcomes(t *testing.T) {
	assert.Equal(t, "success", string(OutcomeSuccess))
	assert.Equal(t, "failure", string(OutcomeFailure))
	assert.Equal(t, "denied", string(OutcomeDenied))
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.False(t, config.Enabled)
	assert.Equal(t, "write", config.Level)
	assert.Equal(t, 90, config.RetentionDays)
	assert.Equal(t, 100, config.BatchSize)
	assert.Equal(t, "file", config.Backend.Type)
	assert.Contains(t, config.RedactFields, "password")
	assert.Contains(t, config.RedactFields, "api_key")
}

func TestInferEventType(t *testing.T) {
	tests := []struct {
		method   string
		path     string
		expected EventType
	}{
		{"POST", "/api/v1/memories", EventMemoryCreate},
		{"GET", "/api/v1/memories/123", EventMemoryRead},
		{"PUT", "/api/v1/memories/123", EventMemoryUpdate},
		{"DELETE", "/api/v1/memories/123", EventMemoryDelete},
		{"POST", "/api/v1/search", EventMemorySearch},
		{"POST", "/api/v1/context", EventContextAssemble},
		{"POST", "/api/v1/namespaces", EventNamespaceCreate},
		{"GET", "/api/v1/namespaces", EventNamespaceList},
		{"GET", "/api/v1/namespaces/default", EventNamespaceRead},
		{"POST", "/admin/tenants", EventTenantCreate},
		{"POST", "/admin/apikeys", EventAPIKeyCreate},
	}

	for _, tt := range tests {
		t.Run(tt.method+"_"+tt.path, func(t *testing.T) {
			result := inferEventType(tt.method, tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInferOutcome(t *testing.T) {
	tests := []struct {
		statusCode int
		expected   Outcome
	}{
		{200, OutcomeSuccess},
		{201, OutcomeSuccess},
		{204, OutcomeSuccess},
		{400, OutcomeFailure},
		{401, OutcomeDenied},
		{403, OutcomeDenied},
		{404, OutcomeFailure},
		{500, OutcomeFailure},
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.statusCode)), func(t *testing.T) {
			result := inferOutcome(tt.statusCode)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestShouldLog(t *testing.T) {
	tests := []struct {
		method   string
		level    string
		expected bool
	}{
		{"GET", "all", true},
		{"POST", "all", true},
		{"GET", "write", false},
		{"POST", "write", true},
		{"PUT", "write", true},
		{"DELETE", "write", true},
		{"GET", "admin", false},
		{"POST", "admin", false},
	}

	for _, tt := range tests {
		t.Run(tt.method+"_"+tt.level, func(t *testing.T) {
			result := shouldLog(tt.method, tt.level)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEventMarshalJSON(t *testing.T) {
	event := &Event{
		ID:        "test-id",
		Timestamp: time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC),
		Type:      EventMemoryCreate,
		Outcome:   OutcomeSuccess,
	}

	data, err := json.Marshal(event)
	require.NoError(t, err)

	// Verify timestamp is in RFC3339 format
	assert.Contains(t, string(data), "2026-01-20T12:00:00")
}
