package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	mcontext "github.com/ar4mirez/maia/internal/context"
	"github.com/ar4mirez/maia/internal/retrieval"
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

// GetContextRequest extended options
type GetContextRequestExtended struct {
	Query         string  `json:"query" binding:"required"`
	Namespace     string  `json:"namespace,omitempty"`
	TokenBudget   int     `json:"token_budget,omitempty"`
	SystemPrompt  string  `json:"system_prompt,omitempty"`
	IncludeScores bool    `json:"include_scores,omitempty"`
	MinScore      float64 `json:"min_score,omitempty"`
}

// ContextResponse represents the assembled context response.
type ContextResponse struct {
	Content     string                    `json:"content"`
	Memories    []*ContextMemoryResponse  `json:"memories"`
	TokenCount  int                       `json:"token_count"`
	TokenBudget int                       `json:"token_budget"`
	Truncated   bool                      `json:"truncated"`
	ZoneStats   *ContextZoneStatsResponse `json:"zone_stats,omitempty"`
	QueryTime   string                    `json:"query_time"`
}

// ContextMemoryResponse represents a memory in the context response.
type ContextMemoryResponse struct {
	ID         string  `json:"id"`
	Content    string  `json:"content"`
	Type       string  `json:"type"`
	Score      float64 `json:"score,omitempty"`
	Position   string  `json:"position"`
	TokenCount int     `json:"token_count"`
	Truncated  bool    `json:"truncated"`
}

// ContextZoneStatsResponse represents zone statistics.
type ContextZoneStatsResponse struct {
	CriticalUsed   int `json:"critical_used"`
	CriticalBudget int `json:"critical_budget"`
	MiddleUsed     int `json:"middle_used"`
	MiddleBudget   int `json:"middle_budget"`
	RecencyUsed    int `json:"recency_used"`
	RecencyBudget  int `json:"recency_budget"`
}

// Context handler - performs query analysis, retrieval, and context assembly
func (s *Server) getContext(c *gin.Context) {
	var req GetContextRequestExtended
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid request body",
			Details: err.Error(),
		})
		return
	}

	ctx := c.Request.Context()

	// Set defaults
	namespace := req.Namespace
	if namespace == "" {
		namespace = s.cfg.Memory.DefaultNamespace
	}

	tokenBudget := req.TokenBudget
	if tokenBudget <= 0 {
		tokenBudget = s.cfg.Memory.DefaultTokenBudget
	}

	// Step 1: Analyze the query
	analysis, err := s.analyzer.Analyze(ctx, req.Query)
	if err != nil {
		s.logger.Error("query analysis failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "query analysis failed",
			Code:  "ANALYSIS_ERROR",
		})
		return
	}

	// Step 2: Retrieve relevant memories
	var retrievalResults *retrieval.Results
	if s.retriever != nil {
		// Use the retriever if available
		retrievalResults, err = s.retriever.Retrieve(ctx, req.Query, &retrieval.RetrieveOptions{
			Namespace: namespace,
			Limit:     50,
			MinScore:  req.MinScore,
			UseVector: true,
			UseText:   true,
			Analysis:  analysis,
		})
		if err != nil {
			s.logger.Error("retrieval failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: "retrieval failed",
				Code:  "RETRIEVAL_ERROR",
			})
			return
		}
	} else {
		// Fallback to basic storage search
		searchOpts := &storage.SearchOptions{
			Namespace: namespace,
			Limit:     50,
		}
		storageResults, err := s.store.SearchMemories(ctx, searchOpts)
		if err != nil {
			s.handleStorageError(c, err)
			return
		}

		// Convert to retrieval results
		items := make([]*retrieval.Result, len(storageResults))
		for i, r := range storageResults {
			items[i] = &retrieval.Result{
				Memory: r.Memory,
				Score:  r.Score,
			}
		}
		retrievalResults = &retrieval.Results{
			Items: items,
			Total: len(items),
		}
	}

	// Step 3: Assemble context with position awareness
	assembleOpts := &mcontext.AssembleOptions{
		TokenBudget:   tokenBudget,
		SystemPrompt:  req.SystemPrompt,
		IncludeScores: req.IncludeScores,
	}

	assembled, err := s.assembler.Assemble(ctx, retrievalResults, assembleOpts)
	if err != nil {
		s.logger.Error("context assembly failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "context assembly failed",
			Code:  "ASSEMBLY_ERROR",
		})
		return
	}

	// Build response
	memories := make([]*ContextMemoryResponse, len(assembled.Memories))
	for i, m := range assembled.Memories {
		memories[i] = &ContextMemoryResponse{
			ID:         m.Memory.ID,
			Content:    m.Memory.Content,
			Type:       string(m.Memory.Type),
			Score:      m.Score,
			Position:   positionToString(m.Position),
			TokenCount: m.TokenCount,
			Truncated:  m.Truncated,
		}
	}

	response := ContextResponse{
		Content:     assembled.Content,
		Memories:    memories,
		TokenCount:  assembled.TokenCount,
		TokenBudget: tokenBudget,
		Truncated:   assembled.Truncated,
		ZoneStats: &ContextZoneStatsResponse{
			CriticalUsed:   assembled.ZoneStats.CriticalUsed,
			CriticalBudget: assembled.ZoneStats.CriticalBudget,
			MiddleUsed:     assembled.ZoneStats.MiddleUsed,
			MiddleBudget:   assembled.ZoneStats.MiddleBudget,
			RecencyUsed:    assembled.ZoneStats.RecencyUsed,
			RecencyBudget:  assembled.ZoneStats.RecencyBudget,
		},
		QueryTime: assembled.AssemblyTime.String(),
	}

	c.JSON(http.StatusOK, response)
}

// positionToString converts a Position to a string.
func positionToString(p mcontext.Position) string {
	switch p {
	case mcontext.PositionCritical:
		return "critical"
	case mcontext.PositionMiddle:
		return "middle"
	case mcontext.PositionRecency:
		return "recency"
	default:
		return "unknown"
	}
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

// Inference health handler types

// InferenceHealthResponse represents the health status of inference providers.
type InferenceHealthResponse struct {
	Enabled   bool                          `json:"enabled"`
	Providers map[string]*ProviderHealthDTO `json:"providers,omitempty"`
}

// ProviderHealthDTO represents the health status of a single provider.
type ProviderHealthDTO struct {
	Status            string `json:"status"`
	LastCheck         string `json:"last_check,omitempty"`
	LastError         string `json:"last_error,omitempty"`
	ConsecutiveErrors int    `json:"consecutive_errors"`
	ConsecutiveOK     int    `json:"consecutive_ok"`
}

// getInferenceHealth returns the health status of all inference providers.
func (s *Server) getInferenceHealth(c *gin.Context) {
	if s.inferenceRouter == nil {
		c.JSON(http.StatusOK, InferenceHealthResponse{
			Enabled: false,
		})
		return
	}

	healthChecker := s.inferenceRouter.GetHealthChecker()
	if healthChecker == nil {
		// Inference enabled but no health checker
		providers := make(map[string]*ProviderHealthDTO)
		for _, p := range s.inferenceRouter.ListProviders() {
			providers[p.Name()] = &ProviderHealthDTO{
				Status: "unknown",
			}
		}
		c.JSON(http.StatusOK, InferenceHealthResponse{
			Enabled:   true,
			Providers: providers,
		})
		return
	}

	// Get health for all providers
	allHealth := healthChecker.GetAllHealth()
	providers := make(map[string]*ProviderHealthDTO)

	for name, health := range allHealth {
		dto := &ProviderHealthDTO{
			Status:            string(health.Status),
			ConsecutiveErrors: health.ConsecutiveErrors,
			ConsecutiveOK:     health.ConsecutiveOK,
		}
		if !health.LastCheck.IsZero() {
			dto.LastCheck = health.LastCheck.Format("2006-01-02T15:04:05Z07:00")
		}
		if health.LastError != nil {
			dto.LastError = health.LastError.Error()
		}
		providers[name] = dto
	}

	c.JSON(http.StatusOK, InferenceHealthResponse{
		Enabled:   true,
		Providers: providers,
	})
}

// checkInferenceProviderHealth checks the health of a specific provider.
func (s *Server) checkInferenceProviderHealth(c *gin.Context) {
	providerName := c.Param("name")

	if s.inferenceRouter == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "inference not enabled",
			Code:  "INFERENCE_DISABLED",
		})
		return
	}

	// Check if provider exists
	provider, exists := s.inferenceRouter.GetProvider(providerName)
	if !exists {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "provider not found",
			Code:  "PROVIDER_NOT_FOUND",
		})
		return
	}

	healthChecker := s.inferenceRouter.GetHealthChecker()
	if healthChecker == nil {
		// No health checker, perform a direct health check
		err := provider.Health(c.Request.Context())
		status := "healthy"
		var errMsg string
		if err != nil {
			status = "unhealthy"
			errMsg = err.Error()
		}
		c.JSON(http.StatusOK, ProviderHealthDTO{
			Status:    status,
			LastError: errMsg,
		})
		return
	}

	// Perform immediate health check
	if err := healthChecker.CheckNow(c.Request.Context(), providerName); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "failed to check provider health",
			Code:  "HEALTH_CHECK_FAILED",
		})
		return
	}

	health, exists := healthChecker.GetHealth(providerName)
	if !exists {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "provider health not found",
			Code:  "HEALTH_NOT_FOUND",
		})
		return
	}

	dto := &ProviderHealthDTO{
		Status:            string(health.Status),
		ConsecutiveErrors: health.ConsecutiveErrors,
		ConsecutiveOK:     health.ConsecutiveOK,
	}
	if !health.LastCheck.IsZero() {
		dto.LastCheck = health.LastCheck.Format("2006-01-02T15:04:05Z07:00")
	}
	if health.LastError != nil {
		dto.LastError = health.LastError.Error()
	}

	c.JSON(http.StatusOK, dto)
}

// CacheStatsResponse represents cache statistics.
type CacheStatsResponse struct {
	Enabled    bool   `json:"enabled"`
	Hits       int64  `json:"hits"`
	Misses     int64  `json:"misses"`
	Evictions  int64  `json:"evictions"`
	Size       int    `json:"size"`
	LastAccess string `json:"last_access,omitempty"`
}

// getInferenceCacheStats returns the cache statistics.
func (s *Server) getInferenceCacheStats(c *gin.Context) {
	if s.inferenceCache == nil {
		c.JSON(http.StatusOK, CacheStatsResponse{
			Enabled: false,
		})
		return
	}

	stats := s.inferenceCache.Stats()
	resp := CacheStatsResponse{
		Enabled:   true,
		Hits:      stats.Hits,
		Misses:    stats.Misses,
		Evictions: stats.Evictions,
		Size:      stats.Size,
	}
	if !stats.LastAccess.IsZero() {
		resp.LastAccess = stats.LastAccess.Format("2006-01-02T15:04:05Z07:00")
	}

	c.JSON(http.StatusOK, resp)
}

// clearInferenceCache clears all cached responses.
func (s *Server) clearInferenceCache(c *gin.Context) {
	if s.inferenceCache == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "cache not enabled",
			Code:  "CACHE_DISABLED",
		})
		return
	}

	s.inferenceCache.Clear(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{
		"message": "cache cleared",
	})
}
