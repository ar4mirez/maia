package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ar4mirez/maia/internal/config"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupTestServerWithAuth(t *testing.T, apiKey string) *Server {
	t.Helper()
	cfg := &config.Config{
		Server: config.ServerConfig{
			HTTPPort:       8080,
			GRPCPort:       9090,
			RequestTimeout: 30 * time.Second,
			CORSOrigins:    []string{"*"},
		},
		Log: config.LogConfig{
			Level: "info",
		},
		Security: config.SecurityConfig{
			APIKey:       apiKey,
			RateLimitRPS: 0, // Disable rate limiting for most tests
		},
	}

	logger, _ := zap.NewDevelopment()
	return New(cfg, nil, logger)
}

func TestAuthMiddleware_NoAPIKeyRequired(t *testing.T) {
	srv := setupTestServerWithAuth(t, "")

	// Using a protected endpoint (v1/memories/test-id) - will get 500 but not 401
	req := httptest.NewRequest(http.MethodGet, "/v1/memories/test-id", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	// Should pass through (even if handler fails due to nil store, no 401)
	assert.NotEqual(t, http.StatusUnauthorized, w.Code)
}

func TestAuthMiddleware_ValidAPIKey_Header(t *testing.T) {
	srv := setupTestServerWithAuth(t, "test-api-key")

	// Test with protected endpoint
	req := httptest.NewRequest(http.MethodGet, "/v1/memories/test-id", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	// Should not be 401 (will likely be 500 due to nil store, but auth passed)
	assert.NotEqual(t, http.StatusUnauthorized, w.Code)
}

func TestAuthMiddleware_ValidAPIKey_Bearer(t *testing.T) {
	srv := setupTestServerWithAuth(t, "test-api-key")

	req := httptest.NewRequest(http.MethodGet, "/v1/memories/test-id", nil)
	req.Header.Set("Authorization", "Bearer test-api-key")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	assert.NotEqual(t, http.StatusUnauthorized, w.Code)
}

func TestAuthMiddleware_ValidAPIKey_Query(t *testing.T) {
	srv := setupTestServerWithAuth(t, "test-api-key")

	req := httptest.NewRequest(http.MethodGet, "/v1/memories/test-id?api_key=test-api-key", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	assert.NotEqual(t, http.StatusUnauthorized, w.Code)
}

func TestAuthMiddleware_MissingAPIKey(t *testing.T) {
	srv := setupTestServerWithAuth(t, "test-api-key")

	req := httptest.NewRequest(http.MethodGet, "/v1/memories/test-id", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "API key is required")
}

func TestAuthMiddleware_InvalidAPIKey(t *testing.T) {
	srv := setupTestServerWithAuth(t, "test-api-key")

	req := httptest.NewRequest(http.MethodGet, "/v1/memories/test-id", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "invalid API key")
}

func TestAuthMiddleware_SkipHealthEndpoint(t *testing.T) {
	srv := setupTestServerWithAuth(t, "test-api-key")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthMiddleware_SkipReadyEndpoint(t *testing.T) {
	srv := setupTestServerWithAuth(t, "test-api-key")

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	// Auth should be skipped - we won't get 401 even without API key
	// Note: We get 500 because store is nil, but the important thing
	// is that we don't get 401 (auth was skipped)
	assert.NotEqual(t, http.StatusUnauthorized, w.Code)
}

func TestRateLimiter_Allow(t *testing.T) {
	limiter := newRateLimiter(10, 10)
	defer limiter.stop()

	// First 10 requests should be allowed (burst)
	for i := 0; i < 10; i++ {
		assert.True(t, limiter.allow("client1"), "request %d should be allowed", i)
	}

	// 11th request should be denied
	assert.False(t, limiter.allow("client1"), "11th request should be denied")
}

func TestRateLimiter_RefillTokens(t *testing.T) {
	limiter := newRateLimiter(100, 10)
	defer limiter.stop()

	// Exhaust all tokens
	for i := 0; i < 10; i++ {
		limiter.allow("client1")
	}
	assert.False(t, limiter.allow("client1"))

	// Wait for token refill (at 100 RPS, should get 1 token per 10ms)
	time.Sleep(50 * time.Millisecond)

	// Should have at least 1 token now
	assert.True(t, limiter.allow("client1"))
}

func TestRateLimiter_MultipleClients(t *testing.T) {
	limiter := newRateLimiter(5, 5)
	defer limiter.stop()

	// Client1 exhausts tokens
	for i := 0; i < 5; i++ {
		limiter.allow("client1")
	}
	assert.False(t, limiter.allow("client1"))

	// Client2 should still have tokens
	assert.True(t, limiter.allow("client2"))
}

func TestRateLimitMiddleware_Disabled(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := &config.Config{
		Server: config.ServerConfig{
			HTTPPort:       8080,
			GRPCPort:       9090,
			RequestTimeout: 30 * time.Second,
			CORSOrigins:    []string{"*"},
		},
		Log: config.LogConfig{
			Level: "info",
		},
		Security: config.SecurityConfig{
			RateLimitRPS: 0, // Disabled
		},
	}
	srv := New(cfg, nil, logger)

	// Make many requests - should all pass
	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		srv.router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}
}

func TestRateLimitMiddleware_Enabled(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := &config.Config{
		Server: config.ServerConfig{
			HTTPPort:       8080,
			GRPCPort:       9090,
			RequestTimeout: 30 * time.Second,
			CORSOrigins:    []string{"*"},
		},
		Log: config.LogConfig{
			Level: "info",
		},
		Security: config.SecurityConfig{
			RateLimitRPS: 5, // 5 requests per second, burst of 10
		},
	}
	srv := New(cfg, nil, logger)

	// Exhaust rate limit
	rateLimited := false
	for i := 0; i < 20; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		srv.router.ServeHTTP(w, req)
		if w.Code == http.StatusTooManyRequests {
			rateLimited = true
			break
		}
	}

	assert.True(t, rateLimited, "should have hit rate limit")
}

func TestRequestIDMiddleware(t *testing.T) {
	srv := setupTestServerWithAuth(t, "")

	// Test generated request ID
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	requestID := w.Header().Get("X-Request-ID")
	assert.NotEmpty(t, requestID)
	assert.Len(t, requestID, 23) // 14 (timestamp) + 1 (-) + 8 (random)
}

func TestRequestIDMiddleware_Passthrough(t *testing.T) {
	srv := setupTestServerWithAuth(t, "")

	// Test passthrough of existing request ID
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Request-ID", "existing-id-12345")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	requestID := w.Header().Get("X-Request-ID")
	assert.Equal(t, "existing-id-12345", requestID)
}

func TestSecurityHeadersMiddleware(t *testing.T) {
	srv := setupTestServerWithAuth(t, "")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
	assert.Equal(t, "1; mode=block", w.Header().Get("X-XSS-Protection"))
	assert.Equal(t, "strict-origin-when-cross-origin", w.Header().Get("Referrer-Policy"))
}

func TestGenerateRequestID(t *testing.T) {
	id1 := generateRequestID()
	time.Sleep(time.Millisecond)
	id2 := generateRequestID()

	require.NotEmpty(t, id1)
	require.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2)
}

func TestRandomString(t *testing.T) {
	s1 := randomString(8)
	s2 := randomString(8)

	assert.Len(t, s1, 8)
	assert.Len(t, s2, 8)
	// With crypto/rand, these should always be different
	assert.NotEqual(t, s1, s2, "random strings should be different")
}

func TestMetricsEndpoint(t *testing.T) {
	srv := setupTestServerWithAuth(t, "")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// Should contain prometheus metrics
	assert.Contains(t, w.Body.String(), "go_gc")
}

func TestMetricsEndpoint_SkipsAuth(t *testing.T) {
	srv := setupTestServerWithAuth(t, "test-api-key")

	// Request without API key should still work for /metrics
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func setupTestServerWithAuthz(t *testing.T, apiKey string, authzConfig config.AuthorizationConfig) *Server {
	t.Helper()
	cfg := &config.Config{
		Server: config.ServerConfig{
			HTTPPort:       8080,
			GRPCPort:       9090,
			RequestTimeout: 30 * time.Second,
			CORSOrigins:    []string{"*"},
		},
		Log: config.LogConfig{
			Level: "info",
		},
		Security: config.SecurityConfig{
			APIKey:        apiKey,
			RateLimitRPS:  0,
			Authorization: authzConfig,
		},
	}

	logger, _ := zap.NewDevelopment()
	return New(cfg, nil, logger)
}

func TestAuthzMiddleware_Disabled(t *testing.T) {
	srv := setupTestServerWithAuthz(t, "test-key", config.AuthorizationConfig{
		Enabled: false,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/namespaces/test-ns", nil)
	req.Header.Set("X-API-Key", "test-key")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	// Should not be forbidden (will be 500 due to nil store, but not 403)
	assert.NotEqual(t, http.StatusForbidden, w.Code)
}

func TestAuthzMiddleware_AllAccessKey(t *testing.T) {
	srv := setupTestServerWithAuthz(t, "admin-key", config.AuthorizationConfig{
		Enabled:       true,
		DefaultPolicy: "deny",
		APIKeyPermissions: map[string][]string{
			"admin-key": {"*"}, // All access
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/namespaces/any-namespace", nil)
	req.Header.Set("X-API-Key", "admin-key")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	// Should not be forbidden
	assert.NotEqual(t, http.StatusForbidden, w.Code)
}

func TestAuthzMiddleware_AllowedNamespace(t *testing.T) {
	srv := setupTestServerWithAuthz(t, "limited-key", config.AuthorizationConfig{
		Enabled:       true,
		DefaultPolicy: "deny",
		APIKeyPermissions: map[string][]string{
			"limited-key": {"allowed-ns"},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/namespaces/allowed-ns", nil)
	req.Header.Set("X-API-Key", "limited-key")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	// Should not be forbidden
	assert.NotEqual(t, http.StatusForbidden, w.Code)
}

func TestAuthzMiddleware_DeniedNamespace(t *testing.T) {
	srv := setupTestServerWithAuthz(t, "limited-key", config.AuthorizationConfig{
		Enabled:       true,
		DefaultPolicy: "deny",
		APIKeyPermissions: map[string][]string{
			"limited-key": {"allowed-ns"},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/namespaces/forbidden-ns", nil)
	req.Header.Set("X-API-Key", "limited-key")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "access denied")
}

func TestAuthzMiddleware_HierarchicalNamespace(t *testing.T) {
	srv := setupTestServerWithAuthz(t, "org-key", config.AuthorizationConfig{
		Enabled:       true,
		DefaultPolicy: "deny",
		APIKeyPermissions: map[string][]string{
			"org-key": {"org1"},
		},
	})

	// Access to parent namespace
	req := httptest.NewRequest(http.MethodGet, "/v1/namespaces/org1", nil)
	req.Header.Set("X-API-Key", "org-key")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	assert.NotEqual(t, http.StatusForbidden, w.Code)

	// Access to child namespace
	req = httptest.NewRequest(http.MethodGet, "/v1/namespaces/org1/project1", nil)
	req.Header.Set("X-API-Key", "org-key")
	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	assert.NotEqual(t, http.StatusForbidden, w.Code)

	// No access to different org
	req = httptest.NewRequest(http.MethodGet, "/v1/namespaces/org2", nil)
	req.Header.Set("X-API-Key", "org-key")
	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestAuthzMiddleware_DefaultPolicyAllow(t *testing.T) {
	srv := setupTestServerWithAuthz(t, "unknown-key", config.AuthorizationConfig{
		Enabled:           true,
		DefaultPolicy:     "allow",
		APIKeyPermissions: map[string][]string{},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/namespaces/any-ns", nil)
	req.Header.Set("X-API-Key", "unknown-key")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	// Should not be forbidden with allow default policy
	assert.NotEqual(t, http.StatusForbidden, w.Code)
}

func TestAuthzMiddleware_DefaultPolicyDeny(t *testing.T) {
	srv := setupTestServerWithAuthz(t, "unknown-key", config.AuthorizationConfig{
		Enabled:           true,
		DefaultPolicy:     "deny",
		APIKeyPermissions: map[string][]string{},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/namespaces/any-ns", nil)
	req.Header.Set("X-API-Key", "unknown-key")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	// Should be forbidden with deny default policy and no permissions
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestAuthzMiddleware_NamespaceFromHeader(t *testing.T) {
	srv := setupTestServerWithAuthz(t, "key", config.AuthorizationConfig{
		Enabled:       true,
		DefaultPolicy: "deny",
		APIKeyPermissions: map[string][]string{
			"key": {"header-ns"},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-API-Key", "key")
	req.Header.Set("X-MAIA-Namespace", "header-ns")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	// Should pass (health endpoint with allowed namespace)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthzMiddleware_NamespaceFromQuery(t *testing.T) {
	srv := setupTestServerWithAuthz(t, "key", config.AuthorizationConfig{
		Enabled:       true,
		DefaultPolicy: "deny",
		APIKeyPermissions: map[string][]string{
			"key": {"query-ns"},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/health?namespace=query-ns", nil)
	req.Header.Set("X-API-Key", "key")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthzMiddleware_MultipleNamespaces(t *testing.T) {
	srv := setupTestServerWithAuthz(t, "multi-key", config.AuthorizationConfig{
		Enabled:       true,
		DefaultPolicy: "deny",
		APIKeyPermissions: map[string][]string{
			"multi-key": {"ns1", "ns2", "ns3"},
		},
	})

	// Test access to each allowed namespace
	for _, ns := range []string{"ns1", "ns2", "ns3"} {
		req := httptest.NewRequest(http.MethodGet, "/v1/namespaces/"+ns, nil)
		req.Header.Set("X-API-Key", "multi-key")
		w := httptest.NewRecorder()
		srv.router.ServeHTTP(w, req)
		assert.NotEqual(t, http.StatusForbidden, w.Code, "should have access to %s", ns)
	}

	// Test denied namespace
	req := httptest.NewRequest(http.MethodGet, "/v1/namespaces/ns4", nil)
	req.Header.Set("X-API-Key", "multi-key")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestExtractNamespace(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name      string
		setup     func(c *gin.Context)
		expected  string
	}{
		{
			name:     "no namespace",
			setup:    func(c *gin.Context) {},
			expected: "",
		},
		{
			name: "from header",
			setup: func(c *gin.Context) {
				c.Request.Header.Set("X-MAIA-Namespace", "header-ns")
			},
			expected: "header-ns",
		},
		{
			name: "from query",
			setup: func(c *gin.Context) {
				c.Request.URL.RawQuery = "namespace=query-ns"
			},
			expected: "query-ns",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
			tt.setup(c)

			ns := extractNamespace(c)
			assert.Equal(t, tt.expected, ns)
		})
	}
}

func TestExtractAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name     string
		setup    func(c *gin.Context)
		expected string
	}{
		{
			name:     "no key",
			setup:    func(c *gin.Context) {},
			expected: "",
		},
		{
			name: "from X-API-Key header",
			setup: func(c *gin.Context) {
				c.Request.Header.Set("X-API-Key", "header-key")
			},
			expected: "header-key",
		},
		{
			name: "from Bearer token",
			setup: func(c *gin.Context) {
				c.Request.Header.Set("Authorization", "Bearer bearer-key")
			},
			expected: "bearer-key",
		},
		{
			name: "from query",
			setup: func(c *gin.Context) {
				c.Request.URL.RawQuery = "api_key=query-key"
			},
			expected: "query-key",
		},
		{
			name: "X-API-Key takes precedence",
			setup: func(c *gin.Context) {
				c.Request.Header.Set("X-API-Key", "header-key")
				c.Request.Header.Set("Authorization", "Bearer bearer-key")
			},
			expected: "header-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
			tt.setup(c)

			key := extractAPIKey(c)
			assert.Equal(t, tt.expected, key)
		})
	}
}

func TestReadCloser(t *testing.T) {
	data := []byte("test data")
	rc := &readCloser{data: data}

	// Read all data
	buf := make([]byte, 100)
	n, err := rc.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, len(data), n)
	assert.Equal(t, data, buf[:n])

	// Second read should return EOF
	n, err = rc.Read(buf)
	assert.Equal(t, 0, n)
	assert.ErrorIs(t, err, io.EOF)

	// Close should be a no-op
	assert.NoError(t, rc.Close())
}
