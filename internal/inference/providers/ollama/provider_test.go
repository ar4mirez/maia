package ollama

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
		name string
		cfg  inference.ProviderConfig
	}{
		{
			name: "default config",
			cfg:  inference.ProviderConfig{},
		},
		{
			name: "with custom base URL",
			cfg: inference.ProviderConfig{
				BaseURL: "http://custom:11434/v1/",
			},
		},
		{
			name: "with timeout",
			cfg: inference.ProviderConfig{
				Timeout: 30 * time.Second,
			},
		},
		{
			name: "with models",
			cfg: inference.ProviderConfig{
				Models: []string{"llama2", "mistral"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewProvider("test", tt.cfg)
			require.NoError(t, err)
			assert.NotNil(t, p)
			assert.Equal(t, "test", p.Name())
		})
	}
}

func TestProvider_Name(t *testing.T) {
	p, err := NewProvider("my-ollama", inference.ProviderConfig{})
	require.NoError(t, err)
	assert.Equal(t, "my-ollama", p.Name())
}

func TestProvider_SupportsModel(t *testing.T) {
	tests := []struct {
		name          string
		models        []string
		modelID       string
		wantSupported bool
	}{
		{
			name:          "no models configured - supports any",
			models:        nil,
			modelID:       "llama2",
			wantSupported: true,
		},
		{
			name:          "no models configured - supports any model",
			models:        nil,
			modelID:       "any-model-name",
			wantSupported: true,
		},
		{
			name:          "configured models exact match",
			models:        []string{"llama2", "mistral"},
			modelID:       "llama2",
			wantSupported: true,
		},
		{
			name:          "configured models no match",
			models:        []string{"llama2", "mistral"},
			modelID:       "llama3",
			wantSupported: false,
		},
		{
			name:          "configured models wildcard match",
			models:        []string{"llama*"},
			modelID:       "llama2",
			wantSupported: true,
		},
		{
			name:          "configured models wildcard match 2",
			models:        []string{"llama*"},
			modelID:       "llama3:8b",
			wantSupported: true,
		},
		{
			name:          "configured models wildcard no match",
			models:        []string{"llama*"},
			modelID:       "mistral",
			wantSupported: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewProvider("test", inference.ProviderConfig{
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
			Models: []string{"llama2", "mistral"},
		})
		require.NoError(t, err)

		models, err := p.ListModels(context.Background())
		require.NoError(t, err)
		assert.Len(t, models, 2)
		assert.Equal(t, "llama2", models[0].ID)
		assert.Equal(t, "ollama", models[0].OwnedBy)
		assert.Equal(t, "test", models[0].Provider)
	})

	t.Run("queries API when none configured", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/models", r.URL.Path)
			resp := ollamaModelsResponse{
				Object: "list",
				Data: []ollamaModel{
					{ID: "llama2", Object: "model", OwnedBy: "ollama", Created: 1234567890},
					{ID: "mistral", Object: "model", OwnedBy: "ollama", Created: 1234567891},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		p, err := NewProvider("test", inference.ProviderConfig{
			BaseURL: server.URL,
		})
		require.NoError(t, err)

		models, err := p.ListModels(context.Background())
		require.NoError(t, err)
		assert.Len(t, models, 2)
		assert.Equal(t, "llama2", models[0].ID)
		assert.Equal(t, "test", models[0].Provider)
	})

	t.Run("returns error when closed", func(t *testing.T) {
		p, err := NewProvider("test", inference.ProviderConfig{})
		require.NoError(t, err)
		require.NoError(t, p.Close())

		_, err = p.ListModels(context.Background())
		assert.ErrorIs(t, err, inference.ErrProviderClosed)
	})

	t.Run("handles API error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal error"))
		}))
		defer server.Close()

		p, err := NewProvider("test", inference.ProviderConfig{
			BaseURL: server.URL,
		})
		require.NoError(t, err)

		_, err = p.ListModels(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "500")
	})
}

func TestProvider_Complete(t *testing.T) {
	t.Run("successful completion", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "/chat/completions", r.URL.Path)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			var req ollamaCompletionRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)
			assert.Equal(t, "llama2", req.Model)
			assert.Len(t, req.Messages, 2)
			assert.False(t, req.Stream)

			resp := ollamaCompletionResponse{
				ID:      "chatcmpl-123",
				Object:  "chat.completion",
				Created: time.Now().Unix(),
				Model:   "llama2",
				Choices: []ollamaChoice{
					{
						Index:        0,
						Message:      &ollamaMessage{Role: "assistant", Content: "Hello! How can I help?"},
						FinishReason: "stop",
					},
				},
				Usage: &ollamaUsage{
					PromptTokens:     10,
					CompletionTokens: 8,
					TotalTokens:      18,
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		p, err := NewProvider("test", inference.ProviderConfig{
			BaseURL: server.URL,
		})
		require.NoError(t, err)

		req := &inference.CompletionRequest{
			Model: "llama2",
			Messages: []inference.Message{
				{Role: "system", Content: "You are helpful"},
				{Role: "user", Content: "Hello"},
			},
		}

		resp, err := p.Complete(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, "chatcmpl-123", resp.ID)
		assert.Equal(t, "llama2", resp.Model)
		assert.Len(t, resp.Choices, 1)
		assert.Equal(t, "Hello! How can I help?", resp.Choices[0].Message.Content)
		assert.Equal(t, 10, resp.Usage.PromptTokens)
		assert.Equal(t, 18, resp.Usage.TotalTokens)
	})

	t.Run("with optional parameters", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req ollamaCompletionRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)
			assert.NotNil(t, req.Temperature)
			assert.Equal(t, 0.7, *req.Temperature)
			assert.NotNil(t, req.TopP)
			assert.Equal(t, 0.9, *req.TopP)
			assert.NotNil(t, req.MaxTokens)
			assert.Equal(t, 100, *req.MaxTokens)
			assert.Equal(t, []string{"END"}, req.Stop)

			resp := ollamaCompletionResponse{
				ID:      "chatcmpl-123",
				Choices: []ollamaChoice{{Index: 0, Message: &ollamaMessage{Role: "assistant", Content: "OK"}}},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		p, err := NewProvider("test", inference.ProviderConfig{
			BaseURL: server.URL,
		})
		require.NoError(t, err)

		temp := 0.7
		topP := 0.9
		maxTokens := 100
		req := &inference.CompletionRequest{
			Model:       "llama2",
			Messages:    []inference.Message{{Role: "user", Content: "Hi"}},
			Temperature: &temp,
			TopP:        &topP,
			MaxTokens:   &maxTokens,
			Stop:        []string{"END"},
		}

		_, err = p.Complete(context.Background(), req)
		require.NoError(t, err)
	})

	t.Run("empty messages error", func(t *testing.T) {
		p, err := NewProvider("test", inference.ProviderConfig{})
		require.NoError(t, err)

		req := &inference.CompletionRequest{
			Model:    "llama2",
			Messages: []inference.Message{},
		}

		_, err = p.Complete(context.Background(), req)
		assert.ErrorIs(t, err, inference.ErrEmptyMessages)
	})

	t.Run("provider closed error", func(t *testing.T) {
		p, err := NewProvider("test", inference.ProviderConfig{})
		require.NoError(t, err)
		require.NoError(t, p.Close())

		req := &inference.CompletionRequest{
			Model:    "llama2",
			Messages: []inference.Message{{Role: "user", Content: "Hi"}},
		}

		_, err = p.Complete(context.Background(), req)
		assert.ErrorIs(t, err, inference.ErrProviderClosed)
	})

	t.Run("API error with message", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			errResp := ollamaErrorResponse{
				Error: &ollamaError{
					Type:    "invalid_request",
					Message: "Invalid model",
				},
			}
			_ = json.NewEncoder(w).Encode(errResp)
		}))
		defer server.Close()

		p, err := NewProvider("test", inference.ProviderConfig{
			BaseURL: server.URL,
		})
		require.NoError(t, err)

		req := &inference.CompletionRequest{
			Model:    "invalid",
			Messages: []inference.Message{{Role: "user", Content: "Hi"}},
		}

		_, err = p.Complete(context.Background(), req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Invalid model")
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("Internal server error"))
		}))
		defer server.Close()

		p, err := NewProvider("test", inference.ProviderConfig{
			BaseURL: server.URL,
		})
		require.NoError(t, err)

		req := &inference.CompletionRequest{
			Model:    "llama2",
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
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "text/event-stream", r.Header.Get("Accept"))

			var req ollamaCompletionRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)
			assert.True(t, req.Stream)

			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)

			// Send chunks
			chunks := []ollamaCompletionResponse{
				{
					ID:      "chatcmpl-123",
					Object:  "chat.completion.chunk",
					Model:   "llama2",
					Choices: []ollamaChoice{{Index: 0, Delta: &ollamaMessage{Content: "Hello"}}},
				},
				{
					ID:      "chatcmpl-123",
					Object:  "chat.completion.chunk",
					Model:   "llama2",
					Choices: []ollamaChoice{{Index: 0, Delta: &ollamaMessage{Content: " world"}}},
				},
				{
					ID:      "chatcmpl-123",
					Object:  "chat.completion.chunk",
					Model:   "llama2",
					Choices: []ollamaChoice{{Index: 0, FinishReason: "stop"}},
				},
			}

			for _, chunk := range chunks {
				data, _ := json.Marshal(chunk)
				_, _ = w.Write([]byte("data: " + string(data) + "\n\n"))
				flusher.Flush()
			}
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			flusher.Flush()
		}))
		defer server.Close()

		p, err := NewProvider("test", inference.ProviderConfig{
			BaseURL: server.URL,
		})
		require.NoError(t, err)

		req := &inference.CompletionRequest{
			Model:    "llama2",
			Messages: []inference.Message{{Role: "user", Content: "Hello"}},
		}

		reader, err := p.Stream(context.Background(), req)
		require.NoError(t, err)
		defer reader.Close()

		var chunks []*inference.CompletionChunk
		for {
			chunk, err := reader.Read()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			chunks = append(chunks, chunk)
		}

		assert.Len(t, chunks, 3)

		var content strings.Builder
		for _, chunk := range chunks {
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta != nil {
				content.WriteString(chunk.Choices[0].Delta.Content)
			}
		}
		assert.Equal(t, "Hello world", content.String())
	})

	t.Run("empty messages error", func(t *testing.T) {
		p, err := NewProvider("test", inference.ProviderConfig{})
		require.NoError(t, err)

		req := &inference.CompletionRequest{
			Model:    "llama2",
			Messages: []inference.Message{},
		}

		_, err = p.Stream(context.Background(), req)
		assert.ErrorIs(t, err, inference.ErrEmptyMessages)
	})

	t.Run("provider closed error", func(t *testing.T) {
		p, err := NewProvider("test", inference.ProviderConfig{})
		require.NoError(t, err)
		require.NoError(t, p.Close())

		req := &inference.CompletionRequest{
			Model:    "llama2",
			Messages: []inference.Message{{Role: "user", Content: "Hi"}},
		}

		_, err = p.Stream(context.Background(), req)
		assert.ErrorIs(t, err, inference.ErrProviderClosed)
	})

	t.Run("API error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("Service unavailable"))
		}))
		defer server.Close()

		p, err := NewProvider("test", inference.ProviderConfig{
			BaseURL: server.URL,
		})
		require.NoError(t, err)

		req := &inference.CompletionRequest{
			Model:    "llama2",
			Messages: []inference.Message{{Role: "user", Content: "Hi"}},
		}

		_, err = p.Stream(context.Background(), req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "503")
	})
}

func TestProvider_Health(t *testing.T) {
	t.Run("healthy when API responds", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/models", r.URL.Path)
			resp := ollamaModelsResponse{
				Object: "list",
				Data:   []ollamaModel{{ID: "llama2"}},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		p, err := NewProvider("test", inference.ProviderConfig{
			BaseURL: server.URL,
		})
		require.NoError(t, err)

		err = p.Health(context.Background())
		assert.NoError(t, err)
	})

	t.Run("unhealthy on error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		p, err := NewProvider("test", inference.ProviderConfig{
			BaseURL: server.URL,
		})
		require.NoError(t, err)

		err = p.Health(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "500")
	})

	t.Run("unhealthy when server unreachable", func(t *testing.T) {
		p, err := NewProvider("test", inference.ProviderConfig{
			BaseURL: "http://localhost:1",
			Timeout: 100 * time.Millisecond,
		})
		require.NoError(t, err)

		err = p.Health(context.Background())
		assert.Error(t, err)
	})

	t.Run("error when closed", func(t *testing.T) {
		p, err := NewProvider("test", inference.ProviderConfig{})
		require.NoError(t, err)
		require.NoError(t, p.Close())

		err = p.Health(context.Background())
		assert.ErrorIs(t, err, inference.ErrProviderClosed)
	})
}

func TestProvider_Close(t *testing.T) {
	p, err := NewProvider("test", inference.ProviderConfig{})
	require.NoError(t, err)

	err = p.Close()
	assert.NoError(t, err)

	_, err = p.ListModels(context.Background())
	assert.ErrorIs(t, err, inference.ErrProviderClosed)
}

func TestConvertRequest(t *testing.T) {
	temp := 0.7
	topP := 0.9
	maxTokens := 100
	req := &inference.CompletionRequest{
		Model: "llama2",
		Messages: []inference.Message{
			{Role: "system", Content: "You are helpful"},
			{Role: "user", Content: "Hi"},
		},
		Temperature: &temp,
		TopP:        &topP,
		MaxTokens:   &maxTokens,
		Stop:        []string{"END"},
	}

	ollamaReq := convertRequest(req)

	assert.Equal(t, "llama2", ollamaReq.Model)
	assert.Len(t, ollamaReq.Messages, 2)
	assert.Equal(t, "system", ollamaReq.Messages[0].Role)
	assert.Equal(t, "user", ollamaReq.Messages[1].Role)
	assert.Equal(t, 0.7, *ollamaReq.Temperature)
	assert.Equal(t, 0.9, *ollamaReq.TopP)
	assert.Equal(t, 100, *ollamaReq.MaxTokens)
	assert.Equal(t, []string{"END"}, ollamaReq.Stop)
}

func TestConvertResponse(t *testing.T) {
	resp := &ollamaCompletionResponse{
		ID:                "chatcmpl-123",
		Object:            "chat.completion",
		Created:           1234567890,
		Model:             "llama2",
		SystemFingerprint: "fp_123",
		Choices: []ollamaChoice{
			{
				Index:        0,
				Message:      &ollamaMessage{Role: "assistant", Content: "Hello!"},
				FinishReason: "stop",
			},
		},
		Usage: &ollamaUsage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}

	converted := convertResponse(resp)

	assert.Equal(t, "chatcmpl-123", converted.ID)
	assert.Equal(t, "chat.completion", converted.Object)
	assert.Equal(t, int64(1234567890), converted.Created)
	assert.Equal(t, "llama2", converted.Model)
	assert.Equal(t, "fp_123", converted.SystemFingerprint)
	assert.Len(t, converted.Choices, 1)
	assert.Equal(t, "Hello!", converted.Choices[0].Message.Content)
	assert.Equal(t, "stop", converted.Choices[0].FinishReason)
	assert.Equal(t, 10, converted.Usage.PromptTokens)
	assert.Equal(t, 15, converted.Usage.TotalTokens)
}

func TestConvertResponse_NilUsage(t *testing.T) {
	resp := &ollamaCompletionResponse{
		ID:      "chatcmpl-123",
		Choices: []ollamaChoice{{Index: 0, Message: &ollamaMessage{Content: "Hi"}}},
		Usage:   nil,
	}

	converted := convertResponse(resp)
	assert.Nil(t, converted.Usage)
}

func TestConvertChunk(t *testing.T) {
	chunk := &ollamaCompletionResponse{
		ID:      "chatcmpl-123",
		Object:  "chat.completion.chunk",
		Created: 1234567890,
		Model:   "llama2",
		Choices: []ollamaChoice{
			{
				Index: 0,
				Delta: &ollamaMessage{Role: "assistant", Content: "Hello"},
			},
		},
	}

	converted := convertChunk(chunk)

	assert.Equal(t, "chatcmpl-123", converted.ID)
	assert.Equal(t, "chat.completion.chunk", converted.Object)
	assert.Len(t, converted.Choices, 1)
	assert.NotNil(t, converted.Choices[0].Delta)
	assert.Equal(t, "Hello", converted.Choices[0].Delta.Content)
}
