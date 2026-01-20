package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ar4mirez/maia/internal/inference"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProvider(t *testing.T) {
	tests := []struct {
		name    string
		cfg     inference.ProviderConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			cfg: inference.ProviderConfig{
				APIKey: "test-api-key",
			},
			wantErr: false,
		},
		{
			name: "valid config with custom base URL",
			cfg: inference.ProviderConfig{
				APIKey:  "test-api-key",
				BaseURL: "https://custom.anthropic.com/v1/",
			},
			wantErr: false,
		},
		{
			name: "valid config with timeout",
			cfg: inference.ProviderConfig{
				APIKey:  "test-api-key",
				Timeout: 60 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "valid config with models",
			cfg: inference.ProviderConfig{
				APIKey: "test-api-key",
				Models: []string{"claude-3-opus-20240229", "claude-3-sonnet-20240229"},
			},
			wantErr: false,
		},
		{
			name:    "missing API key",
			cfg:     inference.ProviderConfig{},
			wantErr: true,
			errMsg:  "API key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewProvider("test", tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, p)
			assert.Equal(t, "test", p.Name())
		})
	}
}

func TestProvider_Name(t *testing.T) {
	p, err := NewProvider("my-anthropic", inference.ProviderConfig{
		APIKey: "test-key",
	})
	require.NoError(t, err)
	assert.Equal(t, "my-anthropic", p.Name())
}

func TestProvider_SupportsModel(t *testing.T) {
	tests := []struct {
		name          string
		models        []string
		modelID       string
		wantSupported bool
	}{
		{
			name:          "default supports claude models",
			models:        nil,
			modelID:       "claude-3-opus-20240229",
			wantSupported: true,
		},
		{
			name:          "default supports claude-3-5",
			models:        nil,
			modelID:       "claude-3-5-sonnet-20241022",
			wantSupported: true,
		},
		{
			name:          "default does not support gpt",
			models:        nil,
			modelID:       "gpt-4",
			wantSupported: false,
		},
		{
			name:          "configured models exact match",
			models:        []string{"claude-3-opus-20240229"},
			modelID:       "claude-3-opus-20240229",
			wantSupported: true,
		},
		{
			name:          "configured models no match",
			models:        []string{"claude-3-opus-20240229"},
			modelID:       "claude-3-sonnet-20240229",
			wantSupported: false,
		},
		{
			name:          "configured models wildcard match",
			models:        []string{"claude-3*"},
			modelID:       "claude-3-opus-20240229",
			wantSupported: true,
		},
		{
			name:          "configured models wildcard no match",
			models:        []string{"claude-3*"},
			modelID:       "claude-2-100k",
			wantSupported: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewProvider("test", inference.ProviderConfig{
				APIKey: "test-key",
				Models: tt.models,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantSupported, p.SupportsModel(tt.modelID))
		})
	}
}

func TestProvider_ListModels(t *testing.T) {
	t.Run("returns configured models", func(t *testing.T) {
		p, err := NewProvider("test", inference.ProviderConfig{
			APIKey: "test-key",
			Models: []string{"claude-3-opus-20240229", "claude-3-sonnet-20240229"},
		})
		require.NoError(t, err)

		models, err := p.ListModels(context.Background())
		require.NoError(t, err)
		assert.Len(t, models, 2)
		assert.Equal(t, "claude-3-opus-20240229", models[0].ID)
		assert.Equal(t, "anthropic", models[0].OwnedBy)
		assert.Equal(t, "test", models[0].Provider)
	})

	t.Run("returns known models when none configured", func(t *testing.T) {
		p, err := NewProvider("test", inference.ProviderConfig{
			APIKey: "test-key",
		})
		require.NoError(t, err)

		models, err := p.ListModels(context.Background())
		require.NoError(t, err)
		assert.NotEmpty(t, models)
		// Should include known Claude models
		modelIDs := make([]string, len(models))
		for i, m := range models {
			modelIDs[i] = m.ID
		}
		assert.Contains(t, modelIDs, "claude-3-5-sonnet-20241022")
		assert.Contains(t, modelIDs, "claude-3-opus-20240229")
	})

	t.Run("returns error when closed", func(t *testing.T) {
		p, err := NewProvider("test", inference.ProviderConfig{
			APIKey: "test-key",
		})
		require.NoError(t, err)
		require.NoError(t, p.Close())

		_, err = p.ListModels(context.Background())
		assert.ErrorIs(t, err, inference.ErrProviderClosed)
	})
}

func TestProvider_Complete(t *testing.T) {
	t.Run("successful completion", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify request
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "/messages", r.URL.Path)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			assert.Equal(t, "test-api-key", r.Header.Get("x-api-key"))
			assert.Equal(t, anthropicVersion, r.Header.Get("anthropic-version"))

			// Parse request body
			var req anthropicMessageRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)
			assert.Equal(t, "claude-3-opus-20240229", req.Model)
			assert.Equal(t, "You are helpful", req.System)
			assert.Len(t, req.Messages, 1)
			assert.Equal(t, "user", req.Messages[0].Role)
			assert.Equal(t, "Hello", req.Messages[0].Content)

			// Send response
			resp := anthropicMessageResponse{
				ID:         "msg_123",
				Type:       "message",
				Role:       "assistant",
				Model:      "claude-3-opus-20240229",
				StopReason: "end_turn",
				Content: []anthropicContentBlock{
					{Type: "text", Text: "Hello! How can I help you?"},
				},
				Usage: anthropicUsage{
					InputTokens:  10,
					OutputTokens: 8,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		p, err := NewProvider("test", inference.ProviderConfig{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		})
		require.NoError(t, err)

		req := &inference.CompletionRequest{
			Model: "claude-3-opus-20240229",
			Messages: []inference.Message{
				{Role: "system", Content: "You are helpful"},
				{Role: "user", Content: "Hello"},
			},
		}

		resp, err := p.Complete(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, "msg_123", resp.ID)
		assert.Equal(t, "claude-3-opus-20240229", resp.Model)
		assert.Len(t, resp.Choices, 1)
		assert.Equal(t, "Hello! How can I help you?", resp.Choices[0].Message.Content)
		assert.Equal(t, "assistant", resp.Choices[0].Message.Role)
		assert.Equal(t, "end_turn", resp.Choices[0].FinishReason)
		assert.Equal(t, 10, resp.Usage.PromptTokens)
		assert.Equal(t, 8, resp.Usage.CompletionTokens)
		assert.Equal(t, 18, resp.Usage.TotalTokens)
	})

	t.Run("with max tokens", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req anthropicMessageRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)
			assert.Equal(t, 100, req.MaxTokens)

			resp := anthropicMessageResponse{
				ID:      "msg_123",
				Content: []anthropicContentBlock{{Type: "text", Text: "OK"}},
				Usage:   anthropicUsage{InputTokens: 5, OutputTokens: 1},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		p, err := NewProvider("test", inference.ProviderConfig{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		})
		require.NoError(t, err)

		maxTokens := 100
		req := &inference.CompletionRequest{
			Model:     "claude-3-haiku-20240307",
			Messages:  []inference.Message{{Role: "user", Content: "Hi"}},
			MaxTokens: &maxTokens,
		}

		_, err = p.Complete(context.Background(), req)
		require.NoError(t, err)
	})

	t.Run("with temperature and top_p", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req anthropicMessageRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)
			assert.NotNil(t, req.Temperature)
			assert.Equal(t, 0.7, *req.Temperature)
			assert.NotNil(t, req.TopP)
			assert.Equal(t, 0.9, *req.TopP)

			resp := anthropicMessageResponse{
				ID:      "msg_123",
				Content: []anthropicContentBlock{{Type: "text", Text: "OK"}},
				Usage:   anthropicUsage{InputTokens: 5, OutputTokens: 1},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		p, err := NewProvider("test", inference.ProviderConfig{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		})
		require.NoError(t, err)

		temp := 0.7
		topP := 0.9
		req := &inference.CompletionRequest{
			Model:       "claude-3-haiku-20240307",
			Messages:    []inference.Message{{Role: "user", Content: "Hi"}},
			Temperature: &temp,
			TopP:        &topP,
		}

		_, err = p.Complete(context.Background(), req)
		require.NoError(t, err)
	})

	t.Run("empty messages error", func(t *testing.T) {
		p, err := NewProvider("test", inference.ProviderConfig{
			APIKey: "test-api-key",
		})
		require.NoError(t, err)

		req := &inference.CompletionRequest{
			Model:    "claude-3-opus-20240229",
			Messages: []inference.Message{},
		}

		_, err = p.Complete(context.Background(), req)
		assert.ErrorIs(t, err, inference.ErrEmptyMessages)
	})

	t.Run("provider closed error", func(t *testing.T) {
		p, err := NewProvider("test", inference.ProviderConfig{
			APIKey: "test-api-key",
		})
		require.NoError(t, err)
		require.NoError(t, p.Close())

		req := &inference.CompletionRequest{
			Model:    "claude-3-opus-20240229",
			Messages: []inference.Message{{Role: "user", Content: "Hi"}},
		}

		_, err = p.Complete(context.Background(), req)
		assert.ErrorIs(t, err, inference.ErrProviderClosed)
	})

	t.Run("API error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			errResp := anthropicErrorResponse{
				Type: "error",
				Error: anthropicError{
					Type:    "invalid_request_error",
					Message: "Invalid model specified",
				},
			}
			_ = json.NewEncoder(w).Encode(errResp)
		}))
		defer server.Close()

		p, err := NewProvider("test", inference.ProviderConfig{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		})
		require.NoError(t, err)

		req := &inference.CompletionRequest{
			Model:    "invalid-model",
			Messages: []inference.Message{{Role: "user", Content: "Hi"}},
		}

		_, err = p.Complete(context.Background(), req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid_request_error")
		assert.Contains(t, err.Error(), "Invalid model specified")
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("Internal server error"))
		}))
		defer server.Close()

		p, err := NewProvider("test", inference.ProviderConfig{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		})
		require.NoError(t, err)

		req := &inference.CompletionRequest{
			Model:    "claude-3-opus-20240229",
			Messages: []inference.Message{{Role: "user", Content: "Hi"}},
		}

		_, err = p.Complete(context.Background(), req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "500")
	})
}

func TestProvider_Stream(t *testing.T) {
	t.Run("successful streaming", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify request
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "text/event-stream", r.Header.Get("Accept"))

			var req anthropicMessageRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)
			assert.True(t, req.Stream)

			// Send SSE stream
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)

			// message_start event
			writeSSE(w, flusher, "message_start", `{"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant","content":[],"model":"claude-3-opus-20240229"}}`)

			// content_block_start event
			writeSSE(w, flusher, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)

			// content_block_delta events
			writeSSE(w, flusher, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`)
			writeSSE(w, flusher, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`)

			// content_block_stop event
			writeSSE(w, flusher, "content_block_stop", `{"type":"content_block_stop","index":0}`)

			// message_delta event
			writeSSE(w, flusher, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`)

			// message_stop event
			writeSSE(w, flusher, "message_stop", `{"type":"message_stop"}`)
		}))
		defer server.Close()

		p, err := NewProvider("test", inference.ProviderConfig{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		})
		require.NoError(t, err)

		req := &inference.CompletionRequest{
			Model:    "claude-3-opus-20240229",
			Messages: []inference.Message{{Role: "user", Content: "Hello"}},
		}

		reader, err := p.Stream(context.Background(), req)
		require.NoError(t, err)
		defer reader.Close()

		// Read chunks
		var chunks []*inference.CompletionChunk
		for {
			chunk, err := reader.Read()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			chunks = append(chunks, chunk)
		}

		// Verify we got content
		assert.NotEmpty(t, chunks)

		// Find content chunks
		var content strings.Builder
		for _, chunk := range chunks {
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta != nil {
				content.WriteString(chunk.Choices[0].Delta.Content)
			}
		}
		assert.Equal(t, "Hello world", content.String())
	})

	t.Run("empty messages error", func(t *testing.T) {
		p, err := NewProvider("test", inference.ProviderConfig{
			APIKey: "test-api-key",
		})
		require.NoError(t, err)

		req := &inference.CompletionRequest{
			Model:    "claude-3-opus-20240229",
			Messages: []inference.Message{},
		}

		_, err = p.Stream(context.Background(), req)
		assert.ErrorIs(t, err, inference.ErrEmptyMessages)
	})

	t.Run("provider closed error", func(t *testing.T) {
		p, err := NewProvider("test", inference.ProviderConfig{
			APIKey: "test-api-key",
		})
		require.NoError(t, err)
		require.NoError(t, p.Close())

		req := &inference.CompletionRequest{
			Model:    "claude-3-opus-20240229",
			Messages: []inference.Message{{Role: "user", Content: "Hi"}},
		}

		_, err = p.Stream(context.Background(), req)
		assert.ErrorIs(t, err, inference.ErrProviderClosed)
	})

	t.Run("API error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			errResp := anthropicErrorResponse{
				Type: "error",
				Error: anthropicError{
					Type:    "authentication_error",
					Message: "Invalid API key",
				},
			}
			_ = json.NewEncoder(w).Encode(errResp)
		}))
		defer server.Close()

		p, err := NewProvider("test", inference.ProviderConfig{
			APIKey:  "invalid-key",
			BaseURL: server.URL,
		})
		require.NoError(t, err)

		req := &inference.CompletionRequest{
			Model:    "claude-3-opus-20240229",
			Messages: []inference.Message{{Role: "user", Content: "Hi"}},
		}

		_, err = p.Stream(context.Background(), req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "authentication_error")
	})
}

func TestProvider_Health(t *testing.T) {
	t.Run("healthy when API responds", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Accept any request to /messages as healthy
			resp := anthropicMessageResponse{
				ID:      "msg_health",
				Content: []anthropicContentBlock{{Type: "text", Text: "ok"}},
				Usage:   anthropicUsage{InputTokens: 1, OutputTokens: 1},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		p, err := NewProvider("test", inference.ProviderConfig{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		})
		require.NoError(t, err)

		err = p.Health(context.Background())
		assert.NoError(t, err)
	})

	t.Run("healthy on 400 error (API reachable)", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			errResp := anthropicErrorResponse{
				Error: anthropicError{Type: "invalid_request", Message: "test"},
			}
			_ = json.NewEncoder(w).Encode(errResp)
		}))
		defer server.Close()

		p, err := NewProvider("test", inference.ProviderConfig{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		})
		require.NoError(t, err)

		err = p.Health(context.Background())
		assert.NoError(t, err) // 400 is still considered reachable
	})

	t.Run("unhealthy on 500 error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		p, err := NewProvider("test", inference.ProviderConfig{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		})
		require.NoError(t, err)

		err = p.Health(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "500")
	})

	t.Run("unhealthy when server unreachable", func(t *testing.T) {
		p, err := NewProvider("test", inference.ProviderConfig{
			APIKey:  "test-api-key",
			BaseURL: "http://localhost:1", // Unreachable
			Timeout: 100 * time.Millisecond,
		})
		require.NoError(t, err)

		err = p.Health(context.Background())
		assert.Error(t, err)
	})

	t.Run("error when closed", func(t *testing.T) {
		p, err := NewProvider("test", inference.ProviderConfig{
			APIKey: "test-api-key",
		})
		require.NoError(t, err)
		require.NoError(t, p.Close())

		err = p.Health(context.Background())
		assert.ErrorIs(t, err, inference.ErrProviderClosed)
	})
}

func TestProvider_Close(t *testing.T) {
	p, err := NewProvider("test", inference.ProviderConfig{
		APIKey: "test-api-key",
	})
	require.NoError(t, err)

	// First close should succeed
	err = p.Close()
	assert.NoError(t, err)

	// Subsequent operations should fail
	_, err = p.ListModels(context.Background())
	assert.ErrorIs(t, err, inference.ErrProviderClosed)
}

func TestConvertRequest(t *testing.T) {
	t.Run("converts system message", func(t *testing.T) {
		req := &inference.CompletionRequest{
			Model: "claude-3-opus-20240229",
			Messages: []inference.Message{
				{Role: "system", Content: "You are helpful"},
				{Role: "user", Content: "Hi"},
			},
		}

		anthropicReq := convertRequest(req)
		assert.Equal(t, "You are helpful", anthropicReq.System)
		assert.Len(t, anthropicReq.Messages, 1)
		assert.Equal(t, "user", anthropicReq.Messages[0].Role)
	})

	t.Run("default max tokens", func(t *testing.T) {
		req := &inference.CompletionRequest{
			Model:    "claude-3-opus-20240229",
			Messages: []inference.Message{{Role: "user", Content: "Hi"}},
		}

		anthropicReq := convertRequest(req)
		assert.Equal(t, 4096, anthropicReq.MaxTokens)
	})

	t.Run("custom max tokens", func(t *testing.T) {
		maxTokens := 100
		req := &inference.CompletionRequest{
			Model:     "claude-3-opus-20240229",
			Messages:  []inference.Message{{Role: "user", Content: "Hi"}},
			MaxTokens: &maxTokens,
		}

		anthropicReq := convertRequest(req)
		assert.Equal(t, 100, anthropicReq.MaxTokens)
	})

	t.Run("stop sequences", func(t *testing.T) {
		req := &inference.CompletionRequest{
			Model:    "claude-3-opus-20240229",
			Messages: []inference.Message{{Role: "user", Content: "Hi"}},
			Stop:     []string{"END", "STOP"},
		}

		anthropicReq := convertRequest(req)
		assert.Equal(t, []string{"END", "STOP"}, anthropicReq.StopSequences)
	})
}

func TestConvertResponse(t *testing.T) {
	resp := &anthropicMessageResponse{
		ID:         "msg_123",
		Type:       "message",
		Role:       "assistant",
		Model:      "claude-3-opus-20240229",
		StopReason: "end_turn",
		Content: []anthropicContentBlock{
			{Type: "text", Text: "Hello "},
			{Type: "text", Text: "world!"},
		},
		Usage: anthropicUsage{
			InputTokens:  10,
			OutputTokens: 5,
		},
	}

	converted := convertResponse(resp)

	assert.Equal(t, "msg_123", converted.ID)
	assert.Equal(t, "chat.completion", converted.Object)
	assert.Equal(t, "claude-3-opus-20240229", converted.Model)
	assert.Len(t, converted.Choices, 1)
	assert.Equal(t, "Hello world!", converted.Choices[0].Message.Content)
	assert.Equal(t, "assistant", converted.Choices[0].Message.Role)
	assert.Equal(t, "end_turn", converted.Choices[0].FinishReason)
	assert.Equal(t, 10, converted.Usage.PromptTokens)
	assert.Equal(t, 5, converted.Usage.CompletionTokens)
	assert.Equal(t, 15, converted.Usage.TotalTokens)
}

// writeSSE writes an SSE event to the response writer.
func writeSSE(w http.ResponseWriter, flusher http.Flusher, event, data string) {
	_, _ = w.Write([]byte("event: " + event + "\n"))
	_, _ = w.Write([]byte("data: " + data + "\n\n"))
	flusher.Flush()
}
