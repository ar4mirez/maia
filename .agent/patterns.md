# MAIA Coding Patterns

> **Purpose**: Document established coding conventions and patterns used throughout MAIA
>
> **Last Updated**: 2026-01-19

---

## Table of Contents

- [Package Structure](#package-structure)
- [Error Handling](#error-handling)
- [Testing Patterns](#testing-patterns)
- [HTTP Handlers](#http-handlers)
- [Storage Layer](#storage-layer)
- [Functional Options](#functional-options)
- [Context Usage](#context-usage)
- [Logging](#logging)

---

## Package Structure

### Interface Definition Pattern

Define interfaces in a central `types.go` file, implementations in separate files.

```go
// internal/storage/types.go
package storage

// Store defines the storage interface.
type Store interface {
    CreateMemory(ctx context.Context, input *CreateMemoryInput) (*Memory, error)
    GetMemory(ctx context.Context, id string) (*Memory, error)
    // ...
}

// internal/storage/badger/store.go
package badger

// Store implements storage.Store using BadgerDB.
type Store struct {
    db     *badger.DB
    mu     sync.RWMutex
    closed bool
}
```

**Why**: Separates contract from implementation, enables easy mocking and testing.

---

## Error Handling

### Custom Error Types

Define typed errors for domain-specific error handling.

```go
// internal/storage/errors.go
package storage

import "errors"

var (
    ErrNotFound      = errors.New("not found")
    ErrAlreadyExists = errors.New("already exists")
    ErrInvalidInput  = errors.New("invalid input")
)

// Usage in implementation
func (s *Store) GetMemory(ctx context.Context, id string) (*Memory, error) {
    mem, err := s.get(id)
    if err != nil {
        if errors.Is(err, badger.ErrKeyNotFound) {
            return nil, storage.ErrNotFound
        }
        return nil, fmt.Errorf("failed to get memory: %w", err)
    }
    return mem, nil
}

// Checking errors
if errors.Is(err, storage.ErrNotFound) {
    c.JSON(http.StatusNotFound, ErrorResponse{Error: "memory not found"})
    return
}
```

**Why**: Enables consistent error handling across layers and meaningful HTTP status codes.

### Error Wrapping

Always wrap errors with context using `fmt.Errorf` and `%w`.

```go
// Good
return nil, fmt.Errorf("failed to create memory: %w", err)

// Bad
return nil, err
return nil, fmt.Errorf("failed to create memory: %v", err)  // loses error chain
```

**Why**: Preserves error chain for `errors.Is()` and `errors.As()` while adding context.

---

## Testing Patterns

### Test Setup/Cleanup Pattern

Use helper functions that return cleanup functions.

```go
func setupTestStore(t *testing.T) (*Store, func()) {
    t.Helper()

    dir, err := os.MkdirTemp("", "maia-test-*")
    require.NoError(t, err)

    store, err := NewWithPath(dir)
    require.NoError(t, err)

    cleanup := func() {
        store.Close()
        os.RemoveAll(dir)
    }

    return store, cleanup
}

func TestStore_CreateMemory(t *testing.T) {
    store, cleanup := setupTestStore(t)
    defer cleanup()

    // test code...
}
```

**Why**: Ensures proper resource cleanup, reduces boilerplate, enables parallel tests.

### Table-Driven Tests

Use table-driven tests for testing multiple scenarios.

```go
func TestStore_CreateMemory_Validation(t *testing.T) {
    store, cleanup := setupTestStore(t)
    defer cleanup()

    ctx := context.Background()

    tests := []struct {
        name    string
        input   *storage.CreateMemoryInput
        wantErr bool
        errMsg  string
    }{
        {
            name:    "nil input",
            input:   nil,
            wantErr: true,
            errMsg:  "input cannot be nil",
        },
        {
            name: "empty content",
            input: &storage.CreateMemoryInput{
                Namespace: "test",
                Content:   "",
            },
            wantErr: true,
            errMsg:  "content is required",
        },
        {
            name: "valid input",
            input: &storage.CreateMemoryInput{
                Namespace: "test",
                Content:   "Valid content",
                Type:      storage.MemoryTypeSemantic,
            },
            wantErr: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mem, err := store.CreateMemory(ctx, tt.input)
            if tt.wantErr {
                require.Error(t, err)
                if tt.errMsg != "" {
                    assert.Contains(t, err.Error(), tt.errMsg)
                }
                return
            }
            require.NoError(t, err)
            assert.NotEmpty(t, mem.ID)
        })
    }
}
```

**Why**: Clear test structure, easy to add new test cases, self-documenting.

### HTTP Handler Tests

Use httptest for handler testing.

```go
func TestHandler_CreateMemory(t *testing.T) {
    store, cleanup := setupTestStore(t)
    defer cleanup()

    server := NewServer(store, nil, nil, nil, nil)
    router := server.Router()

    body := `{"namespace": "test", "content": "Test memory"}`
    req := httptest.NewRequest(http.MethodPost, "/v1/memories", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()

    router.ServeHTTP(w, req)

    assert.Equal(t, http.StatusCreated, w.Code)

    var resp storage.Memory
    err := json.Unmarshal(w.Body.Bytes(), &resp)
    require.NoError(t, err)
    assert.NotEmpty(t, resp.ID)
}
```

**Why**: Tests full request/response cycle, catches routing and middleware issues.

### Mock Store Pattern

Implement mock stores for testing handlers without database.

```go
type mockStore struct {
    memories   map[string]*storage.Memory
    namespaces map[string]*storage.Namespace
    mu         sync.RWMutex
}

func newMockStore() *mockStore {
    return &mockStore{
        memories:   make(map[string]*storage.Memory),
        namespaces: make(map[string]*storage.Namespace),
    }
}

func (m *mockStore) CreateMemory(ctx context.Context, input *storage.CreateMemoryInput) (*storage.Memory, error) {
    m.mu.Lock()
    defer m.mu.Unlock()

    mem := &storage.Memory{
        ID:        uuid.New().String(),
        Namespace: input.Namespace,
        Content:   input.Content,
        // ...
    }
    m.memories[mem.ID] = mem
    return mem, nil
}
```

**Why**: Fast tests, no I/O, controllable behavior for edge cases.

---

## HTTP Handlers

### Request/Response Types

Define explicit types for all API requests and responses.

```go
// CreateMemoryRequest represents the request to create a memory.
type CreateMemoryRequest struct {
    Namespace  string                 `json:"namespace" binding:"required"`
    Content    string                 `json:"content" binding:"required"`
    Type       storage.MemoryType     `json:"type"`
    Metadata   map[string]interface{} `json:"metadata,omitempty"`
    Tags       []string               `json:"tags,omitempty"`
}

// ErrorResponse represents an API error response.
type ErrorResponse struct {
    Error   string `json:"error"`
    Code    string `json:"code,omitempty"`
    Details string `json:"details,omitempty"`
}

// ListResponse represents a paginated list response.
type ListResponse struct {
    Data   interface{} `json:"data"`
    Count  int         `json:"count"`
    Offset int         `json:"offset"`
    Limit  int         `json:"limit"`
}
```

**Why**: Type safety, clear API contracts, automatic validation with Gin bindings.

### Handler Structure

Follow consistent handler patterns with validation, operation, response.

```go
func (s *Server) createMemoryHandler(c *gin.Context) {
    // 1. Parse and validate request
    var req CreateMemoryRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, ErrorResponse{
            Error: "invalid request body",
            Details: err.Error(),
        })
        return
    }

    // 2. Perform operation
    input := &storage.CreateMemoryInput{
        Namespace: req.Namespace,
        Content:   req.Content,
        Type:      req.Type,
        // ...
    }

    mem, err := s.store.CreateMemory(c.Request.Context(), input)
    if err != nil {
        s.handleError(c, err)
        return
    }

    // 3. Return response
    c.JSON(http.StatusCreated, mem)
}
```

**Why**: Predictable structure, easy to review and maintain.

### Error Response Helper

Centralize error handling for consistent responses.

```go
func (s *Server) handleError(c *gin.Context, err error) {
    switch {
    case errors.Is(err, storage.ErrNotFound):
        c.JSON(http.StatusNotFound, ErrorResponse{Error: "not found"})
    case errors.Is(err, storage.ErrAlreadyExists):
        c.JSON(http.StatusConflict, ErrorResponse{Error: "already exists"})
    case errors.Is(err, storage.ErrInvalidInput):
        c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
    default:
        s.logger.Error("internal error", zap.Error(err))
        c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
    }
}
```

**Why**: Consistent error responses, DRY error handling.

---

## Storage Layer

### Key Prefix Pattern

Use prefixes to organize data in key-value stores.

```go
const (
    prefixMemory    = "mem:"    // mem:{id}
    prefixNamespace = "ns:"     // ns:{id}
    prefixNSByName  = "nsn:"    // nsn:{name} -> id (index)
    prefixMemByNS   = "mns:"    // mns:{namespace}:{id} (index)
)

func memoryKey(id string) []byte {
    return []byte(prefixMemory + id)
}

func memoryByNamespaceKey(namespace, id string) []byte {
    return []byte(prefixMemByNS + namespace + ":" + id)
}
```

**Why**: Enables efficient range scans, logical data organization, prevents key collisions.

### Transaction Pattern

Use transactions for multi-key operations.

```go
func (s *Store) CreateMemory(ctx context.Context, input *CreateMemoryInput) (*Memory, error) {
    // Validate input...

    err := s.db.Update(func(txn *badger.Txn) error {
        // Store the memory
        data, err := json.Marshal(mem)
        if err != nil {
            return err
        }
        if err := txn.Set(memoryKey(mem.ID), data); err != nil {
            return err
        }

        // Update the namespace index
        return txn.Set(memoryByNamespaceKey(mem.Namespace, mem.ID), nil)
    })

    if err != nil {
        return nil, fmt.Errorf("failed to create memory: %w", err)
    }

    return mem, nil
}
```

**Why**: Atomic operations, data consistency.

---

## Functional Options

### Client Configuration

Use functional options for flexible configuration.

```go
// ClientOption is a function that configures a Client.
type ClientOption func(*Client)

// WithBaseURL sets the base URL for the client.
func WithBaseURL(baseURL string) ClientOption {
    return func(c *Client) {
        c.baseURL = baseURL
    }
}

// WithTimeout sets the HTTP client timeout.
func WithTimeout(timeout time.Duration) ClientOption {
    return func(c *Client) {
        c.httpClient.Timeout = timeout
    }
}

// WithAPIKey sets the API key for authentication.
func WithAPIKey(apiKey string) ClientOption {
    return func(c *Client) {
        c.headers["X-API-Key"] = apiKey
    }
}

// New creates a new client with the given options.
func New(opts ...ClientOption) *Client {
    c := &Client{
        baseURL: DefaultBaseURL,
        httpClient: &http.Client{Timeout: DefaultTimeout},
        headers: make(map[string]string),
    }

    for _, opt := range opts {
        opt(c)
    }

    return c
}

// Usage
client := maia.New(
    maia.WithBaseURL("http://localhost:8080"),
    maia.WithAPIKey("secret-key"),
    maia.WithTimeout(10 * time.Second),
)
```

**Why**: Flexible configuration, sensible defaults, backward compatible API.

### Retrieval Options

Use options pattern for complex function calls.

```go
// RecallOption configures a Recall request.
type RecallOption func(*recallOptions)

type recallOptions struct {
    namespace    string
    tokenBudget  int
    systemPrompt string
    minScore     float64
}

func WithNamespace(ns string) RecallOption {
    return func(o *recallOptions) {
        o.namespace = ns
    }
}

func WithTokenBudget(budget int) RecallOption {
    return func(o *recallOptions) {
        o.tokenBudget = budget
    }
}

func (c *Client) Recall(ctx context.Context, query string, opts ...RecallOption) (*ContextResponse, error) {
    o := &recallOptions{
        namespace:   "default",
        tokenBudget: 4000,
    }
    for _, opt := range opts {
        opt(o)
    }

    // Use options...
}
```

**Why**: Clean API, optional parameters without nil checks.

---

## Context Usage

### Passing Context

Always pass context as the first parameter.

```go
// Good
func (s *Store) GetMemory(ctx context.Context, id string) (*Memory, error)

// Bad
func (s *Store) GetMemory(id string) (*Memory, error)
func (s *Store) GetMemory(id string, ctx context.Context) (*Memory, error)
```

**Why**: Enables cancellation, timeouts, and request-scoped values.

### Context in Handlers

Use `c.Request.Context()` from Gin handlers.

```go
func (s *Server) getMemoryHandler(c *gin.Context) {
    id := c.Param("id")

    mem, err := s.store.GetMemory(c.Request.Context(), id)
    if err != nil {
        s.handleError(c, err)
        return
    }

    c.JSON(http.StatusOK, mem)
}
```

**Why**: Respects client cancellation, enables per-request timeouts.

---

## Logging

### Structured Logging with Zap

Use structured logging with meaningful fields.

```go
// Logger setup
logger, _ := zap.NewProduction()
defer logger.Sync()

// Logging with context
logger.Info("memory created",
    zap.String("id", mem.ID),
    zap.String("namespace", mem.Namespace),
    zap.Int("content_length", len(mem.Content)),
)

// Error logging
logger.Error("failed to create memory",
    zap.Error(err),
    zap.String("namespace", input.Namespace),
)
```

**Why**: Structured logs are searchable, parseable, and provide consistent format.

### Request Logging Middleware

Log all HTTP requests with timing.

```go
func requestLogger(logger *zap.Logger) gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        path := c.Request.URL.Path

        c.Next()

        logger.Info("request",
            zap.String("method", c.Request.Method),
            zap.String("path", path),
            zap.Int("status", c.Writer.Status()),
            zap.Duration("latency", time.Since(start)),
            zap.String("client_ip", c.ClientIP()),
        )
    }
}
```

**Why**: Observability, debugging, performance monitoring.

---

## See Also

- [.agent/project.md](project.md) - Architecture and technology stack
- [.agent/rfd/](rfd/) - Architectural decision records
- [CLAUDE.md](../CLAUDE.md) - AI development guardrails
