package openrouter

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
				BaseURL: "https://custom.openrouter.ai/api/v1/",
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
				Models: []string{"openai/gpt-4", "anthropic/claude-3-opus"},
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
	p, err := NewProvider("my-openrouter", inference.ProviderConfig{
		APIKey: "test-key",
	})
	require.NoError(t, err)
	assert.Equal(t, "my-openrouter", p.Name())
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
			modelID:       "openai/gpt-4",
			wantSupported: true,
		},
		{
			name:          "no models configured - supports any model",
			models:        nil,
			modelID:       "any/model-name",
			wantSupported: true,
		},
		{
			name:          "configured models exact match",
			models:        []string{"openai/gpt-4", "anthropic/claude-3-opus"},
			modelID:       "openai/gpt-4",
			wantSupported: true,
		},
		{
			name:          "configured models no match",
			models:        []string{"openai/gpt-4"},
			modelID:       "openai/gpt-3.5-turbo",
			wantSupported: false,
		},
		{
			name:          "configured models wildcard match",
			models:        []string{"openai/*"},
			modelID:       "openai/gpt-4",
			wantSupported: true,
		},
		{
			name:          "configured models wildcard match 2",
			models:        []string{"anthropic/*"},
			modelID:       "anthropic/claude-3-opus",
			wantSupported: true,
		},
		{
			name:          "configured models wildcard no match",
			models:        []string{"openai/*"},
			modelID:       "anthropic/claude-3-opus",
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
			Models: []string{"openai/gpt-4", "anthropic/claude-3-opus"},
		})
		require.NoError(t, err)

		models, err := p.ListModels(context.Background())
		require.NoError(t, err)
		assert.Len(t, models, 2)
		assert.Equal(t, "openai/gpt-4", models[0].ID)
		assert.Equal(t, "openrouter", models[0].OwnedBy)
		assert.Equal(t, "test", models[0].Provider)
	})

	t.Run("queries API when none configured", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/models", r.URL.Path)
			assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
			resp := openRouterModelsResponse{
				Data: []openRouterModel{
					{ID: "openai/gpt-4", Name: "GPT-4"},
					{ID: "anthropic/claude-3-opus", Name: "Claude 3 Opus"},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		p, err := NewProvider("test", inference.ProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
		})
		require.NoError(t, err)

		models, err := p.ListModels(context.Background())
		require.NoError(t, err)
		assert.Len(t, models, 2)
		assert.Equal(t, "openai/gpt-4", models[0].ID)
		assert.Equal(t, "test", models[0].Provider)
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

	t.Run("handles API error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			errResp := openRouterErrorResponse{
				Error: &openRouterError{
					Code:    "invalid_api_key",
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

		_, err = p.ListModels(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid_api_key")
	})
}

func TestProvider_Complete(t *testing.T) {
	t.Run("successful completion", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "/chat/completions", r.URL.Path)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			assert.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))
			assert.Equal(t, "https://github.com/ar4mirez/maia", r.Header.Get("HTTP-Referer"))
			assert.Equal(t, "MAIA", r.Header.Get("X-Title"))

			var req openRouterCompletionRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)
			assert.Equal(t, "openai/gpt-4", req.Model)
			assert.Len(t, req.Messages, 2)
			assert.False(t, req.Stream)

			resp := openRouterCompletionResponse{
				ID:      "chatcmpl-123",
				Object:  "chat.completion",
				Created: time.Now().Unix(),
				Model:   "openai/gpt-4",
				Choices: []openRouterChoice{
					{
						Index:        0,
						Message:      &openRouterMessage{Role: "assistant", Content: "Hello! How can I help?"},
						FinishReason: "stop",
					},
				},
				Usage: &openRouterUsage{
					PromptTokens:     10,
					CompletionTokens: 8,
					TotalTokens:      18,
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		p, err := NewProvider("test", inference.ProviderConfig{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		})
		require.NoError(t, err)

		req := &inference.CompletionRequest{
			Model: "openai/gpt-4",
			Messages: []inference.Message{
				{Role: "system", Content: "You are helpful"},
				{Role: "user", Content: "Hello"},
			},
		}

		resp, err := p.Complete(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, "chatcmpl-123", resp.ID)
		assert.Equal(t, "openai/gpt-4", resp.Model)
		assert.Len(t, resp.Choices, 1)
		assert.Equal(t, "Hello! How can I help?", resp.Choices[0].Message.Content)
		assert.Equal(t, 10, resp.Usage.PromptTokens)
		assert.Equal(t, 18, resp.Usage.TotalTokens)
	})

	t.Run("with optional parameters", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req openRouterCompletionRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)
			assert.NotNil(t, req.Temperature)
			assert.Equal(t, 0.7, *req.Temperature)
			assert.NotNil(t, req.TopP)
			assert.Equal(t, 0.9, *req.TopP)
			assert.NotNil(t, req.MaxTokens)
			assert.Equal(t, 100, *req.MaxTokens)
			assert.Equal(t, []string{"END"}, req.Stop)

			resp := openRouterCompletionResponse{
				ID:      "chatcmpl-123",
				Choices: []openRouterChoice{{Index: 0, Message: &openRouterMessage{Role: "assistant", Content: "OK"}}},
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
		maxTokens := 100
		req := &inference.CompletionRequest{
			Model:       "openai/gpt-4",
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
		p, err := NewProvider("test", inference.ProviderConfig{
			APIKey: "test-api-key",
		})
		require.NoError(t, err)

		req := &inference.CompletionRequest{
			Model:    "openai/gpt-4",
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
			Model:    "openai/gpt-4",
			Messages: []inference.Message{{Role: "user", Content: "Hi"}},
		}

		_, err = p.Complete(context.Background(), req)
		assert.ErrorIs(t, err, inference.ErrProviderClosed)
	})

	t.Run("API error with message", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			errResp := openRouterErrorResponse{
				Error: &openRouterError{
					Code:    "invalid_model",
					Message: "Model not found",
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
			Model:    "invalid/model",
			Messages: []inference.Message{{Role: "user", Content: "Hi"}},
		}

		_, err = p.Complete(context.Background(), req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid_model")
		assert.Contains(t, err.Error(), "Model not found")
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
			Model:    "openai/gpt-4",
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

			var req openRouterCompletionRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)
			assert.True(t, req.Stream)

			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)

			// Send chunks
			chunks := []openRouterCompletionResponse{
				{
					ID:      "chatcmpl-123",
					Object:  "chat.completion.chunk",
					Model:   "openai/gpt-4",
					Choices: []openRouterChoice{{Index: 0, Delta: &openRouterMessage{Content: "Hello"}}},
				},
				{
					ID:      "chatcmpl-123",
					Object:  "chat.completion.chunk",
					Model:   "openai/gpt-4",
					Choices: []openRouterChoice{{Index: 0, Delta: &openRouterMessage{Content: " world"}}},
				},
				{
					ID:      "chatcmpl-123",
					Object:  "chat.completion.chunk",
					Model:   "openai/gpt-4",
					Choices: []openRouterChoice{{Index: 0, FinishReason: "stop"}},
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
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		})
		require.NoError(t, err)

		req := &inference.CompletionRequest{
			Model:    "openai/gpt-4",
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
		p, err := NewProvider("test", inference.ProviderConfig{
			APIKey: "test-api-key",
		})
		require.NoError(t, err)

		req := &inference.CompletionRequest{
			Model:    "openai/gpt-4",
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
			Model:    "openai/gpt-4",
			Messages: []inference.Message{{Role: "user", Content: "Hi"}},
		}

		_, err = p.Stream(context.Background(), req)
		assert.ErrorIs(t, err, inference.ErrProviderClosed)
	})

	t.Run("API error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			errResp := openRouterErrorResponse{
				Error: &openRouterError{
					Code:    "rate_limit_exceeded",
					Message: "Rate limit exceeded",
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
			Model:    "openai/gpt-4",
			Messages: []inference.Message{{Role: "user", Content: "Hi"}},
		}

		_, err = p.Stream(context.Background(), req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "rate_limit_exceeded")
	})
}

func TestProvider_Health(t *testing.T) {
	t.Run("healthy when API responds", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/models", r.URL.Path)
			resp := openRouterModelsResponse{
				Data: []openRouterModel{{ID: "openai/gpt-4"}},
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

	t.Run("unhealthy on error response", func(t *testing.T) {
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
			BaseURL: "http://localhost:1",
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
		Model: "openai/gpt-4",
		Messages: []inference.Message{
			{Role: "system", Content: "You are helpful"},
			{Role: "user", Content: "Hi"},
		},
		Temperature: &temp,
		TopP:        &topP,
		MaxTokens:   &maxTokens,
		Stop:        []string{"END"},
	}

	orReq := convertRequest(req)

	assert.Equal(t, "openai/gpt-4", orReq.Model)
	assert.Len(t, orReq.Messages, 2)
	assert.Equal(t, "system", orReq.Messages[0].Role)
	assert.Equal(t, "user", orReq.Messages[1].Role)
	assert.Equal(t, 0.7, *orReq.Temperature)
	assert.Equal(t, 0.9, *orReq.TopP)
	assert.Equal(t, 100, *orReq.MaxTokens)
	assert.Equal(t, []string{"END"}, orReq.Stop)
}

func TestConvertResponse(t *testing.T) {
	resp := &openRouterCompletionResponse{
		ID:                "chatcmpl-123",
		Object:            "chat.completion",
		Created:           1234567890,
		Model:             "openai/gpt-4",
		SystemFingerprint: "fp_123",
		Choices: []openRouterChoice{
			{
				Index:        0,
				Message:      &openRouterMessage{Role: "assistant", Content: "Hello!"},
				FinishReason: "stop",
			},
		},
		Usage: &openRouterUsage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}

	converted := convertResponse(resp)

	assert.Equal(t, "chatcmpl-123", converted.ID)
	assert.Equal(t, "chat.completion", converted.Object)
	assert.Equal(t, int64(1234567890), converted.Created)
	assert.Equal(t, "openai/gpt-4", converted.Model)
	assert.Equal(t, "fp_123", converted.SystemFingerprint)
	assert.Len(t, converted.Choices, 1)
	assert.Equal(t, "Hello!", converted.Choices[0].Message.Content)
	assert.Equal(t, "stop", converted.Choices[0].FinishReason)
	assert.Equal(t, 10, converted.Usage.PromptTokens)
	assert.Equal(t, 15, converted.Usage.TotalTokens)
}

func TestConvertResponse_NilUsage(t *testing.T) {
	resp := &openRouterCompletionResponse{
		ID:      "chatcmpl-123",
		Choices: []openRouterChoice{{Index: 0, Message: &openRouterMessage{Content: "Hi"}}},
		Usage:   nil,
	}

	converted := convertResponse(resp)
	assert.Nil(t, converted.Usage)
}

func TestConvertChunk(t *testing.T) {
	chunk := &openRouterCompletionResponse{
		ID:      "chatcmpl-123",
		Object:  "chat.completion.chunk",
		Created: 1234567890,
		Model:   "openai/gpt-4",
		Choices: []openRouterChoice{
			{
				Index: 0,
				Delta: &openRouterMessage{Role: "assistant", Content: "Hello"},
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
