package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(10, 10) // 10 rps, burst of 10

	// Should allow first 10 requests (burst)
	for i := 0; i < 10; i++ {
		assert.True(t, rl.Allow("test-key"), "request %d should be allowed", i)
	}

	// 11th request should be denied
	assert.False(t, rl.Allow("test-key"))

	// Different key should still work
	assert.True(t, rl.Allow("different-key"))
}

func TestRateLimiter_TokenRefill(t *testing.T) {
	rl := NewRateLimiter(100, 1) // 100 rps, burst of 1

	// Use the single token
	assert.True(t, rl.Allow("test-key"))
	assert.False(t, rl.Allow("test-key"))

	// Wait for token refill (10ms = 1 token at 100 rps)
	time.Sleep(15 * time.Millisecond)

	// Should have a token again
	assert.True(t, rl.Allow("test-key"))
}

func TestRateLimiter_Middleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rl := NewRateLimiter(10, 2) // Low limits for testing

	router := gin.New()
	router.Use(rl.Middleware())
	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})

	// First 2 requests should succeed (burst)
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, "request %d should succeed", i)
	}

	// 3rd request should be rate limited
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestRateLimiter_MiddlewareWithAuthHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rl := NewRateLimiter(10, 1)

	router := gin.New()
	router.Use(rl.Middleware())
	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})

	// First request with auth header
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer key1")
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Second request with same auth should be limited
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer key1")
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	// Request with different auth should succeed
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer key2")
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestDefaultRateLimitConfig(t *testing.T) {
	cfg := DefaultRateLimitConfig()
	assert.Equal(t, 100, cfg.RequestsPerSecond)
	assert.Equal(t, 100, cfg.BurstSize)
}

func TestNewRateLimiter_DefaultValues(t *testing.T) {
	// Test with zero values
	rl := NewRateLimiter(0, 0)
	assert.Equal(t, 100, rl.rate)
	assert.Equal(t, 100, rl.burst)

	// Test with negative values
	rl = NewRateLimiter(-1, -1)
	assert.Equal(t, 100, rl.rate)
	assert.Equal(t, 100, rl.burst)
}
