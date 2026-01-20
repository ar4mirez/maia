package inference

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockProvider_Complete(t *testing.T) {
	provider := NewMockProvider("test-mock")

	req := &CompletionRequest{
		Model: "mock-model-1",
		Messages: []Message{
			{Role: "user", Content: "Hello, world!"},
		},
	}

	resp, err := provider.Complete(context.Background(), req)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.ID)
	assert.Equal(t, "mock-model-1", resp.Model)
	assert.Len(t, resp.Choices, 1)
	assert.Equal(t, "assistant", resp.Choices[0].Message.Role)
	assert.NotEmpty(t, resp.Choices[0].Message.Content)
	assert.Equal(t, "stop", resp.Choices[0].FinishReason)
}

func TestMockProvider_Complete_EmptyMessages(t *testing.T) {
	provider := NewMockProvider("test-mock")

	req := &CompletionRequest{
		Model:    "mock-model-1",
		Messages: []Message{},
	}

	_, err := provider.Complete(context.Background(), req)
	assert.ErrorIs(t, err, ErrEmptyMessages)
}

func TestMockProvider_Complete_Closed(t *testing.T) {
	provider := NewMockProvider("test-mock")
	provider.Close()

	req := &CompletionRequest{
		Model: "mock-model-1",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	}

	_, err := provider.Complete(context.Background(), req)
	assert.ErrorIs(t, err, ErrProviderClosed)
}

func TestMockProvider_Stream(t *testing.T) {
	provider := NewMockProvider("test-mock").
		WithResponse("Hello world from streaming")

	req := &CompletionRequest{
		Model: "mock-model-1",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	}

	stream, err := provider.Stream(context.Background(), req)
	require.NoError(t, err)
	defer stream.Close()

	var chunks []*CompletionChunk
	for {
		chunk, err := stream.Read()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		chunks = append(chunks, chunk)
	}

	assert.NotEmpty(t, chunks)

	// Accumulate content
	acc := NewAccumulator()
	for _, chunk := range chunks {
		acc.Add(chunk)
	}

	assert.Contains(t, acc.GetContent(), "Hello")
}

func TestMockProvider_ListModels(t *testing.T) {
	provider := NewMockProvider("test-mock")

	models, err := provider.ListModels(context.Background())
	require.NoError(t, err)
	assert.Len(t, models, 2)
	assert.Equal(t, "mock-model-1", models[0].ID)
	assert.Equal(t, "mock-model-2", models[1].ID)
}

func TestMockProvider_SupportsModel(t *testing.T) {
	provider := NewMockProvider("test-mock").
		WithModels([]string{"model-a", "model-b*"})

	assert.True(t, provider.SupportsModel("model-a"))
	assert.True(t, provider.SupportsModel("model-b"))
	assert.True(t, provider.SupportsModel("model-b-large"))
	assert.False(t, provider.SupportsModel("model-c"))
}

func TestMockProvider_Health(t *testing.T) {
	provider := NewMockProvider("test-mock")

	err := provider.Health(context.Background())
	assert.NoError(t, err)

	provider.Close()
	err = provider.Health(context.Background())
	assert.ErrorIs(t, err, ErrProviderClosed)
}

func TestMockProvider_WithDelay(t *testing.T) {
	provider := NewMockProvider("test-mock").
		WithDelay(50 * time.Millisecond)

	req := &CompletionRequest{
		Model: "mock-model-1",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	}

	start := time.Now()
	_, err := provider.Complete(context.Background(), req)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.GreaterOrEqual(t, elapsed, 50*time.Millisecond)
}

func TestAccumulator(t *testing.T) {
	acc := NewAccumulator()

	// Add first chunk
	acc.Add(&CompletionChunk{
		ID:      "chunk-1",
		Object:  "chat.completion.chunk",
		Created: 1234567890,
		Model:   "test-model",
		Choices: []Choice{
			{
				Index: 0,
				Delta: &Message{
					Role:    "assistant",
					Content: "Hello ",
				},
			},
		},
	})

	// Add second chunk
	acc.Add(&CompletionChunk{
		ID:      "chunk-1",
		Object:  "chat.completion.chunk",
		Created: 1234567890,
		Model:   "test-model",
		Choices: []Choice{
			{
				Index: 0,
				Delta: &Message{
					Content: "world!",
				},
				FinishReason: "stop",
			},
		},
	})

	assert.Equal(t, "chunk-1", acc.ID)
	assert.Equal(t, "test-model", acc.Model)
	assert.Equal(t, "Hello world!", acc.GetContent())

	resp := acc.ToResponse()
	assert.Equal(t, "chunk-1", resp.ID)
	assert.Equal(t, "test-model", resp.Model)
	assert.Len(t, resp.Choices, 1)
	assert.Equal(t, "Hello world!", resp.Choices[0].Message.Content)
	assert.Equal(t, "stop", resp.Choices[0].FinishReason)
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.False(t, cfg.Enabled)
	assert.Equal(t, "ollama", cfg.DefaultProvider)
	assert.NotNil(t, cfg.Providers)
	assert.NotNil(t, cfg.Routing.ModelMapping)
	assert.False(t, cfg.Cache.Enabled)
	assert.Equal(t, 24*time.Hour, cfg.Cache.TTL)
	assert.Equal(t, "inference:cache", cfg.Cache.Namespace)
}
