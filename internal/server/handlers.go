package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ar4mirez/maia/internal/storage"
)

// API request/response types

// CreateMemoryRequest represents the request to create a memory.
type CreateMemoryRequest struct {
	Namespace  string                 `json:"namespace" binding:"required"`
	Content    string                 `json:"content" binding:"required"`
	Type       storage.MemoryType     `json:"type"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Tags       []string               `json:"tags,omitempty"`
	Confidence float64                `json:"confidence"`
	Source     storage.MemorySource   `json:"source"`
}

// UpdateMemoryRequest represents the request to update a memory.
type UpdateMemoryRequest struct {
	Content    *string                `json:"content,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Tags       []string               `json:"tags,omitempty"`
	Confidence *float64               `json:"confidence,omitempty"`
}

// SearchMemoriesRequest represents the request to search memories.
type SearchMemoriesRequest struct {
	Query     string               `json:"query,omitempty"`
	Namespace string               `json:"namespace,omitempty"`
	Types     []storage.MemoryType `json:"types,omitempty"`
	Tags      []string             `json:"tags,omitempty"`
	Limit     int                  `json:"limit,omitempty"`
	Offset    int                  `json:"offset,omitempty"`
}

// CreateNamespaceRequest represents the request to create a namespace.
type CreateNamespaceRequest struct {
	Name     string                  `json:"name" binding:"required"`
	Parent   string                  `json:"parent,omitempty"`
	Template string                  `json:"template,omitempty"`
	Config   storage.NamespaceConfig `json:"config,omitempty"`
}

// UpdateNamespaceRequest represents the request to update a namespace.
type UpdateNamespaceRequest struct {
	Config storage.NamespaceConfig `json:"config"`
}

// GetContextRequest represents the request to get assembled context.
type GetContextRequest struct {
	Query       string `json:"query" binding:"required"`
	Namespace   string `json:"namespace,omitempty"`
	TokenBudget int    `json:"token_budget,omitempty"`
}

// ErrorResponse represents an API error response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}

// ListResponse represents a paginated list response.
type ListResponse struct {
	Data   interface{} `json:"data"`
	Count  int         `json:"count"`
	Offset int         `json:"offset"`
	Limit  int         `json:"limit"`
}

// Health handlers

func (s *Server) healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"service": "maia",
	})
}

func (s *Server) readyHandler(c *gin.Context) {
	// Check if storage is accessible
	_, err := s.store.Stats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not ready",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "ready",
	})
}

// Memory handlers

func (s *Server) createMemory(c *gin.Context) {
	var req CreateMemoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "invalid request body",
			Details: err.Error(),
		})
		return
	}

	input := &storage.CreateMemoryInput{
		Namespace:  req.Namespace,
		Content:    req.Content,
		Type:       req.Type,
		Metadata:   req.Metadata,
		Tags:       req.Tags,
		Confidence: req.Confidence,
		Source:     req.Source,
	}

	mem, err := s.store.CreateMemory(c.Request.Context(), input)
	if err != nil {
		s.handleStorageError(c, err)
		return
	}

	c.JSON(http.StatusCreated, mem)
}

func (s *Server) getMemory(c *gin.Context) {
	id := c.Param("id")

	mem, err := s.store.GetMemory(c.Request.Context(), id)
	if err != nil {
		s.handleStorageError(c, err)
		return
	}

	// Update access time
	_ = s.store.TouchMemory(c.Request.Context(), id)

	c.JSON(http.StatusOK, mem)
}

func (s *Server) updateMemory(c *gin.Context) {
	id := c.Param("id")

	var req UpdateMemoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "invalid request body",
			Details: err.Error(),
		})
		return
	}

	input := &storage.UpdateMemoryInput{
		Content:    req.Content,
		Metadata:   req.Metadata,
		Tags:       req.Tags,
		Confidence: req.Confidence,
	}

	mem, err := s.store.UpdateMemory(c.Request.Context(), id, input)
	if err != nil {
		s.handleStorageError(c, err)
		return
	}

	c.JSON(http.StatusOK, mem)
}

func (s *Server) deleteMemory(c *gin.Context) {
	id := c.Param("id")

	if err := s.store.DeleteMemory(c.Request.Context(), id); err != nil {
		s.handleStorageError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (s *Server) searchMemories(c *gin.Context) {
	var req SearchMemoriesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "invalid request body",
			Details: err.Error(),
		})
		return
	}

	// Set defaults
	if req.Limit <= 0 {
		req.Limit = 100
	}
	if req.Limit > 1000 {
		req.Limit = 1000
	}

	opts := &storage.SearchOptions{
		Namespace: req.Namespace,
		Types:     req.Types,
		Tags:      req.Tags,
		Limit:     req.Limit,
		Offset:    req.Offset,
	}

	results, err := s.store.SearchMemories(c.Request.Context(), opts)
	if err != nil {
		s.handleStorageError(c, err)
		return
	}

	c.JSON(http.StatusOK, ListResponse{
		Data:   results,
		Count:  len(results),
		Offset: req.Offset,
		Limit:  req.Limit,
	})
}

// Namespace handlers

func (s *Server) createNamespace(c *gin.Context) {
	var req CreateNamespaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "invalid request body",
			Details: err.Error(),
		})
		return
	}

	input := &storage.CreateNamespaceInput{
		Name:     req.Name,
		Parent:   req.Parent,
		Template: req.Template,
		Config:   req.Config,
	}

	ns, err := s.store.CreateNamespace(c.Request.Context(), input)
	if err != nil {
		s.handleStorageError(c, err)
		return
	}

	c.JSON(http.StatusCreated, ns)
}

func (s *Server) getNamespace(c *gin.Context) {
	id := c.Param("id")

	// Try to get by ID first, then by name
	ns, err := s.store.GetNamespace(c.Request.Context(), id)
	if err != nil {
		var notFound *storage.ErrNotFound
		if errors.As(err, &notFound) {
			// Try by name
			ns, err = s.store.GetNamespaceByName(c.Request.Context(), id)
			if err != nil {
				s.handleStorageError(c, err)
				return
			}
		} else {
			s.handleStorageError(c, err)
			return
		}
	}

	c.JSON(http.StatusOK, ns)
}

func (s *Server) updateNamespace(c *gin.Context) {
	id := c.Param("id")

	var req UpdateNamespaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "invalid request body",
			Details: err.Error(),
		})
		return
	}

	ns, err := s.store.UpdateNamespace(c.Request.Context(), id, &req.Config)
	if err != nil {
		s.handleStorageError(c, err)
		return
	}

	c.JSON(http.StatusOK, ns)
}

func (s *Server) deleteNamespace(c *gin.Context) {
	id := c.Param("id")

	if err := s.store.DeleteNamespace(c.Request.Context(), id); err != nil {
		s.handleStorageError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (s *Server) listNamespaces(c *gin.Context) {
	limit := parseIntQuery(c, "limit", 100)
	offset := parseIntQuery(c, "offset", 0)

	if limit > 1000 {
		limit = 1000
	}

	opts := &storage.ListOptions{
		Limit:  limit,
		Offset: offset,
	}

	namespaces, err := s.store.ListNamespaces(c.Request.Context(), opts)
	if err != nil {
		s.handleStorageError(c, err)
		return
	}

	c.JSON(http.StatusOK, ListResponse{
		Data:   namespaces,
		Count:  len(namespaces),
		Offset: offset,
		Limit:  limit,
	})
}

func (s *Server) listNamespaceMemories(c *gin.Context) {
	id := c.Param("id")
	limit := parseIntQuery(c, "limit", 100)
	offset := parseIntQuery(c, "offset", 0)

	if limit > 1000 {
		limit = 1000
	}

	// Get namespace first to verify it exists
	ns, err := s.store.GetNamespace(c.Request.Context(), id)
	if err != nil {
		var notFound *storage.ErrNotFound
		if errors.As(err, &notFound) {
			ns, err = s.store.GetNamespaceByName(c.Request.Context(), id)
			if err != nil {
				s.handleStorageError(c, err)
				return
			}
		} else {
			s.handleStorageError(c, err)
			return
		}
	}

	opts := &storage.ListOptions{
		Limit:  limit,
		Offset: offset,
	}

	memories, err := s.store.ListMemories(c.Request.Context(), ns.Name, opts)
	if err != nil {
		s.handleStorageError(c, err)
		return
	}

	c.JSON(http.StatusOK, ListResponse{
		Data:   memories,
		Count:  len(memories),
		Offset: offset,
		Limit:  limit,
	})
}

// Context handler (placeholder for now)

func (s *Server) getContext(c *gin.Context) {
	var req GetContextRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "invalid request body",
			Details: err.Error(),
		})
		return
	}

	// For now, just search memories and return them
	// This will be expanded in Phase 3 with proper context assembly
	namespace := req.Namespace
	if namespace == "" {
		namespace = s.cfg.Memory.DefaultNamespace
	}

	tokenBudget := req.TokenBudget
	if tokenBudget <= 0 {
		tokenBudget = s.cfg.Memory.DefaultTokenBudget
	}

	opts := &storage.SearchOptions{
		Namespace: namespace,
		Limit:     50,
	}

	results, err := s.store.SearchMemories(c.Request.Context(), opts)
	if err != nil {
		s.handleStorageError(c, err)
		return
	}

	// Extract memories from results
	memories := make([]*storage.Memory, len(results))
	for i, r := range results {
		memories[i] = r.Memory
	}

	c.JSON(http.StatusOK, gin.H{
		"memories":     memories,
		"token_count":  0, // TODO: implement token counting
		"token_budget": tokenBudget,
		"truncated":    false,
	})
}

// Stats handler

func (s *Server) getStats(c *gin.Context) {
	stats, err := s.store.Stats(c.Request.Context())
	if err != nil {
		s.handleStorageError(c, err)
		return
	}

	c.JSON(http.StatusOK, stats)
}

// Error handling

func (s *Server) handleStorageError(c *gin.Context, err error) {
	var notFound *storage.ErrNotFound
	var alreadyExists *storage.ErrAlreadyExists
	var invalidInput *storage.ErrInvalidInput

	switch {
	case errors.As(err, &notFound):
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "NOT_FOUND",
		})
	case errors.As(err, &alreadyExists):
		c.JSON(http.StatusConflict, ErrorResponse{
			Error: err.Error(),
			Code:  "ALREADY_EXISTS",
		})
	case errors.As(err, &invalidInput):
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_INPUT",
		})
	default:
		s.logger.Error("storage error",
			zap.Error(err),
			zap.String("path", c.Request.URL.Path),
		)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "internal server error",
			Code:  "INTERNAL_ERROR",
		})
	}
}

// Helper functions

func parseIntQuery(c *gin.Context, key string, defaultVal int) int {
	if val := c.Query(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}
