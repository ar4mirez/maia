package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// FileLogger implements audit logging to files with rotation.
type FileLogger struct {
	config     *Config
	logger     *zap.Logger
	file       *os.File
	filePath   string
	mu         sync.Mutex
	batch      []*Event
	batchMu    sync.Mutex
	flushTimer *time.Timer
	timerMu    sync.Mutex
	closed     bool
	closeCh    chan struct{}
}

// NewFileLogger creates a new file-based audit logger.
func NewFileLogger(config *Config, logger *zap.Logger) (*FileLogger, error) {
	if config == nil {
		config = DefaultConfig()
	}

	filePath := config.Backend.FilePath
	if filePath == "" {
		filePath = "./logs/audit.log"
	}

	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create audit log directory: %w", err)
	}

	// Open log file
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open audit log file: %w", err)
	}

	fl := &FileLogger{
		config:   config,
		logger:   logger,
		file:     file,
		filePath: filePath,
		batch:    make([]*Event, 0, config.BatchSize),
		closeCh:  make(chan struct{}),
	}

	// Start background flush timer
	if config.BatchSize > 1 {
		fl.startFlushTimer()
	}

	return fl, nil
}

// Log records an audit event.
func (l *FileLogger) Log(ctx context.Context, event *Event) error {
	if l.closed {
		return fmt.Errorf("audit logger is closed")
	}

	// Generate ID if not set
	if event.ID == "" {
		event.ID = uuid.New().String()
	}

	// Set timestamp if not set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	// Redact sensitive fields
	l.redactEvent(event)

	// Batch mode
	if l.config.BatchSize > 1 {
		l.batchMu.Lock()
		l.batch = append(l.batch, event)
		shouldFlush := len(l.batch) >= l.config.BatchSize
		l.batchMu.Unlock()

		if shouldFlush {
			return l.flush()
		}
		return nil
	}

	// Immediate mode
	return l.writeEvent(event)
}

// Query retrieves audit events matching the filter.
func (l *FileLogger) Query(ctx context.Context, filter *QueryFilter) ([]*Event, error) {
	// For file-based logging, we'll implement a basic file scan
	// In production, this would typically be backed by a database or search engine
	l.mu.Lock()
	defer l.mu.Unlock()

	file, err := os.Open(l.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Event{}, nil
		}
		return nil, fmt.Errorf("failed to open audit log for query: %w", err)
	}
	defer file.Close()

	var events []*Event
	decoder := json.NewDecoder(file)

	for decoder.More() {
		var event Event
		if err := decoder.Decode(&event); err != nil {
			// Skip malformed lines
			continue
		}

		if l.matchesFilter(&event, filter) {
			events = append(events, &event)
		}

		// Apply limit
		if filter != nil && filter.Limit > 0 && len(events) >= filter.Limit {
			break
		}
	}

	return events, nil
}

// Close releases resources.
func (l *FileLogger) Close() error {
	// Stop timer first (with its own lock)
	l.timerMu.Lock()
	if l.flushTimer != nil {
		l.flushTimer.Stop()
		l.flushTimer = nil
	}
	l.timerMu.Unlock()

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return nil
	}

	l.closed = true
	close(l.closeCh)

	// Flush remaining events
	l.flushLocked()

	// Close file
	if l.file != nil {
		return l.file.Close()
	}

	return nil
}

// writeEvent writes a single event to the log file.
func (l *FileLogger) writeEvent(event *Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal audit event: %w", err)
	}

	data = append(data, '\n')
	if _, err := l.file.Write(data); err != nil {
		return fmt.Errorf("failed to write audit event: %w", err)
	}

	return nil
}

// flush writes all batched events to the log file.
func (l *FileLogger) flush() error {
	l.batchMu.Lock()
	if len(l.batch) == 0 {
		l.batchMu.Unlock()
		return nil
	}

	events := l.batch
	l.batch = make([]*Event, 0, l.config.BatchSize)
	l.batchMu.Unlock()

	l.mu.Lock()
	defer l.mu.Unlock()

	return l.writeEventsLocked(events)
}

// flushLocked flushes without acquiring the batch lock (caller must hold it).
func (l *FileLogger) flushLocked() error {
	l.batchMu.Lock()
	if len(l.batch) == 0 {
		l.batchMu.Unlock()
		return nil
	}

	events := l.batch
	l.batch = make([]*Event, 0, l.config.BatchSize)
	l.batchMu.Unlock()

	return l.writeEventsLocked(events)
}

// writeEventsLocked writes multiple events (caller must hold mu).
func (l *FileLogger) writeEventsLocked(events []*Event) error {
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			l.logger.Error("failed to marshal audit event", zap.Error(err))
			continue
		}

		data = append(data, '\n')
		if _, err := l.file.Write(data); err != nil {
			return fmt.Errorf("failed to write audit events: %w", err)
		}
	}

	return nil
}

// startFlushTimer starts the periodic flush timer.
func (l *FileLogger) startFlushTimer() {
	l.timerMu.Lock()
	defer l.timerMu.Unlock()

	if l.closed {
		return
	}

	l.flushTimer = time.AfterFunc(l.config.FlushTimeout, func() {
		l.timerMu.Lock()
		closed := l.closed
		l.timerMu.Unlock()

		if closed {
			return
		}

		if err := l.flush(); err != nil {
			l.logger.Error("failed to flush audit events", zap.Error(err))
		}

		// Restart timer
		l.startFlushTimer()
	})
}

// redactEvent removes sensitive data from the event.
func (l *FileLogger) redactEvent(event *Event) {
	if event.Details == nil {
		return
	}

	for _, field := range l.config.RedactFields {
		if _, ok := event.Details[field]; ok {
			event.Details[field] = "[REDACTED]"
		}
	}

	// Also redact from nested maps
	l.redactMap(event.Details)
}

// redactMap recursively redacts sensitive fields from a map.
func (l *FileLogger) redactMap(m map[string]any) {
	for key, value := range m {
		// Check if key should be redacted
		for _, field := range l.config.RedactFields {
			if key == field {
				m[key] = "[REDACTED]"
				continue
			}
		}

		// Recurse into nested maps
		if nested, ok := value.(map[string]any); ok {
			l.redactMap(nested)
		}
	}
}

// matchesFilter checks if an event matches the query filter.
func (l *FileLogger) matchesFilter(event *Event, filter *QueryFilter) bool {
	if filter == nil || event == nil {
		return filter == nil
	}

	// Time range
	if filter.StartTime != nil && event.Timestamp.Before(*filter.StartTime) {
		return false
	}
	if filter.EndTime != nil && event.Timestamp.After(*filter.EndTime) {
		return false
	}

	// Event types
	if len(filter.EventTypes) > 0 {
		found := false
		for _, t := range filter.EventTypes {
			if event.Type == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Outcomes
	if len(filter.Outcomes) > 0 {
		found := false
		for _, o := range filter.Outcomes {
			if event.Outcome == o {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Actor filters
	if filter.ActorID != "" && event.Actor.ID != filter.ActorID {
		return false
	}
	if filter.ActorType != "" && event.Actor.Type != filter.ActorType {
		return false
	}
	if filter.TenantID != "" && event.Actor.TenantID != filter.TenantID {
		return false
	}
	if filter.APIKeyID != "" && event.Actor.APIKeyID != filter.APIKeyID {
		return false
	}

	// Resource filters
	if filter.ResourceType != "" && event.Resource.Type != filter.ResourceType {
		return false
	}
	if filter.ResourceID != "" && event.Resource.ID != filter.ResourceID {
		return false
	}
	if filter.Namespace != "" && event.Resource.Namespace != filter.Namespace {
		return false
	}

	// Request filters
	if filter.RequestID != "" && event.Request.RequestID != filter.RequestID {
		return false
	}
	if filter.ClientIP != "" && event.Request.ClientIP != filter.ClientIP {
		return false
	}

	return true
}

// Verify interface compliance
var _ Logger = (*FileLogger)(nil)
