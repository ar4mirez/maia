// Package server provides the HTTP and gRPC server implementations for MAIA.
package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AuthConfig holds authentication configuration.
type AuthConfig struct {
	// Enabled controls whether authentication is required.
	Enabled bool
	// APIKeys is a list of valid API keys.
	APIKeys []string
	// SkipPaths are paths that don't require authentication.
	SkipPaths []string
}

// authMiddleware creates an API key authentication middleware.
func (s *Server) authMiddleware(config AuthConfig) gin.HandlerFunc {
	// Build lookup set for API keys
	validKeys := make(map[string]struct{}, len(config.APIKeys))
	for _, key := range config.APIKeys {
		if key != "" {
			validKeys[key] = struct{}{}
		}
	}

	// Build lookup set for skip paths
	skipPaths := make(map[string]struct{}, len(config.SkipPaths))
	for _, path := range config.SkipPaths {
		skipPaths[path] = struct{}{}
	}

	return func(c *gin.Context) {
		// Skip authentication if disabled
		if !config.Enabled || len(validKeys) == 0 {
			c.Next()
			return
		}

		// Skip authentication for certain paths
		if _, skip := skipPaths[c.Request.URL.Path]; skip {
			c.Next()
			return
		}

		// Also skip paths with prefixes (like /health)
		for path := range skipPaths {
			if strings.HasPrefix(c.Request.URL.Path, path) {
				c.Next()
				return
			}
		}

		// Extract API key from header or query
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			apiKey = strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		}
		if apiKey == "" {
			apiKey = c.Query("api_key")
		}

		// Validate API key
		if apiKey == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "API key is required",
			})
			return
		}

		if _, valid := validKeys[apiKey]; !valid {
			s.logger.Warn("invalid API key attempt",
				zap.String("path", c.Request.URL.Path),
				zap.String("client_ip", c.ClientIP()),
			)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "invalid API key",
			})
			return
		}

		c.Next()
	}
}

// RateLimitConfig holds rate limiting configuration.
type RateLimitConfig struct {
	// Enabled controls whether rate limiting is active.
	Enabled bool
	// RequestsPerSecond is the maximum requests per second per client.
	RequestsPerSecond int
	// BurstSize is the maximum burst size.
	BurstSize int
}

// rateLimiter implements a token bucket rate limiter per client IP.
type rateLimiter struct {
	mu       sync.RWMutex
	clients  map[string]*clientBucket
	rps      int
	burst    int
	cleanupT *time.Ticker
	done     chan struct{}
}

type clientBucket struct {
	tokens    float64
	lastTime  time.Time
	mu        sync.Mutex
}

// newRateLimiter creates a new rate limiter.
func newRateLimiter(rps, burst int) *rateLimiter {
	rl := &rateLimiter{
		clients: make(map[string]*clientBucket),
		rps:     rps,
		burst:   burst,
		done:    make(chan struct{}),
	}

	// Start cleanup goroutine
	rl.cleanupT = time.NewTicker(time.Minute)
	go rl.cleanup()

	return rl
}

// allow checks if a request from the given client is allowed.
func (rl *rateLimiter) allow(clientID string) bool {
	rl.mu.RLock()
	bucket, exists := rl.clients[clientID]
	rl.mu.RUnlock()

	if !exists {
		rl.mu.Lock()
		// Double-check after acquiring write lock
		if bucket, exists = rl.clients[clientID]; !exists {
			bucket = &clientBucket{
				tokens:   float64(rl.burst),
				lastTime: time.Now(),
			}
			rl.clients[clientID] = bucket
		}
		rl.mu.Unlock()
	}

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(bucket.lastTime).Seconds()
	bucket.lastTime = now

	// Add tokens based on elapsed time
	bucket.tokens += elapsed * float64(rl.rps)
	if bucket.tokens > float64(rl.burst) {
		bucket.tokens = float64(rl.burst)
	}

	// Check if request is allowed
	if bucket.tokens >= 1 {
		bucket.tokens--
		return true
	}

	return false
}

// cleanup removes stale client entries.
func (rl *rateLimiter) cleanup() {
	for {
		select {
		case <-rl.cleanupT.C:
			rl.mu.Lock()
			cutoff := time.Now().Add(-5 * time.Minute)
			for id, bucket := range rl.clients {
				bucket.mu.Lock()
				if bucket.lastTime.Before(cutoff) {
					delete(rl.clients, id)
				}
				bucket.mu.Unlock()
			}
			rl.mu.Unlock()
		case <-rl.done:
			return
		}
	}
}

// stop stops the rate limiter cleanup goroutine.
func (rl *rateLimiter) stop() {
	rl.cleanupT.Stop()
	close(rl.done)
}

// rateLimitMiddleware creates a rate limiting middleware.
func (s *Server) rateLimitMiddleware(config RateLimitConfig) gin.HandlerFunc {
	if !config.Enabled || config.RequestsPerSecond <= 0 {
		return func(c *gin.Context) {
			c.Next()
		}
	}

	burst := config.BurstSize
	if burst <= 0 {
		burst = config.RequestsPerSecond * 2
	}

	limiter := newRateLimiter(config.RequestsPerSecond, burst)

	return func(c *gin.Context) {
		clientID := c.ClientIP()

		if !limiter.allow(clientID) {
			s.logger.Warn("rate limit exceeded",
				zap.String("client_ip", clientID),
				zap.String("path", c.Request.URL.Path),
			)
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "rate_limit_exceeded",
				"message": "too many requests, please slow down",
			})
			return
		}

		c.Next()
	}
}

// requestIDMiddleware adds a unique request ID to each request.
func (s *Server) requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}
		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

// generateRequestID generates a unique request ID.
func generateRequestID() string {
	// Simple timestamp-based ID with random suffix
	return time.Now().Format("20060102150405") + "-" + randomString(8)
}

// randomString generates a random string of the given length.
func randomString(n int) string {
	b := make([]byte, (n+1)/2)
	_, err := rand.Read(b)
	if err != nil {
		// Fallback to timestamp-based if crypto/rand fails
		now := time.Now().UnixNano()
		for i := range b {
			b[i] = byte(now >> (i * 8))
		}
	}
	return hex.EncodeToString(b)[:n]
}

// securityHeadersMiddleware adds security headers to responses.
func (s *Server) securityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Next()
	}
}

// AuthzConfig holds authorization configuration.
type AuthzConfig struct {
	// Enabled controls whether authorization is active.
	Enabled bool
	// DefaultPolicy is the default access policy: "allow" or "deny".
	DefaultPolicy string
	// APIKeyPermissions maps API keys to their allowed namespaces.
	APIKeyPermissions map[string][]string
}

// authzMiddleware creates a namespace-level authorization middleware.
// It checks if the authenticated API key has permission to access the namespace.
func (s *Server) authzMiddleware(config AuthzConfig) gin.HandlerFunc {
	// Pre-compute permission lookups
	type permissions struct {
		namespaces map[string]struct{}
		allAccess  bool
	}
	keyPerms := make(map[string]permissions)

	for key, namespaces := range config.APIKeyPermissions {
		perms := permissions{
			namespaces: make(map[string]struct{}),
		}
		for _, ns := range namespaces {
			if ns == "*" {
				perms.allAccess = true
				break
			}
			perms.namespaces[ns] = struct{}{}
		}
		keyPerms[key] = perms
	}

	return func(c *gin.Context) {
		// Skip if authorization is disabled
		if !config.Enabled {
			c.Next()
			return
		}

		// Extract namespace from various sources
		namespace := extractNamespace(c)
		if namespace == "" {
			// No namespace in request, allow (namespace-specific endpoints will validate later)
			c.Next()
			return
		}

		// Get API key from context (set by auth middleware)
		apiKey := extractAPIKey(c)
		if apiKey == "" {
			// No API key means it passed auth (disabled or skipped path)
			// Apply default policy
			if config.DefaultPolicy == "deny" {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error":   "forbidden",
					"message": "namespace access denied",
				})
				return
			}
			c.Next()
			return
		}

		// Check permissions for this API key
		perms, hasPerms := keyPerms[apiKey]
		if !hasPerms {
			// API key not in permissions map, apply default policy
			if config.DefaultPolicy == "deny" {
				s.logger.Warn("namespace access denied",
					zap.String("namespace", namespace),
					zap.String("client_ip", c.ClientIP()),
				)
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error":   "forbidden",
					"message": "namespace access denied",
				})
				return
			}
			c.Next()
			return
		}

		// Check if API key has access to this namespace
		if perms.allAccess {
			c.Next()
			return
		}

		if _, allowed := perms.namespaces[namespace]; allowed {
			c.Next()
			return
		}

		// Check for hierarchical namespace access (e.g., "org1" grants access to "org1/project1")
		for allowedNs := range perms.namespaces {
			if strings.HasPrefix(namespace, allowedNs+"/") {
				c.Next()
				return
			}
		}

		s.logger.Warn("namespace access denied",
			zap.String("namespace", namespace),
			zap.String("client_ip", c.ClientIP()),
		)
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error":   "forbidden",
			"message": "access denied for namespace: " + namespace,
		})
	}
}

// extractNamespace extracts the namespace from the request.
func extractNamespace(c *gin.Context) string {
	// Try URL parameter (for /namespaces/:id routes)
	if ns := c.Param("id"); ns != "" {
		return ns
	}

	// Try X-MAIA-Namespace header
	if ns := c.GetHeader("X-MAIA-Namespace"); ns != "" {
		return ns
	}

	// Try query parameter
	if ns := c.Query("namespace"); ns != "" {
		return ns
	}

	// For POST/PUT requests, try to extract from JSON body
	// We peek at the body and then restore it
	if c.Request.Method == http.MethodPost || c.Request.Method == http.MethodPut {
		if c.Request.Body != nil {
			// Read body
			bodyBytes, err := c.GetRawData()
			if err == nil && len(bodyBytes) > 0 {
				// Restore body for downstream handlers
				c.Request.Body = &readCloser{data: bodyBytes}

				// Try to extract namespace from JSON
				var body map[string]interface{}
				if err := json.Unmarshal(bodyBytes, &body); err == nil {
					if ns, ok := body["namespace"].(string); ok {
						return ns
					}
				}
			}
		}
	}

	return ""
}

// readCloser is a helper to restore request body after reading.
type readCloser struct {
	data   []byte
	offset int
}

func (r *readCloser) Read(p []byte) (n int, err error) {
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}

func (r *readCloser) Close() error {
	return nil
}

// extractAPIKey extracts the API key from the request.
func extractAPIKey(c *gin.Context) string {
	apiKey := c.GetHeader("X-API-Key")
	if apiKey == "" {
		apiKey = strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	}
	if apiKey == "" {
		apiKey = c.Query("api_key")
	}
	return apiKey
}
