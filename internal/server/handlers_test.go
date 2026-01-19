package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ar4mirez/maia/internal/config"
	mcontext "github.com/ar4mirez/maia/internal/context"
	"github.com/ar4mirez/maia/internal/query"
	"github.com/ar4mirez/maia/internal/storage"
)

// mockStore implements storage.Store for testing.
type mockStore struct {
	memories   map[string]*storage.Memory
	namespaces map[string]*storage.Namespace
	memCounter int

	// Control error responses
	errOnCreate error
	errOnGet    error
	errOnUpdate error
	errOnDelete error
	errOnSearch error
	errOnStats  error
}

func newMockStore() *mockStore {
	return &mockStore{
		memories:   make(map[string]*storage.Memory),
		namespaces: make(map[string]*storage.Namespace),
	}
}

func (m *mockStore) CreateMemory(ctx context.Context, input *storage.CreateMemoryInput) (*storage.Memory, error) {
	if m.errOnCreate != nil {
		return nil, m.errOnCreate
	}
	m.memCounter++
	mem := &storage.Memory{
		ID:         fmt.Sprintf("mem_%s_%d", input.Namespace, m.memCounter),
		Namespace:  input.Namespace,
		Content:    input.Content,
		Type:       input.Type,
		Metadata:   input.Metadata,
		Tags:       input.Tags,
		Confidence: input.Confidence,
		Source:     input.Source,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		AccessedAt: time.Now(),
	}
	m.memories[mem.ID] = mem
	return mem, nil
}

func (m *mockStore) GetMemory(ctx context.Context, id string) (*storage.Memory, error) {
	if m.errOnGet != nil {
		return nil, m.errOnGet
	}
	mem, ok := m.memories[id]
	if !ok {
		return nil, &storage.ErrNotFound{Type: "memory", ID: id}
	}
	return mem, nil
}

func (m *mockStore) UpdateMemory(ctx context.Context, id string, input *storage.UpdateMemoryInput) (*storage.Memory, error) {
	if m.errOnUpdate != nil {
		return nil, m.errOnUpdate
	}
	mem, ok := m.memories[id]
	if !ok {
		return nil, &storage.ErrNotFound{Type: "memory", ID: id}
	}
	if input.Content != nil {
		mem.Content = *input.Content
	}
	if input.Metadata != nil {
		mem.Metadata = input.Metadata
	}
	if input.Tags != nil {
		mem.Tags = input.Tags
	}
	if input.Confidence != nil {
		mem.Confidence = *input.Confidence
	}
	mem.UpdatedAt = time.Now()
	return mem, nil
}

func (m *mockStore) DeleteMemory(ctx context.Context, id string) error {
	if m.errOnDelete != nil {
		return m.errOnDelete
	}
	if _, ok := m.memories[id]; !ok {
		return &storage.ErrNotFound{Type: "memory", ID: id}
	}
	delete(m.memories, id)
	return nil
}

func (m *mockStore) ListMemories(ctx context.Context, namespace string, opts *storage.ListOptions) ([]*storage.Memory, error) {
	var results []*storage.Memory
	for _, mem := range m.memories {
		if namespace == "" || mem.Namespace == namespace {
			results = append(results, mem)
		}
	}
	return results, nil
}

func (m *mockStore) SearchMemories(ctx context.Context, opts *storage.SearchOptions) ([]*storage.SearchResult, error) {
	if m.errOnSearch != nil {
		return nil, m.errOnSearch
	}
	var results []*storage.SearchResult
	for _, mem := range m.memories {
		if opts.Namespace == "" || mem.Namespace == opts.Namespace {
			results = append(results, &storage.SearchResult{
				Memory: mem,
				Score:  0.9,
			})
		}
	}
	return results, nil
}

func (m *mockStore) TouchMemory(ctx context.Context, id string) error {
	mem, ok := m.memories[id]
	if ok {
		mem.AccessedAt = time.Now()
		mem.AccessCount++
	}
	return nil
}

func (m *mockStore) CreateNamespace(ctx context.Context, input *storage.CreateNamespaceInput) (*storage.Namespace, error) {
	if m.errOnCreate != nil {
		return nil, m.errOnCreate
	}
	// Check if already exists
	for _, ns := range m.namespaces {
		if ns.Name == input.Name {
			return nil, &storage.ErrAlreadyExists{Type: "namespace", ID: input.Name}
		}
	}
	ns := &storage.Namespace{
		ID:        "ns_" + input.Name,
		Name:      input.Name,
		Parent:    input.Parent,
		Config:    input.Config,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m.namespaces[ns.ID] = ns
	return ns, nil
}

func (m *mockStore) GetNamespace(ctx context.Context, id string) (*storage.Namespace, error) {
	if m.errOnGet != nil {
		return nil, m.errOnGet
	}
	ns, ok := m.namespaces[id]
	if !ok {
		return nil, &storage.ErrNotFound{Type: "namespace", ID: id}
	}
	return ns, nil
}

func (m *mockStore) GetNamespaceByName(ctx context.Context, name string) (*storage.Namespace, error) {
	if m.errOnGet != nil {
		return nil, m.errOnGet
	}
	for _, ns := range m.namespaces {
		if ns.Name == name {
			return ns, nil
		}
	}
	return nil, &storage.ErrNotFound{Type: "namespace", ID: name}
}

func (m *mockStore) UpdateNamespace(ctx context.Context, id string, cfg *storage.NamespaceConfig) (*storage.Namespace, error) {
	if m.errOnUpdate != nil {
		return nil, m.errOnUpdate
	}
	ns, ok := m.namespaces[id]
	if !ok {
		return nil, &storage.ErrNotFound{Type: "namespace", ID: id}
	}
	ns.Config = *cfg
	ns.UpdatedAt = time.Now()
	return ns, nil
}

func (m *mockStore) DeleteNamespace(ctx context.Context, id string) error {
	if m.errOnDelete != nil {
		return m.errOnDelete
	}
	if _, ok := m.namespaces[id]; !ok {
		return &storage.ErrNotFound{Type: "namespace", ID: id}
	}
	delete(m.namespaces, id)
	return nil
}

func (m *mockStore) ListNamespaces(ctx context.Context, opts *storage.ListOptions) ([]*storage.Namespace, error) {
	var results []*storage.Namespace
	for _, ns := range m.namespaces {
		results = append(results, ns)
	}
	return results, nil
}

func (m *mockStore) BatchCreateMemories(ctx context.Context, inputs []*storage.CreateMemoryInput) ([]*storage.Memory, error) {
	var results []*storage.Memory
	for _, input := range inputs {
		mem, err := m.CreateMemory(ctx, input)
		if err != nil {
			return nil, err
		}
		results = append(results, mem)
	}
	return results, nil
}

func (m *mockStore) BatchDeleteMemories(ctx context.Context, ids []string) error {
	for _, id := range ids {
		if err := m.DeleteMemory(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (m *mockStore) Close() error {
	return nil
}

func (m *mockStore) Stats(ctx context.Context) (*storage.StoreStats, error) {
	if m.errOnStats != nil {
		return nil, m.errOnStats
	}
	return &storage.StoreStats{
		TotalMemories:   int64(len(m.memories)),
		TotalNamespaces: int64(len(m.namespaces)),
	}, nil
}

// testServer creates a server configured for testing.
func testServer(store storage.Store) *Server {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			HTTPPort:       8080,
			RequestTimeout: 30 * time.Second,
			CORSOrigins:    []string{"*"},
		},
		Log: config.LogConfig{
			Level: "debug",
		},
		Memory: config.MemoryConfig{
			DefaultNamespace:   "default",
			DefaultTokenBudget: 4000,
		},
	}

	logger, _ := zap.NewDevelopment()

	return NewWithDeps(cfg, store, logger, nil)
}

// testServerWithDeps creates a server with custom dependencies for testing.
func testServerWithDeps(store storage.Store, deps *ServerDeps) *Server {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			HTTPPort:       8080,
			RequestTimeout: 30 * time.Second,
			CORSOrigins:    []string{"*"},
		},
		Log: config.LogConfig{
			Level: "debug",
		},
		Memory: config.MemoryConfig{
			DefaultNamespace:   "default",
			DefaultTokenBudget: 4000,
		},
	}

	logger, _ := zap.NewDevelopment()

	return NewWithDeps(cfg, store, logger, deps)
}

// performRequest performs an HTTP request and returns the recorder.
func performRequest(router *gin.Engine, method, path string, body interface{}) *httptest.ResponseRecorder {
	var reqBody *bytes.Buffer
	if body != nil {
		data, _ := json.Marshal(body)
		reqBody = bytes.NewBuffer(data)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}

	req, _ := http.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// Health Endpoint Tests

func TestHealthHandler(t *testing.T) {
	store := newMockStore()
	srv := testServer(store)

	w := performRequest(srv.Router(), "GET", "/health", nil)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "healthy", resp["status"])
	assert.Equal(t, "maia", resp["service"])
}

func TestReadyHandler(t *testing.T) {
	t.Run("ready when store accessible", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		w := performRequest(srv.Router(), "GET", "/ready", nil)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "ready", resp["status"])
	})

	t.Run("not ready when store fails", func(t *testing.T) {
		store := newMockStore()
		store.errOnStats = &storage.ErrInvalidInput{Field: "test", Message: "test error"}
		srv := testServer(store)

		w := performRequest(srv.Router(), "GET", "/ready", nil)

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "not ready", resp["status"])
	})
}

// Memory Endpoint Tests

func TestCreateMemory(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		req := CreateMemoryRequest{
			Namespace:  "test-ns",
			Content:    "Test memory content",
			Type:       storage.MemoryTypeEpisodic,
			Confidence: 0.9,
		}

		w := performRequest(srv.Router(), "POST", "/v1/memories", req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var mem storage.Memory
		err := json.Unmarshal(w.Body.Bytes(), &mem)
		require.NoError(t, err)
		assert.Equal(t, "test-ns", mem.Namespace)
		assert.Equal(t, "Test memory content", mem.Content)
	})

	t.Run("invalid request - missing namespace", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		req := map[string]string{
			"content": "Test content",
		}

		w := performRequest(srv.Router(), "POST", "/v1/memories", req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "invalid request body", resp.Error)
	})

	t.Run("invalid request - missing content", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		req := map[string]string{
			"namespace": "test-ns",
		}

		w := performRequest(srv.Router(), "POST", "/v1/memories", req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestGetMemory(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		// Create a memory first
		createReq := CreateMemoryRequest{
			Namespace: "test-ns",
			Content:   "Test content",
		}
		w := performRequest(srv.Router(), "POST", "/v1/memories", createReq)
		require.Equal(t, http.StatusCreated, w.Code)

		var created storage.Memory
		_ = json.Unmarshal(w.Body.Bytes(), &created)

		// Get the memory
		w = performRequest(srv.Router(), "GET", "/v1/memories/"+created.ID, nil)

		assert.Equal(t, http.StatusOK, w.Code)

		var mem storage.Memory
		err := json.Unmarshal(w.Body.Bytes(), &mem)
		require.NoError(t, err)
		assert.Equal(t, created.ID, mem.ID)
	})

	t.Run("not found", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		w := performRequest(srv.Router(), "GET", "/v1/memories/nonexistent", nil)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var resp ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "NOT_FOUND", resp.Code)
	})
}

func TestUpdateMemory(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		// Create a memory first
		createReq := CreateMemoryRequest{
			Namespace: "test-ns",
			Content:   "Original content",
		}
		w := performRequest(srv.Router(), "POST", "/v1/memories", createReq)
		require.Equal(t, http.StatusCreated, w.Code)

		var created storage.Memory
		_ = json.Unmarshal(w.Body.Bytes(), &created)

		// Update the memory
		newContent := "Updated content"
		updateReq := UpdateMemoryRequest{
			Content: &newContent,
		}
		w = performRequest(srv.Router(), "PUT", "/v1/memories/"+created.ID, updateReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var mem storage.Memory
		err := json.Unmarshal(w.Body.Bytes(), &mem)
		require.NoError(t, err)
		assert.Equal(t, "Updated content", mem.Content)
	})

	t.Run("not found", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		newContent := "Updated content"
		updateReq := UpdateMemoryRequest{
			Content: &newContent,
		}
		w := performRequest(srv.Router(), "PUT", "/v1/memories/nonexistent", updateReq)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestDeleteMemory(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		// Create a memory first
		createReq := CreateMemoryRequest{
			Namespace: "test-ns",
			Content:   "Test content",
		}
		w := performRequest(srv.Router(), "POST", "/v1/memories", createReq)
		require.Equal(t, http.StatusCreated, w.Code)

		var created storage.Memory
		_ = json.Unmarshal(w.Body.Bytes(), &created)

		// Delete the memory
		w = performRequest(srv.Router(), "DELETE", "/v1/memories/"+created.ID, nil)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]bool
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["deleted"])

		// Verify it's gone
		w = performRequest(srv.Router(), "GET", "/v1/memories/"+created.ID, nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("not found", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		w := performRequest(srv.Router(), "DELETE", "/v1/memories/nonexistent", nil)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestSearchMemories(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		// Create some memories
		for i := 0; i < 3; i++ {
			createReq := CreateMemoryRequest{
				Namespace: "test-ns",
				Content:   "Test content",
			}
			performRequest(srv.Router(), "POST", "/v1/memories", createReq)
		}

		searchReq := SearchMemoriesRequest{
			Namespace: "test-ns",
			Limit:     10,
		}
		w := performRequest(srv.Router(), "POST", "/v1/memories/search", searchReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp ListResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 3, resp.Count)
	})

	t.Run("default limit", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		searchReq := SearchMemoriesRequest{
			Namespace: "test-ns",
		}
		w := performRequest(srv.Router(), "POST", "/v1/memories/search", searchReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp ListResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 100, resp.Limit)
	})

	t.Run("max limit cap", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		searchReq := SearchMemoriesRequest{
			Limit: 5000, // Exceeds max
		}
		w := performRequest(srv.Router(), "POST", "/v1/memories/search", searchReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp ListResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 1000, resp.Limit)
	})
}

// Namespace Endpoint Tests

func TestCreateNamespace(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		req := CreateNamespaceRequest{
			Name: "test-namespace",
		}

		w := performRequest(srv.Router(), "POST", "/v1/namespaces", req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var ns storage.Namespace
		err := json.Unmarshal(w.Body.Bytes(), &ns)
		require.NoError(t, err)
		assert.Equal(t, "test-namespace", ns.Name)
	})

	t.Run("invalid request - missing name", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		req := map[string]string{}

		w := performRequest(srv.Router(), "POST", "/v1/namespaces", req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("already exists", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		req := CreateNamespaceRequest{
			Name: "test-namespace",
		}

		// Create first
		w := performRequest(srv.Router(), "POST", "/v1/namespaces", req)
		require.Equal(t, http.StatusCreated, w.Code)

		// Try to create again
		w = performRequest(srv.Router(), "POST", "/v1/namespaces", req)
		assert.Equal(t, http.StatusConflict, w.Code)
	})
}

func TestGetNamespace(t *testing.T) {
	t.Run("get by id", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		// Create namespace first
		createReq := CreateNamespaceRequest{
			Name: "test-namespace",
		}
		w := performRequest(srv.Router(), "POST", "/v1/namespaces", createReq)
		require.Equal(t, http.StatusCreated, w.Code)

		var created storage.Namespace
		_ = json.Unmarshal(w.Body.Bytes(), &created)

		// Get by ID
		w = performRequest(srv.Router(), "GET", "/v1/namespaces/"+created.ID, nil)

		assert.Equal(t, http.StatusOK, w.Code)

		var ns storage.Namespace
		err := json.Unmarshal(w.Body.Bytes(), &ns)
		require.NoError(t, err)
		assert.Equal(t, created.ID, ns.ID)
	})

	t.Run("get by name", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		// Create namespace first
		createReq := CreateNamespaceRequest{
			Name: "test-namespace",
		}
		w := performRequest(srv.Router(), "POST", "/v1/namespaces", createReq)
		require.Equal(t, http.StatusCreated, w.Code)

		// Get by name
		w = performRequest(srv.Router(), "GET", "/v1/namespaces/test-namespace", nil)

		assert.Equal(t, http.StatusOK, w.Code)

		var ns storage.Namespace
		err := json.Unmarshal(w.Body.Bytes(), &ns)
		require.NoError(t, err)
		assert.Equal(t, "test-namespace", ns.Name)
	})

	t.Run("not found", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		w := performRequest(srv.Router(), "GET", "/v1/namespaces/nonexistent", nil)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestUpdateNamespace(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		// Create namespace first
		createReq := CreateNamespaceRequest{
			Name: "test-namespace",
		}
		w := performRequest(srv.Router(), "POST", "/v1/namespaces", createReq)
		require.Equal(t, http.StatusCreated, w.Code)

		var created storage.Namespace
		_ = json.Unmarshal(w.Body.Bytes(), &created)

		// Update namespace
		updateReq := UpdateNamespaceRequest{
			Config: storage.NamespaceConfig{
				RetentionDays: 30,
			},
		}
		w = performRequest(srv.Router(), "PUT", "/v1/namespaces/"+created.ID, updateReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var ns storage.Namespace
		err := json.Unmarshal(w.Body.Bytes(), &ns)
		require.NoError(t, err)
		assert.Equal(t, 30, ns.Config.RetentionDays)
	})

	t.Run("not found", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		updateReq := UpdateNamespaceRequest{
			Config: storage.NamespaceConfig{},
		}
		w := performRequest(srv.Router(), "PUT", "/v1/namespaces/nonexistent", updateReq)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestDeleteNamespace(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		// Create namespace first
		createReq := CreateNamespaceRequest{
			Name: "test-namespace",
		}
		w := performRequest(srv.Router(), "POST", "/v1/namespaces", createReq)
		require.Equal(t, http.StatusCreated, w.Code)

		var created storage.Namespace
		_ = json.Unmarshal(w.Body.Bytes(), &created)

		// Delete namespace
		w = performRequest(srv.Router(), "DELETE", "/v1/namespaces/"+created.ID, nil)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]bool
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["deleted"])
	})

	t.Run("not found", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		w := performRequest(srv.Router(), "DELETE", "/v1/namespaces/nonexistent", nil)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestListNamespaces(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		// Create some namespaces
		for i := 0; i < 3; i++ {
			createReq := CreateNamespaceRequest{
				Name: "test-namespace-" + string(rune('a'+i)),
			}
			performRequest(srv.Router(), "POST", "/v1/namespaces", createReq)
		}

		w := performRequest(srv.Router(), "GET", "/v1/namespaces", nil)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp ListResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 3, resp.Count)
	})

	t.Run("with pagination", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		w := performRequest(srv.Router(), "GET", "/v1/namespaces?limit=10&offset=5", nil)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp ListResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 10, resp.Limit)
		assert.Equal(t, 5, resp.Offset)
	})
}

func TestListNamespaceMemories(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		// Create namespace first
		nsReq := CreateNamespaceRequest{
			Name: "test-namespace",
		}
		w := performRequest(srv.Router(), "POST", "/v1/namespaces", nsReq)
		require.Equal(t, http.StatusCreated, w.Code)

		var ns storage.Namespace
		_ = json.Unmarshal(w.Body.Bytes(), &ns)

		// Create memories in the namespace
		for i := 0; i < 3; i++ {
			memReq := CreateMemoryRequest{
				Namespace: "test-namespace",
				Content:   "Test content",
			}
			performRequest(srv.Router(), "POST", "/v1/memories", memReq)
		}

		// List memories
		w = performRequest(srv.Router(), "GET", "/v1/namespaces/"+ns.ID+"/memories", nil)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp ListResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 3, resp.Count)
	})

	t.Run("get by name", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		// Create namespace first
		nsReq := CreateNamespaceRequest{
			Name: "test-namespace",
		}
		w := performRequest(srv.Router(), "POST", "/v1/namespaces", nsReq)
		require.Equal(t, http.StatusCreated, w.Code)

		// List memories by name
		w = performRequest(srv.Router(), "GET", "/v1/namespaces/test-namespace/memories", nil)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("namespace not found", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		w := performRequest(srv.Router(), "GET", "/v1/namespaces/nonexistent/memories", nil)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// Context Endpoint Tests

func TestGetContext(t *testing.T) {
	t.Run("success without retriever", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		// Create some memories first
		for i := 0; i < 3; i++ {
			memReq := CreateMemoryRequest{
				Namespace: "default",
				Content:   "Test memory content",
			}
			performRequest(srv.Router(), "POST", "/v1/memories", memReq)
		}

		contextReq := GetContextRequestExtended{
			Query:       "test query",
			Namespace:   "default",
			TokenBudget: 4000,
		}
		w := performRequest(srv.Router(), "POST", "/v1/context", contextReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp ContextResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.NotEmpty(t, resp.Content)
		assert.NotNil(t, resp.ZoneStats)
	})

	t.Run("success with custom analyzer and assembler", func(t *testing.T) {
		store := newMockStore()
		analyzer := query.NewAnalyzer()
		assembler := mcontext.NewAssembler(mcontext.DefaultAssemblerConfig())

		deps := &ServerDeps{
			Analyzer:  analyzer,
			Assembler: assembler,
		}
		srv := testServerWithDeps(store, deps)

		// Create some memories
		for i := 0; i < 3; i++ {
			memReq := CreateMemoryRequest{
				Namespace: "default",
				Content:   "Test memory content",
			}
			performRequest(srv.Router(), "POST", "/v1/memories", memReq)
		}

		contextReq := GetContextRequestExtended{
			Query:       "test query",
			Namespace:   "default",
			TokenBudget: 4000,
		}
		w := performRequest(srv.Router(), "POST", "/v1/context", contextReq)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("invalid request - missing query", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		contextReq := map[string]string{
			"namespace": "default",
		}
		w := performRequest(srv.Router(), "POST", "/v1/context", contextReq)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("default namespace when not specified", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		contextReq := GetContextRequestExtended{
			Query: "test query",
		}
		w := performRequest(srv.Router(), "POST", "/v1/context", contextReq)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("default token budget when not specified", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		contextReq := GetContextRequestExtended{
			Query:     "test query",
			Namespace: "default",
		}
		w := performRequest(srv.Router(), "POST", "/v1/context", contextReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp ContextResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 4000, resp.TokenBudget)
	})

	t.Run("with system prompt", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		contextReq := GetContextRequestExtended{
			Query:        "test query",
			SystemPrompt: "You are a helpful assistant.",
		}
		w := performRequest(srv.Router(), "POST", "/v1/context", contextReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp ContextResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Contains(t, resp.Content, "You are a helpful assistant.")
	})
}


// Stats Endpoint Tests

func TestGetStats(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		// Create some data
		performRequest(srv.Router(), "POST", "/v1/namespaces", CreateNamespaceRequest{Name: "test"})
		performRequest(srv.Router(), "POST", "/v1/memories", CreateMemoryRequest{
			Namespace: "test",
			Content:   "content",
		})

		w := performRequest(srv.Router(), "GET", "/v1/stats", nil)

		assert.Equal(t, http.StatusOK, w.Code)

		var stats storage.StoreStats
		err := json.Unmarshal(w.Body.Bytes(), &stats)
		require.NoError(t, err)
		assert.Equal(t, int64(1), stats.TotalMemories)
		assert.Equal(t, int64(1), stats.TotalNamespaces)
	})

	t.Run("error", func(t *testing.T) {
		store := newMockStore()
		store.errOnStats = &storage.ErrInvalidInput{Field: "test", Message: "test error"}
		srv := testServer(store)

		w := performRequest(srv.Router(), "GET", "/v1/stats", nil)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// Error Handling Tests

func TestHandleStorageErrors(t *testing.T) {
	t.Run("not found error", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		w := performRequest(srv.Router(), "GET", "/v1/memories/nonexistent", nil)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var resp ErrorResponse
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		assert.Equal(t, "NOT_FOUND", resp.Code)
	})

	t.Run("already exists error", func(t *testing.T) {
		store := newMockStore()
		srv := testServer(store)

		req := CreateNamespaceRequest{Name: "test"}
		performRequest(srv.Router(), "POST", "/v1/namespaces", req)
		w := performRequest(srv.Router(), "POST", "/v1/namespaces", req)

		assert.Equal(t, http.StatusConflict, w.Code)

		var resp ErrorResponse
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		assert.Equal(t, "ALREADY_EXISTS", resp.Code)
	})

	t.Run("invalid input error", func(t *testing.T) {
		store := newMockStore()
		store.errOnSearch = &storage.ErrInvalidInput{Field: "query", Message: "invalid"}
		srv := testServer(store)

		w := performRequest(srv.Router(), "POST", "/v1/memories/search", SearchMemoriesRequest{})

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp ErrorResponse
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		assert.Equal(t, "INVALID_INPUT", resp.Code)
	})
}

// Helper Tests

func TestParseIntQuery(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		key        string
		defaultVal int
		expected   int
	}{
		{"valid value", "limit=10", "limit", 100, 10},
		{"missing key", "", "limit", 100, 100},
		{"invalid value", "limit=abc", "limit", 100, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request, _ = http.NewRequest("GET", "/?"+tt.query, nil)

			result := parseIntQuery(c, tt.key, tt.defaultVal)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPositionToString(t *testing.T) {
	tests := []struct {
		position mcontext.Position
		expected string
	}{
		{mcontext.PositionCritical, "critical"},
		{mcontext.PositionMiddle, "middle"},
		{mcontext.PositionRecency, "recency"},
		{mcontext.Position(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := positionToString(tt.position)
			assert.Equal(t, tt.expected, result)
		})
	}
}

