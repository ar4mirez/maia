package server

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ar4mirez/maia/internal/tenant"
)

// Admin API request/response types

// CreateTenantRequest represents the request to create a tenant.
type CreateTenantRequest struct {
	Name     string                 `json:"name" binding:"required"`
	Plan     tenant.Plan            `json:"plan,omitempty"`
	Config   tenant.Config          `json:"config,omitempty"`
	Quotas   tenant.Quotas          `json:"quotas,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// UpdateTenantRequest represents the request to update a tenant.
type UpdateTenantRequest struct {
	Name     *string                `json:"name,omitempty"`
	Plan     *tenant.Plan           `json:"plan,omitempty"`
	Config   *tenant.Config         `json:"config,omitempty"`
	Quotas   *tenant.Quotas         `json:"quotas,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// TenantUsageResponse represents tenant usage statistics.
type TenantUsageResponse struct {
	Tenant *tenant.Tenant `json:"tenant"`
	Usage  *tenant.Usage  `json:"usage"`
}

// CreateAPIKeyRequest represents the request to create an API key.
type CreateAPIKeyRequest struct {
	Name      string                 `json:"name" binding:"required"`
	Scopes    []string               `json:"scopes,omitempty"`
	ExpiresAt string                 `json:"expires_at,omitempty"` // RFC3339 format
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// CreateAPIKeyResponse represents the response when creating an API key.
type CreateAPIKeyResponse struct {
	APIKey *tenant.APIKey `json:"api_key"`
	Key    string         `json:"key"` // The raw key (only returned once)
}

// createTenant handles tenant creation.
func (s *Server) createTenant(c *gin.Context) {
	var req CreateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "invalid request body",
			Code:  "INVALID_INPUT",
		})
		return
	}

	input := &tenant.CreateTenantInput{
		Name:     req.Name,
		Plan:     req.Plan,
		Config:   req.Config,
		Quotas:   req.Quotas,
		Metadata: req.Metadata,
	}

	t, err := s.tenants.Create(c.Request.Context(), input)
	if err != nil {
		if _, ok := err.(*tenant.ErrAlreadyExists); ok {
			c.JSON(http.StatusConflict, ErrorResponse{
				Error: err.Error(),
				Code:  "ALREADY_EXISTS",
			})
			return
		}
		if _, ok := err.(*tenant.ErrInvalidInput); ok {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: err.Error(),
				Code:  "INVALID_INPUT",
			})
			return
		}

		s.logger.Error("failed to create tenant", logError(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "internal server error",
			Code:  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusCreated, t)
}

// getTenant handles getting a tenant by ID.
func (s *Server) getTenant(c *gin.Context) {
	id := c.Param("id")

	t, err := s.tenants.Get(c.Request.Context(), id)
	if err != nil {
		if _, ok := err.(*tenant.ErrNotFound); ok {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error: err.Error(),
				Code:  "NOT_FOUND",
			})
			return
		}

		s.logger.Error("failed to get tenant", logError(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "internal server error",
			Code:  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, t)
}

// updateTenant handles tenant updates.
func (s *Server) updateTenant(c *gin.Context) {
	id := c.Param("id")

	var req UpdateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "invalid request body",
			Code:  "INVALID_INPUT",
		})
		return
	}

	input := &tenant.UpdateTenantInput{
		Name:     req.Name,
		Plan:     req.Plan,
		Config:   req.Config,
		Quotas:   req.Quotas,
		Metadata: req.Metadata,
	}

	t, err := s.tenants.Update(c.Request.Context(), id, input)
	if err != nil {
		if _, ok := err.(*tenant.ErrNotFound); ok {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error: err.Error(),
				Code:  "NOT_FOUND",
			})
			return
		}
		if _, ok := err.(*tenant.ErrAlreadyExists); ok {
			c.JSON(http.StatusConflict, ErrorResponse{
				Error: err.Error(),
				Code:  "ALREADY_EXISTS",
			})
			return
		}

		s.logger.Error("failed to update tenant", logError(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "internal server error",
			Code:  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, t)
}

// deleteTenant handles tenant deletion.
func (s *Server) deleteTenant(c *gin.Context) {
	id := c.Param("id")

	// Prevent deletion of system tenant
	if id == tenant.SystemTenantID {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error: "cannot delete system tenant",
			Code:  "FORBIDDEN",
		})
		return
	}

	if err := s.tenants.Delete(c.Request.Context(), id); err != nil {
		if _, ok := err.(*tenant.ErrNotFound); ok {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error: err.Error(),
				Code:  "NOT_FOUND",
			})
			return
		}

		s.logger.Error("failed to delete tenant", logError(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "internal server error",
			Code:  "INTERNAL_ERROR",
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// listTenants handles listing tenants with optional filtering.
func (s *Server) listTenants(c *gin.Context) {
	opts := &tenant.ListTenantsOptions{
		Limit:  parseIntQuery(c, "limit", 100),
		Offset: parseIntQuery(c, "offset", 0),
	}

	// Parse status filter
	if status := c.Query("status"); status != "" {
		opts.Status = tenant.Status(status)
	}

	// Parse plan filter
	if plan := c.Query("plan"); plan != "" {
		opts.Plan = tenant.Plan(plan)
	}

	tenants, err := s.tenants.List(c.Request.Context(), opts)
	if err != nil {
		s.logger.Error("failed to list tenants", logError(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "internal server error",
			Code:  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, ListResponse{
		Data:   tenants,
		Count:  len(tenants),
		Offset: opts.Offset,
		Limit:  opts.Limit,
	})
}

// getTenantUsage handles getting tenant usage statistics.
func (s *Server) getTenantUsage(c *gin.Context) {
	id := c.Param("id")

	t, err := s.tenants.Get(c.Request.Context(), id)
	if err != nil {
		if _, ok := err.(*tenant.ErrNotFound); ok {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error: err.Error(),
				Code:  "NOT_FOUND",
			})
			return
		}

		s.logger.Error("failed to get tenant", logError(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "internal server error",
			Code:  "INTERNAL_ERROR",
		})
		return
	}

	usage, err := s.tenants.GetUsage(c.Request.Context(), id)
	if err != nil {
		s.logger.Error("failed to get tenant usage", logError(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "internal server error",
			Code:  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, TenantUsageResponse{
		Tenant: t,
		Usage:  usage,
	})
}

// suspendTenant handles suspending a tenant.
func (s *Server) suspendTenant(c *gin.Context) {
	id := c.Param("id")

	// Prevent suspension of system tenant
	if id == tenant.SystemTenantID {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error: "cannot suspend system tenant",
			Code:  "FORBIDDEN",
		})
		return
	}

	if err := s.tenants.Suspend(c.Request.Context(), id); err != nil {
		if _, ok := err.(*tenant.ErrNotFound); ok {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error: err.Error(),
				Code:  "NOT_FOUND",
			})
			return
		}

		s.logger.Error("failed to suspend tenant", logError(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "internal server error",
			Code:  "INTERNAL_ERROR",
		})
		return
	}

	// Return updated tenant
	t, err := s.tenants.Get(c.Request.Context(), id)
	if err != nil {
		s.logger.Error("failed to get suspended tenant", logError(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "internal server error",
			Code:  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, t)
}

// activateTenant handles activating a suspended tenant.
func (s *Server) activateTenant(c *gin.Context) {
	id := c.Param("id")

	if err := s.tenants.Activate(c.Request.Context(), id); err != nil {
		if _, ok := err.(*tenant.ErrNotFound); ok {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error: err.Error(),
				Code:  "NOT_FOUND",
			})
			return
		}

		s.logger.Error("failed to activate tenant", logError(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "internal server error",
			Code:  "INTERNAL_ERROR",
		})
		return
	}

	// Return updated tenant
	t, err := s.tenants.Get(c.Request.Context(), id)
	if err != nil {
		s.logger.Error("failed to get activated tenant", logError(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "internal server error",
			Code:  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, t)
}

// logError creates a zap field for error logging.
func logError(err error) zap.Field {
	return zap.Error(err)
}

// createAPIKey handles creating an API key for a tenant.
func (s *Server) createAPIKey(c *gin.Context) {
	tenantID := c.Param("id")

	// Check if tenant manager implements APIKeyManager
	apiKeyManager, ok := s.tenants.(tenant.APIKeyManager)
	if !ok {
		c.JSON(http.StatusNotImplemented, ErrorResponse{
			Error: "API key management not supported",
			Code:  "NOT_IMPLEMENTED",
		})
		return
	}

	var req CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "invalid request body",
			Code:  "INVALID_INPUT",
		})
		return
	}

	input := &tenant.CreateAPIKeyInput{
		TenantID: tenantID,
		Name:     req.Name,
		Scopes:   req.Scopes,
		Metadata: req.Metadata,
	}

	// Parse expiration time if provided
	if req.ExpiresAt != "" {
		expiresAt, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: "invalid expires_at format, expected RFC3339",
				Code:  "INVALID_INPUT",
			})
			return
		}
		input.ExpiresAt = expiresAt
	}

	apiKey, rawKey, err := apiKeyManager.CreateAPIKey(c.Request.Context(), input)
	if err != nil {
		if _, ok := err.(*tenant.ErrNotFound); ok {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error: err.Error(),
				Code:  "NOT_FOUND",
			})
			return
		}
		if _, ok := err.(*tenant.ErrInvalidInput); ok {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: err.Error(),
				Code:  "INVALID_INPUT",
			})
			return
		}

		s.logger.Error("failed to create API key", logError(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "internal server error",
			Code:  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusCreated, CreateAPIKeyResponse{
		APIKey: apiKey,
		Key:    rawKey,
	})
}

// listAPIKeys handles listing API keys for a tenant.
func (s *Server) listAPIKeys(c *gin.Context) {
	tenantID := c.Param("id")

	// Check if tenant manager implements APIKeyManager
	apiKeyManager, ok := s.tenants.(tenant.APIKeyManager)
	if !ok {
		c.JSON(http.StatusNotImplemented, ErrorResponse{
			Error: "API key management not supported",
			Code:  "NOT_IMPLEMENTED",
		})
		return
	}

	apiKeys, err := apiKeyManager.ListAPIKeys(c.Request.Context(), tenantID)
	if err != nil {
		if _, ok := err.(*tenant.ErrInvalidInput); ok {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: err.Error(),
				Code:  "INVALID_INPUT",
			})
			return
		}

		s.logger.Error("failed to list API keys", logError(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "internal server error",
			Code:  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, ListResponse{
		Data:  apiKeys,
		Count: len(apiKeys),
	})
}

// revokeAPIKey handles revoking an API key.
func (s *Server) revokeAPIKey(c *gin.Context) {
	key := c.Param("key")

	// Check if tenant manager implements APIKeyManager
	apiKeyManager, ok := s.tenants.(tenant.APIKeyManager)
	if !ok {
		c.JSON(http.StatusNotImplemented, ErrorResponse{
			Error: "API key management not supported",
			Code:  "NOT_IMPLEMENTED",
		})
		return
	}

	if err := apiKeyManager.RevokeAPIKey(c.Request.Context(), key); err != nil {
		if _, ok := err.(*tenant.ErrAPIKeyNotFound); ok {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error: err.Error(),
				Code:  "NOT_FOUND",
			})
			return
		}

		s.logger.Error("failed to revoke API key", logError(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "internal server error",
			Code:  "INTERNAL_ERROR",
		})
		return
	}

	c.Status(http.StatusNoContent)
}
