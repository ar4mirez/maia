package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	client := NewClient("http://localhost:8080")
	assert.NotNil(t, client)
	assert.Equal(t, "http://localhost:8080", client.baseURL)
	assert.NotNil(t, client.httpClient)
	assert.Equal(t, 30*time.Second, client.httpClient.Timeout)
}

func TestClient_Get(t *testing.T) {
	tests := []struct {
		name       string
		handler    func(w http.ResponseWriter, r *http.Request)
		result     interface{}
		wantErr    bool
		errMessage string
	}{
		{
			name: "successful GET",
			handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "application/json", r.Header.Get("Accept"))
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			},
			result:  &map[string]string{},
			wantErr: false,
		},
		{
			name: "404 error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found", "code": "NOT_FOUND"})
			},
			result:     &map[string]string{},
			wantErr:    true,
			errMessage: "not found",
		},
		{
			name: "500 error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("internal error"))
			},
			result:     &map[string]string{},
			wantErr:    true,
			errMessage: "internal error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.handler))
			defer server.Close()

			client := NewClient(server.URL)
			err := client.Get("/test", tt.result)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMessage)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestClient_Post(t *testing.T) {
	tests := []struct {
		name       string
		body       interface{}
		handler    func(w http.ResponseWriter, r *http.Request)
		wantErr    bool
	}{
		{
			name: "successful POST with body",
			body: map[string]string{"key": "value"},
			handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				var received map[string]string
				_ = json.NewDecoder(r.Body).Decode(&received)
				assert.Equal(t, "value", received["key"])

				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]string{"id": "123"})
			},
			wantErr: false,
		},
		{
			name: "POST without body",
			body: nil,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.handler))
			defer server.Close()

			client := NewClient(server.URL)
			var result map[string]string
			err := client.Post("/test", tt.body, &result)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestClient_Put(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"updated": "true"})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	var result map[string]string
	err := client.Put("/test", map[string]string{"key": "value"}, &result)

	require.NoError(t, err)
	assert.Equal(t, "true", result["updated"])
}

func TestClient_Delete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"deleted": true})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	var result map[string]bool
	err := client.Delete("/test", &result)

	require.NoError(t, err)
	assert.True(t, result["deleted"])
}

func TestError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      Error
		expected string
	}{
		{
			name:     "with code",
			err:      Error{Message: "not found", Code: "NOT_FOUND"},
			expected: "not found (NOT_FOUND)",
		},
		{
			name:     "without code",
			err:      Error{Message: "something went wrong"},
			expected: "something went wrong",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}
