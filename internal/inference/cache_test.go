package inference

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCache(t *testing.T) {
	cfg := CacheConfig{
		Enabled:    true,
		TTL:        time.Hour,
		MaxEntries: 100,
	}

	cache := NewCache(cfg)
	assert.NotNil(t, cache)
	assert.Equal(t, 100, cache.maxSize)
}

func TestCache_GetSet(t *testing.T) {
	cfg := CacheConfig{
		Enabled:    true,
		TTL:        time.Hour,
		MaxEntries: 100,
	}
	cache := NewCache(cfg)

	req := &CompletionRequest{
		Model: "test-model",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	}

	resp := &CompletionResponse{
		ID:    "test-id",
		Model: "test-model",
		Choices: []Choice{
			{
				Index:   0,
				Message: &Message{Role: "assistant", Content: "Hi there!"},
			},
		},
	}

	// Initially no cache
	cached := cache.Get(context.Background(), req)
	assert.Nil(t, cached)

	// Set response
	cache.Set(context.Background(), req, resp)

	// Now should be cached
	cached = cache.Get(context.Background(), req)
	require.NotNil(t, cached)
	assert.Equal(t, "test-id", cached.ID)
	assert.Equal(t, "Hi there!", cached.Choices[0].Message.Content)
}

func TestCache_Disabled(t *testing.T) {
	cfg := CacheConfig{
		Enabled: false,
		TTL:     time.Hour,
	}
	cache := NewCache(cfg)

	req := &CompletionRequest{
		Model: "test-model",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	}

	resp := &CompletionResponse{
		ID:      "test-id",
		Choices: []Choice{{Index: 0}},
	}

	// Set should be no-op when disabled
	cache.Set(context.Background(), req, resp)

	// Get should return nil when disabled
	cached := cache.Get(context.Background(), req)
	assert.Nil(t, cached)
}

func TestCache_TTLExpiration(t *testing.T) {
	cfg := CacheConfig{
		Enabled:    true,
		TTL:        50 * time.Millisecond,
		MaxEntries: 100,
	}
	cache := NewCache(cfg)

	req := &CompletionRequest{
		Model: "test-model",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	}

	resp := &CompletionResponse{
		ID:      "test-id",
		Choices: []Choice{{Index: 0, Message: &Message{Content: "Hi"}}},
	}

	cache.Set(context.Background(), req, resp)

	// Should be cached initially
	cached := cache.Get(context.Background(), req)
	require.NotNil(t, cached)

	// Wait for expiration
	time.Sleep(60 * time.Millisecond)

	// Should be expired now
	cached = cache.Get(context.Background(), req)
	assert.Nil(t, cached)
}

func TestCache_LRUEviction(t *testing.T) {
	cfg := CacheConfig{
		Enabled:    true,
		TTL:        time.Hour,
		MaxEntries: 3,
	}
	cache := NewCache(cfg)

	// Add 3 entries
	for i := 0; i < 3; i++ {
		req := &CompletionRequest{
			Model: "test-model",
			Messages: []Message{
				{Role: "user", Content: string(rune('A' + i))},
			},
		}
		resp := &CompletionResponse{
			ID:      string(rune('A' + i)),
			Choices: []Choice{{Index: 0, Message: &Message{Content: "resp"}}},
		}
		cache.Set(context.Background(), req, resp)
	}

	assert.Equal(t, 3, len(cache.entries))

	// Add a 4th entry - should evict the LRU (first one added)
	req4 := &CompletionRequest{
		Model: "test-model",
		Messages: []Message{
			{Role: "user", Content: "D"},
		},
	}
	resp4 := &CompletionResponse{
		ID:      "D",
		Choices: []Choice{{Index: 0, Message: &Message{Content: "resp"}}},
	}
	cache.Set(context.Background(), req4, resp4)

	assert.Equal(t, 3, len(cache.entries))

	// First entry should be evicted
	req1 := &CompletionRequest{
		Model: "test-model",
		Messages: []Message{
			{Role: "user", Content: "A"},
		},
	}
	cached := cache.Get(context.Background(), req1)
	assert.Nil(t, cached)

	// Last entry should still be there
	cached = cache.Get(context.Background(), req4)
	require.NotNil(t, cached)
	assert.Equal(t, "D", cached.ID)
}

func TestCache_Invalidate(t *testing.T) {
	cfg := CacheConfig{
		Enabled:    true,
		TTL:        time.Hour,
		MaxEntries: 100,
	}
	cache := NewCache(cfg)

	req := &CompletionRequest{
		Model: "test-model",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	}

	resp := &CompletionResponse{
		ID:      "test-id",
		Choices: []Choice{{Index: 0, Message: &Message{Content: "Hi"}}},
	}

	cache.Set(context.Background(), req, resp)

	// Should be cached
	cached := cache.Get(context.Background(), req)
	require.NotNil(t, cached)

	// Invalidate
	cache.Invalidate(context.Background(), req)

	// Should no longer be cached
	cached = cache.Get(context.Background(), req)
	assert.Nil(t, cached)
}

func TestCache_Clear(t *testing.T) {
	cfg := CacheConfig{
		Enabled:    true,
		TTL:        time.Hour,
		MaxEntries: 100,
	}
	cache := NewCache(cfg)

	// Add multiple entries
	for i := 0; i < 5; i++ {
		req := &CompletionRequest{
			Model: "test-model",
			Messages: []Message{
				{Role: "user", Content: string(rune('A' + i))},
			},
		}
		resp := &CompletionResponse{
			ID:      string(rune('A' + i)),
			Choices: []Choice{{Index: 0, Message: &Message{Content: "resp"}}},
		}
		cache.Set(context.Background(), req, resp)
	}

	assert.Equal(t, 5, len(cache.entries))

	// Clear
	cache.Clear(context.Background())

	assert.Equal(t, 0, len(cache.entries))
	assert.Equal(t, 0, len(cache.order))
}

func TestCache_Stats(t *testing.T) {
	cfg := CacheConfig{
		Enabled:    true,
		TTL:        time.Hour,
		MaxEntries: 100,
	}
	cache := NewCache(cfg)

	req := &CompletionRequest{
		Model: "test-model",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	}

	resp := &CompletionResponse{
		ID:      "test-id",
		Choices: []Choice{{Index: 0, Message: &Message{Content: "Hi"}}},
	}

	// Miss
	cache.Get(context.Background(), req)
	stats := cache.Stats()
	assert.Equal(t, int64(1), stats.Misses)
	assert.Equal(t, int64(0), stats.Hits)

	// Set
	cache.Set(context.Background(), req, resp)

	// Hit
	cache.Get(context.Background(), req)
	stats = cache.Stats()
	assert.Equal(t, int64(1), stats.Misses)
	assert.Equal(t, int64(1), stats.Hits)
	assert.Equal(t, 1, stats.Size)
}

func TestCache_GenerateKey(t *testing.T) {
	cfg := CacheConfig{Enabled: true}
	cache := NewCache(cfg)

	temp := 0.7
	req1 := &CompletionRequest{
		Model: "test-model",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
		Temperature: &temp,
	}

	req2 := &CompletionRequest{
		Model: "test-model",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
		Temperature: &temp,
	}

	// Same request should generate same key
	key1 := cache.generateKey(req1)
	key2 := cache.generateKey(req2)
	assert.Equal(t, key1, key2)

	// Different message should generate different key
	req3 := &CompletionRequest{
		Model: "test-model",
		Messages: []Message{
			{Role: "user", Content: "Goodbye"},
		},
		Temperature: &temp,
	}
	key3 := cache.generateKey(req3)
	assert.NotEqual(t, key1, key3)

	// Different temperature should generate different key
	temp2 := 0.8
	req4 := &CompletionRequest{
		Model: "test-model",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
		Temperature: &temp2,
	}
	key4 := cache.generateKey(req4)
	assert.NotEqual(t, key1, key4)
}

func TestCache_NoStreamCaching(t *testing.T) {
	cfg := CacheConfig{
		Enabled:    true,
		TTL:        time.Hour,
		MaxEntries: 100,
	}
	cache := NewCache(cfg)

	req := &CompletionRequest{
		Model:  "test-model",
		Stream: true, // Streaming request
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	}

	resp := &CompletionResponse{
		ID:      "test-id",
		Choices: []Choice{{Index: 0, Message: &Message{Content: "Hi"}}},
	}

	// Should not cache streaming requests
	cache.Set(context.Background(), req, resp)
	assert.Equal(t, 0, len(cache.entries))
}

func TestCache_ExportImport(t *testing.T) {
	cfg := CacheConfig{
		Enabled:    true,
		TTL:        time.Hour,
		MaxEntries: 100,
	}
	cache := NewCache(cfg)

	req := &CompletionRequest{
		Model: "test-model",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	}

	resp := &CompletionResponse{
		ID:    "test-id",
		Model: "test-model",
		Choices: []Choice{
			{Index: 0, Message: &Message{Role: "assistant", Content: "Hi"}},
		},
	}

	cache.Set(context.Background(), req, resp)

	// Export
	data, err := cache.Export()
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Create new cache and import
	cache2 := NewCache(cfg)
	err = cache2.Import(data)
	require.NoError(t, err)

	// Should have the same entry
	cached := cache2.Get(context.Background(), req)
	require.NotNil(t, cached)
	assert.Equal(t, "test-id", cached.ID)
}

func TestCachingRouter_Complete(t *testing.T) {
	cfg := CacheConfig{
		Enabled:    true,
		TTL:        time.Hour,
		MaxEntries: 100,
	}
	cache := NewCache(cfg)

	router := NewRouter(RoutingConfig{}, "mock")
	mockProvider := NewMockProvider("mock").WithResponse("Cached response")
	router.RegisterProvider(mockProvider)

	cachingRouter := NewCachingRouter(router, cache)

	req := &CompletionRequest{
		Model: "any-model",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	}

	// First call - should hit provider
	resp1, err := cachingRouter.Complete(context.Background(), req)
	require.NoError(t, err)
	assert.Contains(t, resp1.Choices[0].Message.Content, "Cached response")

	// Change provider response
	mockProvider.WithResponse("New response")

	// Second call - should hit cache
	resp2, err := cachingRouter.Complete(context.Background(), req)
	require.NoError(t, err)
	// Should still return cached response
	assert.Contains(t, resp2.Choices[0].Message.Content, "Cached response")

	// Check stats
	stats := cache.Stats()
	assert.Equal(t, int64(1), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)
}

func TestCachingRouter_Stream(t *testing.T) {
	cfg := CacheConfig{
		Enabled:    true,
		TTL:        time.Hour,
		MaxEntries: 100,
	}
	cache := NewCache(cfg)

	router := NewRouter(RoutingConfig{}, "mock")
	mockProvider := NewMockProvider("mock").WithResponse("Stream test")
	router.RegisterProvider(mockProvider)

	cachingRouter := NewCachingRouter(router, cache)

	req := &CompletionRequest{
		Model: "any-model",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	}

	// Stream should bypass cache
	stream, err := cachingRouter.Stream(context.Background(), req)
	require.NoError(t, err)
	defer stream.Close()

	// Should get streaming response
	chunk, err := stream.Read()
	require.NoError(t, err)
	assert.NotNil(t, chunk)
}
