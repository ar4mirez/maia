// Package server provides the HTTP and gRPC server implementations for MAIA.
package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.uber.org/zap"

	"github.com/ar4mirez/maia/internal/config"
	mcontext "github.com/ar4mirez/maia/internal/context"
	"github.com/ar4mirez/maia/internal/inference"
	"github.com/ar4mirez/maia/internal/inference/providers/anthropic"
	"github.com/ar4mirez/maia/internal/inference/providers/ollama"
	"github.com/ar4mirez/maia/internal/inference/providers/openrouter"
	"github.com/ar4mirez/maia/internal/metrics"
	"github.com/ar4mirez/maia/internal/query"
	"github.com/ar4mirez/maia/internal/retrieval"
	"github.com/ar4mirez/maia/internal/storage"
	"github.com/ar4mirez/maia/internal/tenant"
	"github.com/ar4mirez/maia/pkg/proxy"
)

// Server represents the MAIA HTTP server.
type Server struct {
	cfg             *config.Config
	store           storage.Store
	tenantStore     *tenant.TenantAwareStore
	tenants         tenant.Manager
	logger          *zap.Logger
	router          *gin.Engine
	server          *http.Server
	analyzer        *query.Analyzer
	retriever       *retrieval.Retriever
	assembler       *mcontext.Assembler
	metrics         *metrics.Metrics
	inferenceRouter inference.InferenceRouter
	inferenceCache  *inference.Cache
	proxy           *proxy.Proxy
}

// ServerDeps holds optional dependencies for the server.
type ServerDeps struct {
	Retriever       *retrieval.Retriever
	Assembler       *mcontext.Assembler
	Analyzer        *query.Analyzer
	TenantManager   tenant.Manager
	InferenceRouter inference.InferenceRouter
}

// New creates a new HTTP server.
func New(cfg *config.Config, store storage.Store, logger *zap.Logger) *Server {
	return NewWithDeps(cfg, store, logger, nil)
}

// NewWithDeps creates a new HTTP server with optional dependencies.
func NewWithDeps(cfg *config.Config, store storage.Store, logger *zap.Logger, deps *ServerDeps) *Server {
	// Set Gin mode based on log level
	if cfg.Log.Level == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	s := &Server{
		cfg:     cfg,
		store:   store,
		logger:  logger,
		router:  router,
		metrics: metrics.Default(),
	}

	// Set up dependencies if provided
	if deps != nil {
		s.retriever = deps.Retriever
		s.assembler = deps.Assembler
		s.analyzer = deps.Analyzer
		s.tenants = deps.TenantManager
		s.inferenceRouter = deps.InferenceRouter
	}

	// Create TenantAwareStore if tenant manager is available
	if s.tenants != nil {
		s.tenantStore = tenant.NewTenantAwareStore(store, s.tenants)
	}

	// Create default analyzer if not provided
	if s.analyzer == nil {
		s.analyzer = query.NewAnalyzer()
	}

	// Create default assembler if not provided
	if s.assembler == nil {
		s.assembler = mcontext.NewAssembler(mcontext.DefaultAssemblerConfig())
	}

	// Initialize inference router if enabled and not provided
	if cfg.Inference.Enabled && s.inferenceRouter == nil {
		s.inferenceRouter = s.initInferenceRouter()
	}

	// Initialize proxy if backend is configured or inference is enabled
	if cfg.Proxy.Backend != "" || cfg.Inference.Enabled {
		s.initProxy()
	}

	s.setupMiddleware()
	s.setupRoutes()

	return s
}

// initInferenceRouter initializes the inference router based on configuration.
func (s *Server) initInferenceRouter() inference.InferenceRouter {
	routingCfg := inference.RoutingConfig{
		ModelMapping: s.cfg.Inference.Routing.ModelMapping,
	}
	router := inference.NewRouter(routingCfg, s.cfg.Inference.DefaultProvider)

	// Register configured providers
	for name, provCfg := range s.cfg.Inference.Providers {
		infCfg := inference.ProviderConfig{
			Type:       provCfg.Type,
			BaseURL:    provCfg.BaseURL,
			APIKey:     provCfg.APIKey,
			Models:     provCfg.Models,
			Timeout:    provCfg.Timeout,
			MaxRetries: provCfg.MaxRetries,
		}

		var provider inference.Provider
		var err error

		switch provCfg.Type {
		case "ollama":
			provider, err = ollama.NewProvider(name, infCfg)
		case "openrouter":
			provider, err = openrouter.NewProvider(name, infCfg)
		case "anthropic":
			provider, err = anthropic.NewProvider(name, infCfg)
		default:
			s.logger.Warn("unknown inference provider type",
				zap.String("name", name),
				zap.String("type", provCfg.Type),
			)
			continue
		}

		if err != nil {
			s.logger.Error("failed to create inference provider",
				zap.String("name", name),
				zap.Error(err),
			)
			continue
		}

		if err := router.RegisterProvider(provider); err != nil {
			s.logger.Error("failed to register inference provider",
				zap.String("name", name),
				zap.Error(err),
			)
			continue
		}

		s.logger.Info("registered inference provider",
			zap.String("name", name),
			zap.String("type", provCfg.Type),
		)
	}

	// Set up health checking if configured
	if s.cfg.Inference.Health.Enabled {
		healthCfg := inference.HealthConfig{
			Enabled:            true,
			Interval:           s.cfg.Inference.Health.Interval,
			Timeout:            s.cfg.Inference.Health.Timeout,
			UnhealthyThreshold: s.cfg.Inference.Health.UnhealthyThreshold,
			HealthyThreshold:   s.cfg.Inference.Health.HealthyThreshold,
		}
		if healthCfg.Interval == 0 {
			healthCfg.Interval = inference.DefaultHealthConfig().Interval
		}
		if healthCfg.Timeout == 0 {
			healthCfg.Timeout = inference.DefaultHealthConfig().Timeout
		}
		if healthCfg.UnhealthyThreshold == 0 {
			healthCfg.UnhealthyThreshold = inference.DefaultHealthConfig().UnhealthyThreshold
		}
		if healthCfg.HealthyThreshold == 0 {
			healthCfg.HealthyThreshold = inference.DefaultHealthConfig().HealthyThreshold
		}

		healthChecker := inference.NewHealthChecker(healthCfg)
		router.SetHealthChecker(healthChecker)
		healthChecker.Start()

		s.logger.Info("inference health checking enabled",
			zap.Duration("interval", healthCfg.Interval),
			zap.Int("unhealthy_threshold", healthCfg.UnhealthyThreshold),
		)
	}

	// Wrap with caching router if cache is enabled
	if s.cfg.Inference.Cache.Enabled {
		cacheCfg := inference.CacheConfig{
			Enabled:    true,
			TTL:        s.cfg.Inference.Cache.TTL,
			Namespace:  s.cfg.Inference.Cache.Namespace,
			MaxEntries: s.cfg.Inference.Cache.MaxEntries,
		}
		if cacheCfg.TTL == 0 {
			cacheCfg.TTL = 24 * time.Hour
		}
		if cacheCfg.MaxEntries == 0 {
			cacheCfg.MaxEntries = 1000
		}

		cache := inference.NewCache(cacheCfg)
		s.inferenceCache = cache

		s.logger.Info("inference caching enabled",
			zap.Duration("ttl", cacheCfg.TTL),
			zap.Int("max_entries", cacheCfg.MaxEntries),
		)

		return inference.NewCachingRouter(router, cache)
	}

	return router
}

// initProxy initializes the OpenAI-compatible proxy.
func (s *Server) initProxy() {
	proxyCfg := &proxy.ProxyConfig{
		Backend:          s.cfg.Proxy.Backend,
		AutoRemember:     s.cfg.Proxy.AutoRemember,
		AutoContext:      s.cfg.Proxy.AutoContext,
		ContextPosition:  proxy.ContextPosition(s.cfg.Proxy.ContextPosition),
		TokenBudget:      s.cfg.Proxy.TokenBudget,
		DefaultNamespace: s.cfg.Memory.DefaultNamespace,
		Timeout:          s.cfg.Server.RequestTimeout,
	}

	proxyDeps := &proxy.ProxyDeps{
		Store:           s.store,
		Retriever:       s.retriever,
		Assembler:       s.assembler,
		Logger:          s.logger,
		InferenceRouter: s.inferenceRouter,
	}

	s.proxy = proxy.NewProxy(proxyCfg, proxyDeps)
}

// setupMiddleware configures middleware for the router.
func (s *Server) setupMiddleware() {
	// Recovery middleware
	s.router.Use(gin.Recovery())

	// OpenTelemetry tracing middleware (if enabled)
	if s.cfg.Tracing.Enabled {
		s.router.Use(otelgin.Middleware(s.cfg.Tracing.ServiceName))
	}

	// Security headers middleware
	s.router.Use(s.securityHeadersMiddleware())

	// Request ID middleware
	s.router.Use(s.requestIDMiddleware())

	// Logging middleware
	s.router.Use(s.loggingMiddleware())

	// CORS middleware
	s.router.Use(s.corsMiddleware())

	// Rate limiting middleware
	s.router.Use(s.rateLimitMiddleware(RateLimitConfig{
		Enabled:           s.cfg.Security.RateLimitRPS > 0,
		RequestsPerSecond: s.cfg.Security.RateLimitRPS,
		BurstSize:         s.cfg.Security.RateLimitRPS * 2,
	}))

	// Authentication middleware
	s.router.Use(s.authMiddleware(AuthConfig{
		Enabled: s.cfg.Security.APIKey != "",
		APIKeys: []string{s.cfg.Security.APIKey},
		SkipPaths: []string{
			"/health",
			"/ready",
			"/metrics",
		},
	}))

	// Authorization middleware (namespace-level access control)
	s.router.Use(s.authzMiddleware(AuthzConfig{
		Enabled:           s.cfg.Security.Authorization.Enabled,
		DefaultPolicy:     s.cfg.Security.Authorization.DefaultPolicy,
		APIKeyPermissions: s.cfg.Security.Authorization.APIKeyPermissions,
	}))

	// Request timeout middleware
	s.router.Use(s.timeoutMiddleware())
}

// loggingMiddleware logs requests and records metrics.
func (s *Server) loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		// Track in-flight requests
		if s.metrics != nil {
			s.metrics.HTTPRequestsInFlight.Inc()
			defer s.metrics.HTTPRequestsInFlight.Dec()
		}

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		s.logger.Info("request",
			zap.String("method", method),
			zap.String("path", path),
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.String("client_ip", c.ClientIP()),
		)

		// Record metrics
		if s.metrics != nil {
			s.metrics.RecordHTTPRequest(method, path, status, latency.Seconds())
		}
	}
}

// corsMiddleware handles CORS.
func (s *Server) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Check if origin is allowed
		allowed := false
		for _, o := range s.cfg.Server.CORSOrigins {
			if o == "*" || o == origin {
				allowed = true
				break
			}
		}

		if allowed {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
			c.Header("Access-Control-Max-Age", "86400")
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// timeoutMiddleware adds request timeout.
func (s *Server) timeoutMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), s.cfg.Server.RequestTimeout)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// setupRoutes configures API routes.
func (s *Server) setupRoutes() {
	// Health check
	s.router.GET("/health", s.healthHandler)
	s.router.GET("/ready", s.readyHandler)

	// Metrics endpoint (Prometheus)
	s.router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// API v1
	v1 := s.router.Group("/v1")
	{
		// Memory routes
		memories := v1.Group("/memories")
		{
			memories.POST("", s.createMemory)
			memories.GET("/:id", s.getMemory)
			memories.PUT("/:id", s.updateMemory)
			memories.DELETE("/:id", s.deleteMemory)
			memories.POST("/search", s.searchMemories)
		}

		// Namespace routes
		namespaces := v1.Group("/namespaces")
		{
			namespaces.POST("", s.createNamespace)
			namespaces.GET("", s.listNamespaces)
			namespaces.GET("/:id", s.getNamespace)
			namespaces.PUT("/:id", s.updateNamespace)
			namespaces.DELETE("/:id", s.deleteNamespace)
			namespaces.GET("/:id/memories", s.listNamespaceMemories)
		}

		// Context routes (for future use)
		v1.POST("/context", s.getContext)

		// Stats
		v1.GET("/stats", s.getStats)

		// Inference health (available even if inference is disabled)
		inferenceGroup := v1.Group("/inference")
		{
			inferenceGroup.GET("/health", s.getInferenceHealth)
			inferenceGroup.POST("/health/:name", s.checkInferenceProviderHealth)

			// Cache endpoints
			cache := inferenceGroup.Group("/cache")
			{
				cache.GET("/stats", s.getInferenceCacheStats)
				cache.POST("/clear", s.clearInferenceCache)
			}
		}
	}

	// Admin API (requires tenant manager)
	if s.tenants != nil {
		admin := s.router.Group("/admin")
		{
			tenants := admin.Group("/tenants")
			{
				tenants.POST("", s.createTenant)
				tenants.GET("", s.listTenants)
				tenants.GET("/:id", s.getTenant)
				tenants.PUT("/:id", s.updateTenant)
				tenants.DELETE("/:id", s.deleteTenant)
				tenants.GET("/:id/usage", s.getTenantUsage)
				tenants.POST("/:id/suspend", s.suspendTenant)
				tenants.POST("/:id/activate", s.activateTenant)
			}
		}
	}

	// OpenAI-compatible proxy routes (if proxy is initialized)
	if s.proxy != nil {
		s.proxy.RegisterRoutes(s.router)
		s.logger.Info("proxy routes registered",
			zap.Bool("inference_enabled", s.inferenceRouter != nil),
		)
	}
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.cfg.Server.HTTPPort)

	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.logger.Info("starting HTTP server", zap.String("addr", addr))

	if s.cfg.Security.EnableTLS {
		return s.server.ListenAndServeTLS(
			s.cfg.Security.TLSCertPath,
			s.cfg.Security.TLSKeyPath,
		)
	}

	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down HTTP server")

	// Close inference router if initialized
	if s.inferenceRouter != nil {
		if err := s.inferenceRouter.Close(); err != nil {
			s.logger.Error("failed to close inference router", zap.Error(err))
		}
	}

	return s.server.Shutdown(ctx)
}

// Router returns the Gin router (for testing).
func (s *Server) Router() *gin.Engine {
	return s.router
}
