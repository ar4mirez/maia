// Package server provides the HTTP and gRPC server implementations for MAIA.
package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ar4mirez/maia/internal/config"
	"github.com/ar4mirez/maia/internal/storage"
)

// Server represents the MAIA HTTP server.
type Server struct {
	cfg    *config.Config
	store  storage.Store
	logger *zap.Logger
	router *gin.Engine
	server *http.Server
}

// New creates a new HTTP server.
func New(cfg *config.Config, store storage.Store, logger *zap.Logger) *Server {
	// Set Gin mode based on log level
	if cfg.Log.Level == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	s := &Server{
		cfg:    cfg,
		store:  store,
		logger: logger,
		router: router,
	}

	s.setupMiddleware()
	s.setupRoutes()

	return s
}

// setupMiddleware configures middleware for the router.
func (s *Server) setupMiddleware() {
	// Recovery middleware
	s.router.Use(gin.Recovery())

	// Logging middleware
	s.router.Use(s.loggingMiddleware())

	// CORS middleware
	s.router.Use(s.corsMiddleware())

	// Request timeout middleware
	s.router.Use(s.timeoutMiddleware())
}

// loggingMiddleware logs requests.
func (s *Server) loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		s.logger.Info("request",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.String("client_ip", c.ClientIP()),
		)
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
	return s.server.Shutdown(ctx)
}

// Router returns the Gin router (for testing).
func (s *Server) Router() *gin.Engine {
	return s.router
}
