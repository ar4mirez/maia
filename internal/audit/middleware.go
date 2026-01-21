package audit

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ContextKey is the type for context keys.
type ContextKey string

const (
	// ContextKeyAuditEvent is the key for storing the audit event in context.
	ContextKeyAuditEvent ContextKey = "audit_event"
	// ContextKeyRequestID is the key for the request ID.
	ContextKeyRequestID ContextKey = "request_id"
)

// MiddlewareConfig configures the audit middleware.
type MiddlewareConfig struct {
	// Logger is the audit logger to use
	Logger Logger

	// Level controls which events are logged (all, write, admin)
	Level string

	// SkipPaths are paths that should not be audited
	SkipPaths []string

	// GetTenantID extracts tenant ID from the request
	GetTenantID func(*gin.Context) string

	// GetActorID extracts actor ID from the request
	GetActorID func(*gin.Context) string

	// GetAPIKeyID extracts API key ID from the request
	GetAPIKeyID func(*gin.Context) string
}

// Middleware creates a Gin middleware for audit logging.
func Middleware(config MiddlewareConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if path should be skipped
		for _, path := range config.SkipPaths {
			if strings.HasPrefix(c.Request.URL.Path, path) {
				c.Next()
				return
			}
		}

		// Check if event should be logged based on level
		if !shouldLog(c.Request.Method, config.Level) {
			c.Next()
			return
		}

		// Generate request ID
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}
		c.Set(string(ContextKeyRequestID), requestID)

		// Start timer
		start := time.Now()

		// Create initial event
		event := &Event{
			ID:        uuid.New().String(),
			Timestamp: start.UTC(),
			Type:      inferEventType(c.Request.Method, c.Request.URL.Path),
			Actor: Actor{
				Type:      "user",
				UserAgent: c.Request.UserAgent(),
			},
			Request: RequestInfo{
				Method:    c.Request.Method,
				Path:      c.Request.URL.Path,
				ClientIP:  c.ClientIP(),
				RequestID: requestID,
			},
			Details: make(map[string]any),
		}

		// Extract tenant ID
		if config.GetTenantID != nil {
			tenantID := config.GetTenantID(c)
			event.Actor.TenantID = tenantID
			event.Resource.TenantID = tenantID
		}

		// Extract actor ID
		if config.GetActorID != nil {
			event.Actor.ID = config.GetActorID(c)
		}

		// Extract API key ID
		if config.GetAPIKeyID != nil {
			event.Actor.APIKeyID = config.GetAPIKeyID(c)
		}

		// Store event in context for handlers to enrich
		c.Set(string(ContextKeyAuditEvent), event)

		// Process request
		c.Next()

		// Calculate duration
		event.DurationMs = time.Since(start).Milliseconds()

		// Set outcome based on status code
		statusCode := c.Writer.Status()
		event.Outcome = inferOutcome(statusCode)

		// Add error info if failed
		if statusCode >= 400 {
			event.Error = &ErrorInfo{
				Code:    http.StatusText(statusCode),
				Message: c.Errors.String(),
			}
		}

		// Log the event
		if config.Logger != nil {
			ctx := context.Background()
			if err := config.Logger.Log(ctx, event); err != nil {
				// Don't fail the request, just log the error
				c.Error(err)
			}
		}
	}
}

// GetAuditEvent retrieves the audit event from the Gin context.
func GetAuditEvent(c *gin.Context) *Event {
	if event, exists := c.Get(string(ContextKeyAuditEvent)); exists {
		if e, ok := event.(*Event); ok {
			return e
		}
	}
	return nil
}

// EnrichEvent adds details to the current audit event.
func EnrichEvent(c *gin.Context, key string, value any) {
	if event := GetAuditEvent(c); event != nil {
		event.Details[key] = value
	}
}

// SetResource sets the resource information on the audit event.
func SetResource(c *gin.Context, resourceType, resourceID, namespace string) {
	if event := GetAuditEvent(c); event != nil {
		event.Resource.Type = resourceType
		event.Resource.ID = resourceID
		event.Resource.Namespace = namespace
	}
}

// SetEventType overrides the inferred event type.
func SetEventType(c *gin.Context, eventType EventType) {
	if event := GetAuditEvent(c); event != nil {
		event.Type = eventType
	}
}

// shouldLog determines if an event should be logged based on the level.
func shouldLog(method, level string) bool {
	switch level {
	case "all":
		return true
	case "write":
		// Log all write operations
		return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
	case "admin":
		// Only log admin operations (would need path-based detection)
		return false
	default:
		return true
	}
}

// inferEventType attempts to determine the event type from the request.
func inferEventType(method, path string) EventType {
	parts := strings.Split(strings.Trim(path, "/"), "/")

	// Memory endpoints
	if containsSegment(parts, "memories") {
		switch method {
		case http.MethodPost:
			return EventMemoryCreate
		case http.MethodGet:
			return EventMemoryRead
		case http.MethodPut, http.MethodPatch:
			return EventMemoryUpdate
		case http.MethodDelete:
			return EventMemoryDelete
		}
	}

	// Search endpoint
	if containsSegment(parts, "search") {
		return EventMemorySearch
	}

	// Context endpoint
	if containsSegment(parts, "context") {
		return EventContextAssemble
	}

	// Namespace endpoints
	if containsSegment(parts, "namespaces") {
		switch method {
		case http.MethodPost:
			return EventNamespaceCreate
		case http.MethodGet:
			// Check if there's a namespace ID after "namespaces"
			for i, p := range parts {
				if p == "namespaces" && i+1 < len(parts) && parts[i+1] != "" {
					return EventNamespaceRead
				}
			}
			return EventNamespaceList
		case http.MethodPut, http.MethodPatch:
			return EventNamespaceUpdate
		case http.MethodDelete:
			return EventNamespaceDelete
		}
	}

	// Tenant endpoints
	if containsSegment(parts, "tenants") {
		switch method {
		case http.MethodPost:
			return EventTenantCreate
		case http.MethodPut, http.MethodPatch:
			return EventTenantUpdate
		case http.MethodDelete:
			return EventTenantDelete
		}
	}

	// API key endpoints
	if containsSegment(parts, "apikeys") || containsSegment(parts, "api-keys") {
		switch method {
		case http.MethodPost:
			return EventAPIKeyCreate
		case http.MethodDelete:
			return EventAPIKeyRevoke
		}
	}

	// Default based on method
	switch method {
	case http.MethodGet:
		return EventMemoryRead
	case http.MethodPost:
		return EventMemoryCreate
	case http.MethodPut, http.MethodPatch:
		return EventMemoryUpdate
	case http.MethodDelete:
		return EventMemoryDelete
	default:
		return EventMemoryRead
	}
}

// inferOutcome determines the outcome from the HTTP status code.
func inferOutcome(statusCode int) Outcome {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return OutcomeSuccess
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return OutcomeDenied
	default:
		return OutcomeFailure
	}
}

// containsSegment checks if a path segment exists.
func containsSegment(parts []string, segment string) bool {
	for _, p := range parts {
		if p == segment {
			return true
		}
	}
	return false
}
