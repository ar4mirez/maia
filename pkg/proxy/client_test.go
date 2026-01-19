package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_ChatCompletion(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		request        *ChatCompletionRequest
		authHeader     string
		wantErr        bool
		checkFn        func(t *testing.T, resp *ChatCompletionResponse, err error)
	}{
		{
			name: "successful request",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

				resp := ChatCompletionResponse{
					ID:      "chatcmpl-123",
					Object:  "chat.completion",
					Created: time.Now().Unix(),
					Model:   "gpt-4",
					Choices: []Choice{
						{
							Index: 0,
							Message: &ChatMessage{
								Role:    "assistant",
								Content: "Hello!",
							},
							FinishReason: "stop",
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			},
			request: &ChatCompletionRequest{
				Model: "gpt-4",
				Messages: []ChatMessage{
					{Role: "user", Content: "Hi"},
				},
			},
			authHeader: "Bearer test-key",
			checkFn: func(t *testing.T, resp *ChatCompletionResponse, err error) {
				require.NoError(t, err)
				assert.Equal(t, "chatcmpl-123", resp.ID)
				assert.Len(t, resp.Choices, 1)
				assert.Equal(t, "Hello!", resp.Choices[0].Message.GetContentString())
			},
		},
		{
			name: "error response",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(ErrorResponse{
					Error: &APIError{
						Message: "Invalid API key",
						Type:    "invalid_request_error",
					},
				})
			},
			request: &ChatCompletionRequest{
				Model: "gpt-4",
				Messages: []ChatMessage{
					{Role: "user", Content: "Hi"},
				},
			},
			checkFn: func(t *testing.T, resp *ChatCompletionResponse, err error) {
				require.Error(t, err)
				assert.Nil(t, resp)
				be, ok := err.(*BackendError)
				require.True(t, ok)
				assert.Equal(t, http.StatusUnauthorized, be.StatusCode)
				assert.Contains(t, be.Message, "Invalid API key")
			},
		},
		{
			name: "non-json error response",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("Internal Server Error"))
			},
			request: &ChatCompletionRequest{
				Model:    "gpt-4",
				Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
			},
			checkFn: func(t *testing.T, resp *ChatCompletionResponse, err error) {
				require.Error(t, err)
				be, ok := err.(*BackendError)
				require.True(t, ok)
				assert.Equal(t, http.StatusInternalServerError, be.StatusCode)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			client := NewClient(&ClientConfig{
				BaseURL: server.URL,
				Timeout: 5 * time.Second,
			})

			resp, err := client.ChatCompletion(context.Background(), tt.request, tt.authHeader)
			tt.checkFn(t, resp, err)
		})
	}
}

func TestClient_ChatCompletionStream(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		checkFn        func(t *testing.T, reader *StreamReader, err error)
	}{
		{
			name: "successful streaming",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "text/event-stream", r.Header.Get("Accept"))

				// Verify request has stream:true
				body, _ := io.ReadAll(r.Body)
				assert.Contains(t, string(body), `"stream":true`)

				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)

				chunks := []string{
					`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
					`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
					`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}`,
					`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
				}

				for _, chunk := range chunks {
					_, _ = w.Write([]byte("data: " + chunk + "\n\n"))
				}
				_, _ = w.Write([]byte("data: [DONE]\n\n"))
			},
			checkFn: func(t *testing.T, reader *StreamReader, err error) {
				require.NoError(t, err)
				require.NotNil(t, reader)
				defer reader.Close()

				var contents []string
				for {
					chunk, err := reader.Read()
					if err == io.EOF {
						break
					}
					require.NoError(t, err)

					if len(chunk.Choices) > 0 && chunk.Choices[0].Delta != nil {
						content := chunk.Choices[0].Delta.GetContentString()
						if content != "" {
							contents = append(contents, content)
						}
					}
				}

				assert.Equal(t, []string{"Hello", " world"}, contents)
			},
		},
		{
			name: "error response",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(ErrorResponse{
					Error: &APIError{
						Message: "Invalid API key",
						Type:    "invalid_request_error",
					},
				})
			},
			checkFn: func(t *testing.T, reader *StreamReader, err error) {
				require.Error(t, err)
				assert.Nil(t, reader)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			client := NewClient(&ClientConfig{
				BaseURL: server.URL,
				Timeout: 5 * time.Second,
			})

			req := &ChatCompletionRequest{
				Model:  "gpt-4",
				Stream: true,
				Messages: []ChatMessage{
					{Role: "user", Content: "Hi"},
				},
			}

			reader, err := client.ChatCompletionStream(context.Background(), req, "Bearer test-key")
			tt.checkFn(t, reader, err)
		})
	}
}

func TestClient_ListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/models", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		resp := ModelsResponse{
			Object: "list",
			Data: []Model{
				{ID: "gpt-4", Object: "model", OwnedBy: "openai"},
				{ID: "gpt-3.5-turbo", Object: "model", OwnedBy: "openai"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(&ClientConfig{BaseURL: server.URL})

	resp, err := client.ListModels(context.Background(), "Bearer test-key")
	require.NoError(t, err)
	assert.Len(t, resp.Data, 2)
	assert.Equal(t, "gpt-4", resp.Data[0].ID)
}

func TestStreamReader_Read(t *testing.T) {
	// Test is covered by TestClient_ChatCompletionStream which uses
	// the actual StreamReader with a real HTTP response
}

func TestAccumulator(t *testing.T) {
	acc := NewAccumulator()

	// Add first chunk
	acc.Add(&ChatCompletionChunk{
		ID:      "chatcmpl-1",
		Object:  "chat.completion.chunk",
		Created: 1234567890,
		Model:   "gpt-4",
		Choices: []Choice{
			{
				Index: 0,
				Delta: &ChatMessage{
					Role:    "assistant",
					Content: nil,
				},
			},
		},
	})

	// Add content chunks
	acc.Add(&ChatCompletionChunk{
		ID:     "chatcmpl-1",
		Object: "chat.completion.chunk",
		Choices: []Choice{
			{
				Index: 0,
				Delta: &ChatMessage{
					Content: "Hello",
				},
			},
		},
	})

	acc.Add(&ChatCompletionChunk{
		ID:     "chatcmpl-1",
		Object: "chat.completion.chunk",
		Choices: []Choice{
			{
				Index: 0,
				Delta: &ChatMessage{
					Content: " world!",
				},
			},
		},
	})

	// Add final chunk
	acc.Add(&ChatCompletionChunk{
		ID:     "chatcmpl-1",
		Object: "chat.completion.chunk",
		Choices: []Choice{
			{
				Index:        0,
				FinishReason: "stop",
			},
		},
	})

	// Verify accumulated data
	assert.Equal(t, "chatcmpl-1", acc.ID)
	assert.Equal(t, "gpt-4", acc.Model)
	assert.Equal(t, "Hello world!", acc.GetAssistantContent())

	// Convert to response
	resp := acc.ToResponse()
	assert.Equal(t, "chatcmpl-1", resp.ID)
	assert.Len(t, resp.Choices, 1)
	assert.Equal(t, "Hello world!", resp.Choices[0].Message.GetContentString())
	assert.Equal(t, "stop", resp.Choices[0].FinishReason)
}

func TestBackendError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *BackendError
		expected string
	}{
		{
			name: "with type",
			err: &BackendError{
				StatusCode: 401,
				Type:       "invalid_request_error",
				Message:    "Invalid API key",
			},
			expected: "backend error (401): invalid_request_error - Invalid API key",
		},
		{
			name: "without type",
			err: &BackendError{
				StatusCode: 500,
				Message:    "Internal error",
			},
			expected: "backend error (500): Internal error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}
