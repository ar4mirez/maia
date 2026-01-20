package inference

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// MockProvider implements Provider for testing purposes.
type MockProvider struct {
	name      string
	models    []string
	closed    bool
	mu        sync.RWMutex
	response  string
	delay     time.Duration
	healthErr error
}

// NewMockProvider creates a new mock provider.
func NewMockProvider(name string) *MockProvider {
	return &MockProvider{
		name: name,
		models: []string{
			"mock-model-1",
			"mock-model-2",
		},
		response: "This is a mock response from the inference provider.",
	}
}

// WithModels sets the available models for the mock provider.
func (m *MockProvider) WithModels(models []string) *MockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.models = models
	return m
}

// WithResponse sets the response content for the mock provider.
func (m *MockProvider) WithResponse(response string) *MockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.response = response
	return m
}

// WithDelay sets an artificial delay for responses.
func (m *MockProvider) WithDelay(delay time.Duration) *MockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.delay = delay
	return m
}

// WithError sets an error to return from Health checks.
func (m *MockProvider) WithError(err error) *MockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthErr = err
	return m
}

// Complete implements Provider.Complete.
func (m *MockProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, ErrProviderClosed
	}

	if len(req.Messages) == 0 {
		return nil, ErrEmptyMessages
	}

	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return &CompletionResponse{
		ID:      fmt.Sprintf("mock-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []Choice{
			{
				Index: 0,
				Message: &Message{
					Role:    "assistant",
					Content: m.response,
				},
				FinishReason: "stop",
			},
		},
		Usage: &Usage{
			PromptTokens:     len(strings.Fields(req.Messages[len(req.Messages)-1].Content)),
			CompletionTokens: len(strings.Fields(m.response)),
			TotalTokens:      len(strings.Fields(req.Messages[len(req.Messages)-1].Content)) + len(strings.Fields(m.response)),
		},
	}, nil
}

// Stream implements Provider.Stream.
func (m *MockProvider) Stream(ctx context.Context, req *CompletionRequest) (StreamReader, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, ErrProviderClosed
	}

	if len(req.Messages) == 0 {
		return nil, ErrEmptyMessages
	}

	// Split response into words for streaming
	words := strings.Fields(m.response)
	return &mockStreamReader{
		id:      fmt.Sprintf("mock-stream-%d", time.Now().UnixNano()),
		model:   req.Model,
		words:   words,
		current: 0,
		delay:   m.delay / time.Duration(len(words)+1),
	}, nil
}

// ListModels implements Provider.ListModels.
func (m *MockProvider) ListModels(ctx context.Context) ([]Model, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, ErrProviderClosed
	}

	models := make([]Model, len(m.models))
	for i, modelID := range m.models {
		models[i] = Model{
			ID:       modelID,
			Object:   "model",
			OwnedBy:  "mock",
			Provider: m.name,
		}
	}
	return models, nil
}

// Name implements Provider.Name.
func (m *MockProvider) Name() string {
	return m.name
}

// SupportsModel implements Provider.SupportsModel.
func (m *MockProvider) SupportsModel(modelID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, model := range m.models {
		if model == modelID {
			return true
		}
		// Support wildcard matching
		if strings.HasSuffix(model, "*") {
			prefix := strings.TrimSuffix(model, "*")
			if strings.HasPrefix(modelID, prefix) {
				return true
			}
		}
	}
	return false
}

// Health implements Provider.Health.
func (m *MockProvider) Health(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return ErrProviderClosed
	}
	if m.healthErr != nil {
		return m.healthErr
	}
	return nil
}

// Close implements Provider.Close.
func (m *MockProvider) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.closed = true
	return nil
}

// mockStreamReader implements StreamReader for mock streaming.
type mockStreamReader struct {
	id      string
	model   string
	words   []string
	current int
	delay   time.Duration
	closed  bool
}

// Read implements StreamReader.Read.
func (s *mockStreamReader) Read() (*CompletionChunk, error) {
	if s.closed {
		return nil, ErrProviderClosed
	}

	if s.current >= len(s.words) {
		// Send final chunk with finish_reason
		if s.current == len(s.words) {
			s.current++
			return &CompletionChunk{
				ID:      s.id,
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   s.model,
				Choices: []Choice{
					{
						Index: 0,
						Delta: &Message{
							Role:    "assistant",
							Content: "",
						},
						FinishReason: "stop",
					},
				},
			}, nil
		}
		return nil, io.EOF
	}

	if s.delay > 0 {
		time.Sleep(s.delay)
	}

	word := s.words[s.current]
	s.current++

	// Add space after each word except the last
	content := word
	if s.current < len(s.words) {
		content += " "
	}

	return &CompletionChunk{
		ID:      s.id,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   s.model,
		Choices: []Choice{
			{
				Index: 0,
				Delta: &Message{
					Role:    "assistant",
					Content: content,
				},
			},
		},
	}, nil
}

// Close implements StreamReader.Close.
func (s *mockStreamReader) Close() error {
	s.closed = true
	return nil
}
