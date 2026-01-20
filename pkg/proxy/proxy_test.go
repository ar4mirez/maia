package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ar4mirez/maia/internal/storage"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupTestProxy(backendURL string) *Proxy {
	return NewProxy(&ProxyConfig{
		Backend:          backendURL,
		AutoRemember:     false,
		AutoContext:      false,
		ContextPosition:  PositionSystem,
		TokenBudget:      4000,
		DefaultNamespace: "default",
		Timeout:          5 * time.Second,
	}, &ProxyDeps{
		Logger: zap.NewNop(),
	})
}

func TestProxy_RegisterRoutes(t *testing.T) {
	proxy := setupTestProxy("http://localhost:8080")

	router := gin.New()
	proxy.RegisterRoutes(router)

	// Verify routes are registered
	routes := router.Routes()
	routePaths := make(map[string]bool)
	for _, r := range routes {
		routePaths[r.Method+":"+r.Path] = true
	}

	assert.True(t, routePaths["POST:/v1/chat/completions"])
	assert.True(t, routePaths["GET:/v1/models"])
	assert.True(t, routePaths["POST:/proxy/v1/chat/completions"])
	assert.True(t, routePaths["GET:/proxy/v1/models"])
}

func TestProxy_handleChatCompletion_NonStreaming(t *testing.T) {
	// Create mock backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ChatCompletionResponse{
			ID:      "chatcmpl-test",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "gpt-4",
			Choices: []Choice{
				{
					Index: 0,
					Message: &ChatMessage{
						Role:    "assistant",
						Content: "Hello! How can I help?",
					},
					FinishReason: "stop",
				},
			},
			Usage: &Usage{
				PromptTokens:     10,
				CompletionTokens: 20,
				TotalTokens:      30,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer backend.Close()

	proxy := setupTestProxy(backend.URL)
	router := gin.New()
	proxy.RegisterRoutes(router)

	// Create request
	reqBody := ChatCompletionRequest{
		Model: "gpt-4",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	}
	body, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key")

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp ChatCompletionResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "chatcmpl-test", resp.ID)
	assert.Len(t, resp.Choices, 1)
	assert.Equal(t, "Hello! How can I help?", resp.Choices[0].Message.GetContentString())
}

func TestProxy_handleChatCompletion_InvalidRequest(t *testing.T) {
	proxy := setupTestProxy("http://localhost:8080")
	router := gin.New()
	proxy.RegisterRoutes(router)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var errResp ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &errResp)
	require.NoError(t, err)
	assert.Equal(t, "invalid_request", errResp.Error.Type)
}

func TestProxy_handleChatCompletion_BackendError(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(ErrorResponse{
			Error: &APIError{
				Message: "Invalid API key",
				Type:    "invalid_request_error",
			},
		})
	}))
	defer backend.Close()

	proxy := setupTestProxy(backend.URL)
	router := gin.New()
	proxy.RegisterRoutes(router)

	reqBody := ChatCompletionRequest{
		Model:    "gpt-4",
		Messages: []ChatMessage{{Role: "user", Content: "Hello"}},
	}
	body, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestProxy_handleListModels(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/models", r.URL.Path)
		resp := ModelsResponse{
			Object: "list",
			Data: []Model{
				{ID: "gpt-4", Object: "model", OwnedBy: "openai"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer backend.Close()

	proxy := setupTestProxy(backend.URL)
	router := gin.New()
	proxy.RegisterRoutes(router)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer test-key")

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp ModelsResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Len(t, resp.Data, 1)
	assert.Equal(t, "gpt-4", resp.Data[0].ID)
}

func TestProxy_extractOptions(t *testing.T) {
	proxy := NewProxy(&ProxyConfig{
		DefaultNamespace: "default-ns",
		TokenBudget:      4000,
	}, &ProxyDeps{Logger: zap.NewNop()})

	tests := []struct {
		name           string
		headers        map[string]string
		wantNamespace  string
		wantBudget     int
		wantSkipMemory bool
		wantSkipExtract bool
	}{
		{
			name:          "defaults",
			headers:       map[string]string{},
			wantNamespace: "default-ns",
			wantBudget:    4000,
		},
		{
			name: "custom namespace",
			headers: map[string]string{
				HeaderNamespace: "custom-ns",
			},
			wantNamespace: "custom-ns",
			wantBudget:    4000,
		},
		{
			name: "custom token budget",
			headers: map[string]string{
				HeaderTokenBudget: "2000",
			},
			wantNamespace: "default-ns",
			wantBudget:    2000,
		},
		{
			name: "skip memory true",
			headers: map[string]string{
				HeaderSkipMemory: "true",
			},
			wantNamespace:  "default-ns",
			wantBudget:     4000,
			wantSkipMemory: true,
		},
		{
			name: "skip memory 1",
			headers: map[string]string{
				HeaderSkipMemory: "1",
			},
			wantNamespace:  "default-ns",
			wantBudget:     4000,
			wantSkipMemory: true,
		},
		{
			name: "skip extract",
			headers: map[string]string{
				HeaderSkipExtract: "true",
			},
			wantNamespace:   "default-ns",
			wantBudget:      4000,
			wantSkipExtract: true,
		},
		{
			name: "invalid token budget",
			headers: map[string]string{
				HeaderTokenBudget: "invalid",
			},
			wantNamespace: "default-ns",
			wantBudget:    4000,
		},
		{
			name: "all options",
			headers: map[string]string{
				HeaderNamespace:   "test-ns",
				HeaderTokenBudget: "3000",
				HeaderSkipMemory:  "true",
				HeaderSkipExtract: "1",
			},
			wantNamespace:   "test-ns",
			wantBudget:      3000,
			wantSkipMemory:  true,
			wantSkipExtract: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			for k, v := range tt.headers {
				c.Request.Header.Set(k, v)
			}

			opts := proxy.extractOptions(c)
			assert.Equal(t, tt.wantNamespace, opts.Namespace)
			assert.Equal(t, tt.wantBudget, opts.TokenBudget)
			assert.Equal(t, tt.wantSkipMemory, opts.SkipMemory)
			assert.Equal(t, tt.wantSkipExtract, opts.SkipExtract)
		})
	}
}

func TestProxy_sendError(t *testing.T) {
	proxy := setupTestProxy("http://localhost:8080")

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	proxy.sendError(c, http.StatusBadRequest, "invalid_request", "Missing model field")

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var errResp ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &errResp)
	require.NoError(t, err)
	assert.Equal(t, "invalid_request", errResp.Error.Type)
	assert.Equal(t, "Missing model field", errResp.Error.Message)
}

func TestProxy_handleChatCompletion_Streaming(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		chunks := []string{
			`{"id":"1","object":"chat.completion.chunk","created":123,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
			`{"id":"1","object":"chat.completion.chunk","created":123,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hi"},"finish_reason":null}]}`,
			`{"id":"1","object":"chat.completion.chunk","created":123,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		}

		for _, chunk := range chunks {
			_, _ = w.Write([]byte("data: " + chunk + "\n\n"))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer backend.Close()

	proxy := setupTestProxy(backend.URL)
	router := gin.New()
	proxy.RegisterRoutes(router)

	reqBody := ChatCompletionRequest{
		Model:  "gpt-4",
		Stream: true,
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	}
	body, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "data: ")
	assert.Contains(t, w.Body.String(), "[DONE]")
}

func TestProxy_HeaderConstants(t *testing.T) {
	// Verify header constants are defined
	assert.Equal(t, "X-MAIA-Namespace", HeaderNamespace)
	assert.Equal(t, "X-MAIA-Skip-Memory", HeaderSkipMemory)
	assert.Equal(t, "X-MAIA-Skip-Extract", HeaderSkipExtract)
	assert.Equal(t, "X-MAIA-Token-Budget", HeaderTokenBudget)
}

func TestProxy_handleBackendError_ContextDeadlineExceeded(t *testing.T) {
	proxy := setupTestProxy("http://localhost:8080")

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Test context deadline exceeded error
	err := fmt.Errorf("something failed: context deadline exceeded")
	proxy.handleBackendError(c, err)

	assert.Equal(t, http.StatusGatewayTimeout, w.Code)

	var errResp ErrorResponse
	unmarshalErr := json.Unmarshal(w.Body.Bytes(), &errResp)
	require.NoError(t, unmarshalErr)
	assert.Equal(t, "timeout", errResp.Error.Type)
	assert.Contains(t, errResp.Error.Message, "timed out")
}

func TestProxy_handleBackendError_ContextCanceled(t *testing.T) {
	proxy := setupTestProxy("http://localhost:8080")

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Test context canceled error
	err := fmt.Errorf("request failed: context canceled")
	proxy.handleBackendError(c, err)

	assert.Equal(t, http.StatusGatewayTimeout, w.Code)
}

func TestProxy_handleBackendError_BackendError(t *testing.T) {
	proxy := setupTestProxy("http://localhost:8080")

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Test BackendError
	err := &BackendError{
		StatusCode: http.StatusUnauthorized,
		Type:       "invalid_api_key",
		Message:    "Invalid API key provided",
	}
	proxy.handleBackendError(c, err)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var errResp ErrorResponse
	unmarshalErr := json.Unmarshal(w.Body.Bytes(), &errResp)
	require.NoError(t, unmarshalErr)
	assert.Equal(t, "invalid_api_key", errResp.Error.Type)
	assert.Equal(t, "Invalid API key provided", errResp.Error.Message)
}

func TestProxy_handleBackendError_GenericError(t *testing.T) {
	proxy := setupTestProxy("http://localhost:8080")

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Test generic error
	err := fmt.Errorf("connection refused")
	proxy.handleBackendError(c, err)

	assert.Equal(t, http.StatusBadGateway, w.Code)

	var errResp ErrorResponse
	unmarshalErr := json.Unmarshal(w.Body.Bytes(), &errResp)
	require.NoError(t, unmarshalErr)
	assert.Equal(t, "backend_error", errResp.Error.Type)
	assert.Contains(t, errResp.Error.Message, "connection refused")
}

func TestProxy_handleListModels_BackendError(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(ErrorResponse{
			Error: &APIError{
				Message: "Internal server error",
				Type:    "server_error",
			},
		})
	}))
	defer backend.Close()

	proxy := setupTestProxy(backend.URL)
	router := gin.New()
	proxy.RegisterRoutes(router)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestNewProxy_WithAllDependencies(t *testing.T) {
	// Create proxy with all dependencies set
	proxy := NewProxy(&ProxyConfig{
		Backend:          "http://localhost:8080",
		AutoRemember:     true,
		AutoContext:      true,
		ContextPosition:  PositionSystem,
		TokenBudget:      4000,
		DefaultNamespace: "test-namespace",
		Timeout:          10 * time.Second,
	}, &ProxyDeps{
		Logger: zap.NewNop(),
		// Note: Store, Retriever, Assembler are nil for this test
	})

	assert.NotNil(t, proxy)
	assert.Equal(t, "test-namespace", proxy.defaultNamespace)
	assert.Equal(t, 4000, proxy.defaultBudget)
}

func TestNewProxy_WithNilLogger(t *testing.T) {
	// Create proxy without logger - should use nop logger
	proxy := NewProxy(&ProxyConfig{
		Backend: "http://localhost:8080",
	}, &ProxyDeps{
		Logger: nil,
	})

	assert.NotNil(t, proxy)
	assert.NotNil(t, proxy.logger)
}

// mockProxyStore implements storage.Store for testing proxy memory extraction.
type mockProxyStore struct {
	memories   []*mockMemory
	createErr  error
	memCounter int
	mu         sync.Mutex
}

type mockMemory struct {
	ID        string
	Namespace string
	Content   string
}

func newMockProxyStore() *mockProxyStore {
	return &mockProxyStore{
		memories: make([]*mockMemory, 0),
	}
}

func (m *mockProxyStore) CreateMemory(_ context.Context, input *storage.CreateMemoryInput) (*storage.Memory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return nil, m.createErr
	}
	m.memCounter++
	mem := &mockMemory{
		ID:        fmt.Sprintf("mem_%d", m.memCounter),
		Namespace: input.Namespace,
		Content:   input.Content,
	}
	m.memories = append(m.memories, mem)
	return &storage.Memory{
		ID:        mem.ID,
		Namespace: input.Namespace,
		Content:   input.Content,
	}, nil
}

func (m *mockProxyStore) GetMemory(_ context.Context, _ string) (*storage.Memory, error) {
	return nil, nil
}
func (m *mockProxyStore) UpdateMemory(_ context.Context, _ string, _ *storage.UpdateMemoryInput) (*storage.Memory, error) {
	return nil, nil
}
func (m *mockProxyStore) DeleteMemory(_ context.Context, _ string) error { return nil }
func (m *mockProxyStore) ListMemories(_ context.Context, _ string, _ *storage.ListOptions) ([]*storage.Memory, error) {
	return nil, nil
}
func (m *mockProxyStore) SearchMemories(_ context.Context, _ *storage.SearchOptions) ([]*storage.SearchResult, error) {
	return nil, nil
}
func (m *mockProxyStore) TouchMemory(_ context.Context, _ string) error { return nil }
func (m *mockProxyStore) CreateNamespace(_ context.Context, _ *storage.CreateNamespaceInput) (*storage.Namespace, error) {
	return nil, nil
}
func (m *mockProxyStore) GetNamespace(_ context.Context, _ string) (*storage.Namespace, error) {
	return nil, nil
}
func (m *mockProxyStore) GetNamespaceByName(_ context.Context, _ string) (*storage.Namespace, error) {
	return nil, nil
}
func (m *mockProxyStore) UpdateNamespace(_ context.Context, _ string, _ *storage.NamespaceConfig) (*storage.Namespace, error) {
	return nil, nil
}
func (m *mockProxyStore) DeleteNamespace(_ context.Context, _ string) error { return nil }
func (m *mockProxyStore) ListNamespaces(_ context.Context, _ *storage.ListOptions) ([]*storage.Namespace, error) {
	return nil, nil
}
func (m *mockProxyStore) BatchCreateMemories(_ context.Context, _ []*storage.CreateMemoryInput) ([]*storage.Memory, error) {
	return nil, nil
}
func (m *mockProxyStore) BatchDeleteMemories(_ context.Context, _ []string) error { return nil }
func (m *mockProxyStore) Close() error                                            { return nil }
func (m *mockProxyStore) Stats(_ context.Context) (*storage.StoreStats, error)    { return nil, nil }

func TestProxy_extractAndStoreMemories_Direct(t *testing.T) {
	store := newMockProxyStore()

	proxy := NewProxy(&ProxyConfig{
		Backend:          "http://localhost:8080",
		AutoRemember:     true,
		DefaultNamespace: "test",
	}, &ProxyDeps{
		Store:  store,
		Logger: zap.NewNop(),
	})

	messages := []ChatMessage{
		{Role: "user", Content: "I prefer dark mode for my IDE."},
	}

	resp := &ChatCompletionResponse{
		Choices: []Choice{
			{
				Index: 0,
				Message: &ChatMessage{
					Role:    "assistant",
					Content: "I understand you prefer dark mode for your IDE. I'll remember that preference.",
				},
			},
		},
	}

	// Call extractAndStoreMemories directly
	proxy.extractAndStoreMemories("test-ns", messages, resp)

	// Wait a bit for processing
	time.Sleep(50 * time.Millisecond)

	store.mu.Lock()
	defer store.mu.Unlock()
	// Should have extracted at least one memory
	assert.NotEmpty(t, store.memories)
}

func TestProxy_extractAndStoreMemories_EmptyResponse(t *testing.T) {
	store := newMockProxyStore()

	proxy := NewProxy(&ProxyConfig{
		Backend:          "http://localhost:8080",
		AutoRemember:     true,
		DefaultNamespace: "test",
	}, &ProxyDeps{
		Store:  store,
		Logger: zap.NewNop(),
	})

	messages := []ChatMessage{
		{Role: "user", Content: "Hello"},
	}

	// Response with no choices
	resp := &ChatCompletionResponse{
		Choices: []Choice{},
	}

	proxy.extractAndStoreMemories("test-ns", messages, resp)

	time.Sleep(50 * time.Millisecond)

	store.mu.Lock()
	defer store.mu.Unlock()
	assert.Empty(t, store.memories)
}

func TestProxy_extractAndStoreMemories_NilMessage(t *testing.T) {
	store := newMockProxyStore()

	proxy := NewProxy(&ProxyConfig{
		Backend:          "http://localhost:8080",
		AutoRemember:     true,
		DefaultNamespace: "test",
	}, &ProxyDeps{
		Store:  store,
		Logger: zap.NewNop(),
	})

	messages := []ChatMessage{
		{Role: "user", Content: "Hello"},
	}

	// Response with nil message
	resp := &ChatCompletionResponse{
		Choices: []Choice{
			{Index: 0, Message: nil},
		},
	}

	proxy.extractAndStoreMemories("test-ns", messages, resp)

	time.Sleep(50 * time.Millisecond)

	store.mu.Lock()
	defer store.mu.Unlock()
	assert.Empty(t, store.memories)
}

func TestProxy_extractAndStoreMemoriesFromAccumulator_Direct(t *testing.T) {
	store := newMockProxyStore()

	proxy := NewProxy(&ProxyConfig{
		Backend:          "http://localhost:8080",
		AutoRemember:     true,
		DefaultNamespace: "test",
	}, &ProxyDeps{
		Store:  store,
		Logger: zap.NewNop(),
	})

	messages := []ChatMessage{
		{Role: "user", Content: "I prefer dark mode for coding."},
	}

	acc := NewAccumulator()
	acc.Add(&ChatCompletionChunk{
		ID:      "chunk-1",
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   "gpt-4",
		Choices: []Choice{
			{Index: 0, Delta: &ChatMessage{Content: "I understand you prefer dark mode. I'll remember that preference."}},
		},
	})

	proxy.extractAndStoreMemoriesFromAccumulator("test-ns", messages, acc)

	time.Sleep(50 * time.Millisecond)

	store.mu.Lock()
	defer store.mu.Unlock()
	assert.NotEmpty(t, store.memories)
}

func TestProxy_extractAndStoreMemoriesFromAccumulator_EmptyContent(t *testing.T) {
	store := newMockProxyStore()

	proxy := NewProxy(&ProxyConfig{
		Backend:          "http://localhost:8080",
		AutoRemember:     true,
		DefaultNamespace: "test",
	}, &ProxyDeps{
		Store:  store,
		Logger: zap.NewNop(),
	})

	messages := []ChatMessage{
		{Role: "user", Content: "Hello"},
	}

	// Empty accumulator
	acc := NewAccumulator()

	proxy.extractAndStoreMemoriesFromAccumulator("test-ns", messages, acc)

	time.Sleep(50 * time.Millisecond)

	store.mu.Lock()
	defer store.mu.Unlock()
	assert.Empty(t, store.memories)
}
