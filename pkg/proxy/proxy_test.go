package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
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
		json.NewEncoder(w).Encode(resp)
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
		json.NewEncoder(w).Encode(ErrorResponse{
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
		json.NewEncoder(w).Encode(resp)
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
			w.Write([]byte("data: " + chunk + "\n\n"))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		w.Write([]byte("data: [DONE]\n\n"))
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
