package maia

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		opts    []ClientOption
		wantURL string
	}{
		{
			name:    "default options",
			opts:    nil,
			wantURL: DefaultBaseURL,
		},
		{
			name:    "custom base URL",
			opts:    []ClientOption{WithBaseURL("http://custom:9000")},
			wantURL: "http://custom:9000",
		},
		{
			name: "custom timeout",
			opts: []ClientOption{WithTimeout(60 * time.Second)},
		},
		{
			name: "custom header",
			opts: []ClientOption{WithHeader("X-Custom", "value")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := New(tt.opts...)
			require.NotNil(t, client)
			if tt.wantURL != "" {
				assert.Equal(t, tt.wantURL, client.baseURL)
			}
		})
	}
}

func TestClient_Health(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/health", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(HealthResponse{Status: "healthy", Service: "maia"})
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))
	resp, err := client.Health(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "healthy", resp.Status)
	assert.Equal(t, "maia", resp.Service)
}

func TestClient_Ready(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ready", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))
	err := client.Ready(context.Background())

	require.NoError(t, err)
}

func TestClient_Stats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/stats", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Stats{
			TotalMemories:   100,
			TotalNamespaces: 5,
		})
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))
	stats, err := client.Stats(context.Background())

	require.NoError(t, err)
	assert.Equal(t, int64(100), stats.TotalMemories)
	assert.Equal(t, int64(5), stats.TotalNamespaces)
}

func TestClient_CreateMemory(t *testing.T) {
	tests := []struct {
		name    string
		input   *CreateMemoryInput
		handler func(w http.ResponseWriter, r *http.Request)
		wantErr bool
		errMsg  string
	}{
		{
			name: "success",
			input: &CreateMemoryInput{
				Namespace: "test",
				Content:   "Test memory",
				Type:      MemoryTypeSemantic,
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/memories", r.URL.Path)
				assert.Equal(t, http.MethodPost, r.Method)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(Memory{
					ID:        "mem-123",
					Namespace: "test",
					Content:   "Test memory",
					Type:      MemoryTypeSemantic,
				})
			},
			wantErr: false,
		},
		{
			name:    "nil input",
			input:   nil,
			wantErr: true,
			errMsg:  "input cannot be nil",
		},
		{
			name: "empty namespace",
			input: &CreateMemoryInput{
				Content: "Test memory",
			},
			wantErr: true,
			errMsg:  "namespace is required",
		},
		{
			name: "empty content",
			input: &CreateMemoryInput{
				Namespace: "test",
			},
			wantErr: true,
			errMsg:  "content is required",
		},
		{
			name: "server error",
			input: &CreateMemoryInput{
				Namespace: "test",
				Content:   "Test memory",
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "internal error"})
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.handler != nil {
				server = httptest.NewServer(http.HandlerFunc(tt.handler))
				defer server.Close()
			}

			var client *Client
			if server != nil {
				client = New(WithBaseURL(server.URL))
			} else {
				client = New()
			}

			mem, err := client.CreateMemory(context.Background(), tt.input)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, mem.ID)
			assert.Equal(t, tt.input.Content, mem.Content)
		})
	}
}

func TestClient_GetMemory(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		handler func(w http.ResponseWriter, r *http.Request)
		wantErr bool
	}{
		{
			name: "success",
			id:   "mem-123",
			handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/memories/mem-123", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(Memory{
					ID:      "mem-123",
					Content: "Test memory",
				})
			},
			wantErr: false,
		},
		{
			name:    "empty id",
			id:      "",
			wantErr: true,
		},
		{
			name: "not found",
			id:   "nonexistent",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "memory not found",
					"code":  "NOT_FOUND",
				})
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.handler != nil {
				server = httptest.NewServer(http.HandlerFunc(tt.handler))
				defer server.Close()
			}

			var client *Client
			if server != nil {
				client = New(WithBaseURL(server.URL))
			} else {
				client = New()
			}

			mem, err := client.GetMemory(context.Background(), tt.id)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.id, mem.ID)
		})
	}
}

func TestClient_UpdateMemory(t *testing.T) {
	newContent := "Updated content"
	tests := []struct {
		name    string
		id      string
		input   *UpdateMemoryInput
		handler func(w http.ResponseWriter, r *http.Request)
		wantErr bool
	}{
		{
			name:  "success",
			id:    "mem-123",
			input: &UpdateMemoryInput{Content: &newContent},
			handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/memories/mem-123", r.URL.Path)
				assert.Equal(t, http.MethodPut, r.Method)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(Memory{
					ID:      "mem-123",
					Content: "Updated content",
				})
			},
			wantErr: false,
		},
		{
			name:    "empty id",
			id:      "",
			input:   &UpdateMemoryInput{Content: &newContent},
			wantErr: true,
		},
		{
			name:    "nil input",
			id:      "mem-123",
			input:   nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.handler != nil {
				server = httptest.NewServer(http.HandlerFunc(tt.handler))
				defer server.Close()
			}

			var client *Client
			if server != nil {
				client = New(WithBaseURL(server.URL))
			} else {
				client = New()
			}

			mem, err := client.UpdateMemory(context.Background(), tt.id, tt.input)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, "Updated content", mem.Content)
		})
	}
}

func TestClient_DeleteMemory(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		handler func(w http.ResponseWriter, r *http.Request)
		wantErr bool
	}{
		{
			name: "success",
			id:   "mem-123",
			handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/memories/mem-123", r.URL.Path)
				assert.Equal(t, http.MethodDelete, r.Method)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(DeleteResponse{Deleted: true})
			},
			wantErr: false,
		},
		{
			name:    "empty id",
			id:      "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.handler != nil {
				server = httptest.NewServer(http.HandlerFunc(tt.handler))
				defer server.Close()
			}

			var client *Client
			if server != nil {
				client = New(WithBaseURL(server.URL))
			} else {
				client = New()
			}

			err := client.DeleteMemory(context.Background(), tt.id)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestClient_SearchMemories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/memories/search", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ListResponse[*SearchResult]{
			Data: []*SearchResult{
				{Memory: &Memory{ID: "mem-1", Content: "Result 1"}, Score: 0.9},
				{Memory: &Memory{ID: "mem-2", Content: "Result 2"}, Score: 0.8},
			},
			Count:  2,
			Limit:  100,
			Offset: 0,
		})
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))
	results, err := client.SearchMemories(context.Background(), &SearchMemoriesInput{
		Query:     "test query",
		Namespace: "default",
	})

	require.NoError(t, err)
	assert.Len(t, results.Data, 2)
	assert.Equal(t, "mem-1", results.Data[0].Memory.ID)
}

func TestClient_CreateNamespace(t *testing.T) {
	tests := []struct {
		name    string
		input   *CreateNamespaceInput
		handler func(w http.ResponseWriter, r *http.Request)
		wantErr bool
	}{
		{
			name: "success",
			input: &CreateNamespaceInput{
				Name: "test-namespace",
				Config: NamespaceConfig{
					TokenBudget: 4000,
				},
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/namespaces", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(Namespace{
					ID:   "ns-123",
					Name: "test-namespace",
				})
			},
			wantErr: false,
		},
		{
			name:    "nil input",
			input:   nil,
			wantErr: true,
		},
		{
			name: "empty name",
			input: &CreateNamespaceInput{
				Config: NamespaceConfig{TokenBudget: 4000},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.handler != nil {
				server = httptest.NewServer(http.HandlerFunc(tt.handler))
				defer server.Close()
			}

			var client *Client
			if server != nil {
				client = New(WithBaseURL(server.URL))
			} else {
				client = New()
			}

			ns, err := client.CreateNamespace(context.Background(), tt.input)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, ns.ID)
		})
	}
}

func TestClient_GetNamespace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/namespaces/test-namespace", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Namespace{
			ID:   "ns-123",
			Name: "test-namespace",
		})
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))
	ns, err := client.GetNamespace(context.Background(), "test-namespace")

	require.NoError(t, err)
	assert.Equal(t, "ns-123", ns.ID)
	assert.Equal(t, "test-namespace", ns.Name)
}

func TestClient_ListNamespaces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/namespaces", r.URL.Path)
		assert.Equal(t, "10", r.URL.Query().Get("limit"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ListResponse[*Namespace]{
			Data: []*Namespace{
				{ID: "ns-1", Name: "namespace-1"},
				{ID: "ns-2", Name: "namespace-2"},
			},
			Count:  2,
			Limit:  10,
			Offset: 0,
		})
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))
	results, err := client.ListNamespaces(context.Background(), &ListOptions{Limit: 10})

	require.NoError(t, err)
	assert.Len(t, results.Data, 2)
}

func TestClient_ListNamespaceMemories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/namespaces/test/memories", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ListResponse[*Memory]{
			Data: []*Memory{
				{ID: "mem-1", Content: "Memory 1"},
				{ID: "mem-2", Content: "Memory 2"},
			},
			Count:  2,
			Limit:  100,
			Offset: 0,
		})
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))
	results, err := client.ListNamespaceMemories(context.Background(), "test", nil)

	require.NoError(t, err)
	assert.Len(t, results.Data, 2)
}

func TestClient_GetContext(t *testing.T) {
	tests := []struct {
		name    string
		input   *GetContextInput
		handler func(w http.ResponseWriter, r *http.Request)
		wantErr bool
	}{
		{
			name: "success",
			input: &GetContextInput{
				Query:       "What are the user preferences?",
				Namespace:   "default",
				TokenBudget: 2000,
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/context", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(ContextResponse{
					Content:     "User prefers dark mode.",
					TokenCount:  10,
					TokenBudget: 2000,
					Truncated:   false,
				})
			},
			wantErr: false,
		},
		{
			name:    "nil input",
			input:   nil,
			wantErr: true,
		},
		{
			name: "empty query",
			input: &GetContextInput{
				Namespace: "default",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.handler != nil {
				server = httptest.NewServer(http.HandlerFunc(tt.handler))
				defer server.Close()
			}

			var client *Client
			if server != nil {
				client = New(WithBaseURL(server.URL))
			} else {
				client = New()
			}

			resp, err := client.GetContext(context.Background(), tt.input)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, resp.Content)
		})
	}
}

func TestClient_Remember(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input CreateMemoryInput
		_ = json.NewDecoder(r.Body).Decode(&input)
		assert.Equal(t, "test", input.Namespace)
		assert.Equal(t, "User likes coffee", input.Content)
		assert.Equal(t, MemoryTypeSemantic, input.Type)
		assert.Equal(t, MemorySourceUser, input.Source)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(Memory{
			ID:        "mem-123",
			Namespace: input.Namespace,
			Content:   input.Content,
		})
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))
	mem, err := client.Remember(context.Background(), "test", "User likes coffee")

	require.NoError(t, err)
	assert.NotEmpty(t, mem.ID)
	assert.Equal(t, "User likes coffee", mem.Content)
}

func TestClient_Recall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input GetContextInput
		_ = json.NewDecoder(r.Body).Decode(&input)
		assert.Equal(t, "user preferences", input.Query)
		assert.Equal(t, "test", input.Namespace)
		assert.Equal(t, 1000, input.TokenBudget)
		assert.True(t, input.IncludeScores)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ContextResponse{
			Content:    "User likes coffee",
			TokenCount: 5,
		})
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))
	resp, err := client.Recall(context.Background(), "user preferences",
		WithNamespace("test"),
		WithTokenBudget(1000),
		WithScores(),
	)

	require.NoError(t, err)
	assert.Equal(t, "User likes coffee", resp.Content)
}

func TestClient_Forget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/memories/mem-123", r.URL.Path)
		assert.Equal(t, http.MethodDelete, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DeleteResponse{Deleted: true})
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))
	err := client.Forget(context.Background(), "mem-123")

	require.NoError(t, err)
}

func TestClient_WithHTTPClient(t *testing.T) {
	customClient := &http.Client{Timeout: 5 * time.Second}
	client := New(WithHTTPClient(customClient))

	assert.Equal(t, customClient, client.httpClient)
}

func TestAPIError(t *testing.T) {
	tests := []struct {
		name  string
		err   *APIError
		check func(*testing.T, *APIError)
	}{
		{
			name: "not found",
			err:  &APIError{StatusCode: 404, Code: "NOT_FOUND", Message: "memory not found"},
			check: func(t *testing.T, e *APIError) {
				assert.True(t, e.IsNotFound())
				assert.False(t, e.IsAlreadyExists())
				assert.False(t, e.IsServerError())
			},
		},
		{
			name: "already exists",
			err:  &APIError{StatusCode: 409, Code: "ALREADY_EXISTS", Message: "namespace exists"},
			check: func(t *testing.T, e *APIError) {
				assert.True(t, e.IsAlreadyExists())
				assert.False(t, e.IsNotFound())
			},
		},
		{
			name: "invalid input",
			err:  &APIError{StatusCode: 400, Code: "INVALID_INPUT", Message: "missing field"},
			check: func(t *testing.T, e *APIError) {
				assert.True(t, e.IsInvalidInput())
			},
		},
		{
			name: "server error",
			err:  &APIError{StatusCode: 500, Message: "internal error"},
			check: func(t *testing.T, e *APIError) {
				assert.True(t, e.IsServerError())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEmpty(t, tt.err.Error())
			tt.check(t, tt.err)
		})
	}
}

func TestIsNotFoundError(t *testing.T) {
	assert.True(t, IsNotFoundError(&APIError{StatusCode: 404}))
	assert.True(t, IsNotFoundError(&ErrNotFound{Resource: "memory", ID: "123"}))
	assert.False(t, IsNotFoundError(&APIError{StatusCode: 200}))
	assert.False(t, IsNotFoundError(nil))
}

func TestIsAlreadyExistsError(t *testing.T) {
	assert.True(t, IsAlreadyExistsError(&APIError{StatusCode: 409}))
	assert.False(t, IsAlreadyExistsError(&APIError{StatusCode: 404}))
	assert.False(t, IsAlreadyExistsError(nil))
}

func TestClient_UpdateNamespace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/v1/namespaces/ns-123", r.URL.Path)

		var input UpdateNamespaceInput
		err := json.NewDecoder(r.Body).Decode(&input)
		require.NoError(t, err)
		assert.Equal(t, 8000, input.Config.TokenBudget)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Namespace{
			ID:   "ns-123",
			Name: "test-namespace",
			Config: NamespaceConfig{
				TokenBudget: 8000,
			},
		})
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))
	ns, err := client.UpdateNamespace(context.Background(), "ns-123", &UpdateNamespaceInput{
		Config: NamespaceConfig{TokenBudget: 8000},
	})

	require.NoError(t, err)
	assert.Equal(t, "ns-123", ns.ID)
	assert.Equal(t, 8000, ns.Config.TokenBudget)
}

func TestClient_UpdateNamespace_EmptyID(t *testing.T) {
	client := New()
	_, err := client.UpdateNamespace(context.Background(), "", &UpdateNamespaceInput{Config: NamespaceConfig{TokenBudget: 5000}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "id is required")
}

func TestClient_UpdateNamespace_NilInput(t *testing.T) {
	client := New()
	_, err := client.UpdateNamespace(context.Background(), "ns-123", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "input cannot be nil")
}

func TestClient_DeleteNamespace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/v1/namespaces/ns-123", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DeleteResponse{Deleted: true})
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))
	err := client.DeleteNamespace(context.Background(), "ns-123")

	require.NoError(t, err)
}

func TestClient_DeleteNamespace_EmptyID(t *testing.T) {
	client := New()
	err := client.DeleteNamespace(context.Background(), "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "id is required")
}

func TestClient_ListNamespaceMemories_WithPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Contains(t, r.URL.Path, "/v1/namespaces/test-ns/memories")
		assert.Equal(t, "20", r.URL.Query().Get("limit"))
		assert.Equal(t, "10", r.URL.Query().Get("offset"))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ListResponse[*Memory]{
			Data: []*Memory{
				{ID: "mem-1", Content: "Memory 1"},
				{ID: "mem-2", Content: "Memory 2"},
			},
			Count: 30,
		})
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))
	resp, err := client.ListNamespaceMemories(context.Background(), "test-ns", &ListOptions{
		Limit:  20,
		Offset: 10,
	})

	require.NoError(t, err)
	assert.Len(t, resp.Data, 2)
	assert.Equal(t, 30, resp.Count)
}

func TestClient_ListNamespaceMemories_EmptyNamespace(t *testing.T) {
	client := New()
	_, err := client.ListNamespaceMemories(context.Background(), "", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "namespace is required")
}

func TestRecallOptions(t *testing.T) {
	t.Run("WithSystemPrompt", func(t *testing.T) {
		input := &GetContextInput{}
		opt := WithSystemPrompt("You are a helpful assistant")
		opt(input)
		assert.Equal(t, "You are a helpful assistant", input.SystemPrompt)
	})

	t.Run("WithMinScore", func(t *testing.T) {
		input := &GetContextInput{}
		opt := WithMinScore(0.5)
		opt(input)
		assert.Equal(t, 0.5, input.MinScore)
	})
}

func TestErrNotFound_Error(t *testing.T) {
	err := &ErrNotFound{Resource: "memory", ID: "mem-123"}
	assert.Equal(t, "memory not found: mem-123", err.Error())
}

func TestWithHeader_AppendsHeaders(t *testing.T) {
	client := New(
		WithHeader("X-Custom-1", "value1"),
		WithHeader("X-Custom-2", "value2"),
	)
	assert.Equal(t, "value1", client.headers["X-Custom-1"])
	assert.Equal(t, "value2", client.headers["X-Custom-2"])
}

func TestClient_Health_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(APIError{Message: "server error"})
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))
	_, err := client.Health(context.Background())

	assert.Error(t, err)
}

func TestClient_Stats_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(APIError{Message: "server error"})
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))
	_, err := client.Stats(context.Background())

	assert.Error(t, err)
}

func TestClient_SearchMemories_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(APIError{Message: "bad request"})
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))
	_, err := client.SearchMemories(context.Background(), &SearchMemoriesInput{Query: "test"})

	assert.Error(t, err)
}

func TestClient_GetNamespace_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(APIError{StatusCode: 404, Code: "NOT_FOUND", Message: "namespace not found"})
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))
	_, err := client.GetNamespace(context.Background(), "nonexistent")

	assert.Error(t, err)
	assert.True(t, IsNotFoundError(err))
}
