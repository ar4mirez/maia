package proxy

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimiter implements a token bucket rate limiter.
type RateLimiter struct {
	mu           sync.Mutex
	buckets      map[string]*bucket
	rate         int           // requests per second
	burst        int           // maximum burst size
	cleanupEvery time.Duration // cleanup interval for old buckets
}

// bucket represents a token bucket for a single client.
type bucket struct {
	tokens     float64
	lastUpdate time.Time
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(rps int, burst int) *RateLimiter {
	if rps <= 0 {
		rps = 100
	}
	if burst <= 0 {
		burst = rps
	}

	rl := &RateLimiter{
		buckets:      make(map[string]*bucket),
		rate:         rps,
		burst:        burst,
		cleanupEvery: 10 * time.Minute,
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}

// Allow checks if a request is allowed for the given key.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	b, exists := rl.buckets[key]
	if !exists {
		b = &bucket{
			tokens:     float64(rl.burst),
			lastUpdate: now,
		}
		rl.buckets[key] = b
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(b.lastUpdate).Seconds()
	b.tokens += elapsed * float64(rl.rate)
	if b.tokens > float64(rl.burst) {
		b.tokens = float64(rl.burst)
	}
	b.lastUpdate = now

	// Check if we have tokens
	if b.tokens >= 1 {
		b.tokens--
		return true
	}

	return false
}

// Middleware returns a Gin middleware for rate limiting.
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Use client IP as rate limit key
		key := c.ClientIP()

		// Check for API key header as alternate key
		if apiKey := c.GetHeader("Authorization"); apiKey != "" {
			key = apiKey
		}

		if !rl.Allow(key) {
			c.JSON(http.StatusTooManyRequests, ErrorResponse{
				Error: &APIError{
					Type:    "rate_limit_exceeded",
					Message: "Rate limit exceeded. Please try again later.",
				},
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// cleanup periodically removes old buckets.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.cleanupEvery)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for key, b := range rl.buckets {
			// Remove buckets that haven't been used in 10 minutes
			if now.Sub(b.lastUpdate) > 10*time.Minute {
				delete(rl.buckets, key)
			}
		}
		rl.mu.Unlock()
	}
}

// RateLimitConfig holds rate limiter configuration.
type RateLimitConfig struct {
	RequestsPerSecond int
	BurstSize         int
}

// DefaultRateLimitConfig returns default rate limit configuration.
func DefaultRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		RequestsPerSecond: 100,
		BurstSize:         100,
	}
}
