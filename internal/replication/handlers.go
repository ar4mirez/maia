package replication

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Handler provides HTTP handlers for replication endpoints.
type Handler struct {
	manager   *Manager
	wal       WAL
	migration *MigrationExecutor
	routing   *RoutingMiddleware
	logger    *zap.Logger
}

// HandlerConfig configures the replication handler.
type HandlerConfig struct {
	Manager   *Manager
	WAL       WAL
	Migration *MigrationExecutor
	Routing   *RoutingMiddleware
	Logger    *zap.Logger
}

// NewHandler creates a new replication HTTP handler.
func NewHandler(manager *Manager, wal WAL, logger *zap.Logger) *Handler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Handler{
		manager: manager,
		wal:     wal,
		logger:  logger,
	}
}

// NewHandlerWithConfig creates a handler with full configuration.
func NewHandlerWithConfig(cfg *HandlerConfig) *Handler {
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Handler{
		manager:   cfg.Manager,
		wal:       cfg.WAL,
		migration: cfg.Migration,
		routing:   cfg.Routing,
		logger:    logger,
	}
}

// RegisterRoutes registers replication routes on the given router group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	repl := rg.Group("/replication")
	{
		// WAL operations
		repl.GET("/entries", h.getEntries)
		repl.POST("/entries", h.receiveEntries)

		// Position and status
		repl.GET("/position", h.getPosition)
		repl.GET("/stats", h.getStats)
		repl.GET("/health", h.getReplicationHealth)

		// Leader/follower management
		repl.GET("/leader", h.getLeaderInfo)
		repl.PUT("/leader", h.setLeader)
		repl.GET("/followers", h.listFollowers)
		repl.POST("/followers", h.addFollower)
		repl.GET("/followers/:id", h.getFollowerStatus)
		repl.DELETE("/followers/:id", h.removeFollower)
	}

	// Tenant placement API
	placement := rg.Group("/placements")
	{
		placement.GET("/:tenant_id", h.getTenantPlacement)
		placement.PUT("/:tenant_id", h.setTenantPlacement)
		placement.DELETE("/:tenant_id", h.deleteTenantPlacement)
	}
}

// RegisterAdminRoutes registers admin routes for migrations.
func (h *Handler) RegisterAdminRoutes(admin *gin.RouterGroup) {
	if h.migration == nil {
		return
	}

	// Migration endpoints
	admin.POST("/tenants/:id/migrate", h.startMigration)
	admin.GET("/tenants/:id/migrations", h.listTenantMigrations)
	admin.GET("/migrations", h.listAllMigrations)
	admin.GET("/migrations/:id", h.getMigration)
	admin.POST("/migrations/:id/cancel", h.cancelMigration)
}

// getEntries returns WAL entries after a given sequence.
func (h *Handler) getEntries(c *gin.Context) {
	afterStr := c.DefaultQuery("after", "0")
	after, err := strconv.ParseUint(afterStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid 'after' parameter",
			"code":  "INVALID_PARAMETER",
		})
		return
	}

	limitStr := c.DefaultQuery("limit", "100")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	entries, err := h.wal.Read(c.Request.Context(), after, limit)
	if err != nil {
		h.logger.Error("failed to read WAL entries", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to read entries",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	// Add replication headers
	c.Header("X-MAIA-Region", h.manager.Region())
	c.Header("X-MAIA-Instance-ID", h.manager.cfg.InstanceID)
	c.Header("X-MAIA-Role", string(h.manager.Role()))

	c.JSON(http.StatusOK, entries)
}

// receiveEntries receives WAL entries from the leader (for followers).
func (h *Handler) receiveEntries(c *gin.Context) {
	if h.manager.Role() != RoleFollower {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "this instance is not a follower",
			"code":  "NOT_FOLLOWER",
		})
		return
	}

	var entries []*WALEntry
	if err := c.ShouldBindJSON(&entries); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
			"code":  "INVALID_REQUEST",
		})
		return
	}

	// Apply entries
	applied := 0
	for _, entry := range entries {
		if err := h.manager.applyEntry(c.Request.Context(), entry); err != nil {
			h.logger.Error("failed to apply entry",
				zap.String("entry_id", entry.ID),
				zap.Error(err),
			)
			break
		}
		applied++
	}

	c.JSON(http.StatusOK, gin.H{
		"applied": applied,
		"total":   len(entries),
	})
}

// getPosition returns the current WAL position.
func (h *Handler) getPosition(c *gin.Context) {
	pos, err := h.manager.Position(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to get WAL position", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get position",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, pos)
}

// getStats returns replication statistics.
func (h *Handler) getStats(c *gin.Context) {
	stats, err := h.manager.Stats(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to get replication stats", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get stats",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// getLeaderInfo returns information about the current leader.
func (h *Handler) getLeaderInfo(c *gin.Context) {
	info, err := h.manager.GetLeaderInfo(c.Request.Context())
	if err != nil {
		if err == ErrNotFollower {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "this instance is not a follower",
				"code":  "NOT_FOLLOWER",
			})
			return
		}
		if err == ErrLeaderUnavailable {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "leader is unavailable",
				"code":  "LEADER_UNAVAILABLE",
			})
			return
		}
		h.logger.Error("failed to get leader info", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get leader info",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, info)
}

// setLeader configures the leader endpoint (for followers).
func (h *Handler) setLeader(c *gin.Context) {
	var req struct {
		Endpoint string `json:"endpoint" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "endpoint is required",
			"code":  "INVALID_REQUEST",
		})
		return
	}

	if err := h.manager.SetLeader(c.Request.Context(), req.Endpoint); err != nil {
		if err == ErrNotFollower {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "this instance is not a follower",
				"code":  "NOT_FOLLOWER",
			})
			return
		}
		h.logger.Error("failed to set leader", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to set leader",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "leader configured",
		"endpoint": req.Endpoint,
	})
}

// listFollowers returns all registered followers.
func (h *Handler) listFollowers(c *gin.Context) {
	followers, err := h.manager.ListFollowers(c.Request.Context())
	if err != nil {
		if err == ErrNotLeader {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "this instance is not a leader",
				"code":  "NOT_LEADER",
			})
			return
		}
		h.logger.Error("failed to list followers", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to list followers",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"followers": followers,
	})
}

// addFollower adds a new follower.
func (h *Handler) addFollower(c *gin.Context) {
	var cfg FollowerConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid follower configuration",
			"code":  "INVALID_REQUEST",
		})
		return
	}

	if err := h.manager.AddFollower(c.Request.Context(), &cfg); err != nil {
		if err == ErrNotLeader {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "this instance is not a leader",
				"code":  "NOT_LEADER",
			})
			return
		}
		h.logger.Error("failed to add follower", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to add follower",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":     "follower added",
		"follower_id": cfg.ID,
	})
}

// getFollowerStatus returns the status of a specific follower.
func (h *Handler) getFollowerStatus(c *gin.Context) {
	id := c.Param("id")

	status, err := h.manager.GetFollowerStatus(c.Request.Context(), id)
	if err != nil {
		if err == ErrNotLeader {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "this instance is not a leader",
				"code":  "NOT_LEADER",
			})
			return
		}
		if err == ErrFollowerNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "follower not found",
				"code":  "FOLLOWER_NOT_FOUND",
			})
			return
		}
		h.logger.Error("failed to get follower status", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get follower status",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, status)
}

// removeFollower removes a follower.
func (h *Handler) removeFollower(c *gin.Context) {
	id := c.Param("id")

	if err := h.manager.RemoveFollower(c.Request.Context(), id); err != nil {
		if err == ErrNotLeader {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "this instance is not a leader",
				"code":  "NOT_LEADER",
			})
			return
		}
		if err == ErrFollowerNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "follower not found",
				"code":  "FOLLOWER_NOT_FOUND",
			})
			return
		}
		h.logger.Error("failed to remove follower", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to remove follower",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "follower removed",
		"follower_id": id,
	})
}

// getTenantPlacement returns the placement for a tenant.
func (h *Handler) getTenantPlacement(c *gin.Context) {
	tenantID := c.Param("tenant_id")

	placement, err := h.manager.GetTenantPlacement(c.Request.Context(), tenantID)
	if err != nil {
		if err == ErrTenantNotReplicated {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "tenant placement not found",
				"code":  "PLACEMENT_NOT_FOUND",
			})
			return
		}
		h.logger.Error("failed to get tenant placement", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get tenant placement",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, placement)
}

// setTenantPlacement configures placement for a tenant.
func (h *Handler) setTenantPlacement(c *gin.Context) {
	tenantID := c.Param("tenant_id")

	var placement TenantPlacement
	if err := c.ShouldBindJSON(&placement); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid placement configuration",
			"code":  "INVALID_REQUEST",
		})
		return
	}

	placement.TenantID = tenantID

	if err := h.manager.SetTenantPlacement(c.Request.Context(), &placement); err != nil {
		if err == ErrInvalidPlacement {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
				"code":  "INVALID_PLACEMENT",
			})
			return
		}
		h.logger.Error("failed to set tenant placement", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to set tenant placement",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "placement configured",
		"tenant_id": tenantID,
	})
}

// deleteTenantPlacement removes placement configuration for a tenant.
func (h *Handler) deleteTenantPlacement(c *gin.Context) {
	tenantID := c.Param("tenant_id")

	// Set to single-region mode (effectively removing replication)
	placement := &TenantPlacement{
		TenantID:      tenantID,
		PrimaryRegion: h.manager.Region(),
		Mode:          PlacementSingle,
	}

	if err := h.manager.SetTenantPlacement(c.Request.Context(), placement); err != nil {
		h.logger.Error("failed to delete tenant placement", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to delete tenant placement",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "placement removed",
		"tenant_id": tenantID,
	})
}

// ReplicationHealthResponse contains the replication health status.
type ReplicationHealthResponse struct {
	Status    string                   `json:"status"`
	Role      Role                     `json:"role"`
	Region    string                   `json:"region"`
	WAL       *WALHealthInfo           `json:"wal"`
	Followers []FollowerHealthInfo     `json:"followers,omitempty"`
	Leader    *LeaderHealthInfo        `json:"leader,omitempty"`
}

// WALHealthInfo contains WAL health information.
type WALHealthInfo struct {
	Position *WALPosition `json:"position"`
	Size     int64        `json:"size"`
	Entries  int64        `json:"entries"`
}

// FollowerHealthInfo contains follower health information.
type FollowerHealthInfo struct {
	ID        string  `json:"id"`
	Region    string  `json:"region"`
	Connected bool    `json:"connected"`
	Lag       string  `json:"lag"`
	Healthy   bool    `json:"healthy"`
}

// LeaderHealthInfo contains leader health information.
type LeaderHealthInfo struct {
	ID        string `json:"id"`
	Endpoint  string `json:"endpoint"`
	Region    string `json:"region"`
	Connected bool   `json:"connected"`
}

// getReplicationHealth returns the overall replication health status.
func (h *Handler) getReplicationHealth(c *gin.Context) {
	stats, err := h.manager.Stats(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to get replication stats", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get replication health",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	// Determine overall status
	status := "healthy"
	var followers []FollowerHealthInfo
	var leader *LeaderHealthInfo

	if h.manager.Role() == RoleLeader {
		for _, f := range stats.Followers {
			healthy := f.Connected && f.Lag < h.manager.cfg.MaxReplicationLag
			if !healthy {
				status = "degraded"
			}
			followers = append(followers, FollowerHealthInfo{
				ID:        f.ID,
				Region:    f.Region,
				Connected: f.Connected,
				Lag:       f.Lag.String(),
				Healthy:   healthy,
			})
		}
	} else if h.manager.Role() == RoleFollower {
		if stats.Leader != nil {
			leader = &LeaderHealthInfo{
				ID:        stats.Leader.ID,
				Endpoint:  stats.Leader.Endpoint,
				Region:    stats.Leader.Region,
				Connected: true,
			}
		} else {
			status = "unhealthy"
		}
	}

	response := ReplicationHealthResponse{
		Status:    status,
		Role:      stats.Role,
		Region:    stats.Region,
		WAL: &WALHealthInfo{
			Position: stats.Position,
			Size:     stats.WALSize,
			Entries:  stats.WALEntries,
		},
		Followers: followers,
		Leader:    leader,
	}

	c.JSON(http.StatusOK, response)
}

// startMigration initiates a tenant migration.
func (h *Handler) startMigration(c *gin.Context) {
	if h.migration == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "migration service not available",
			"code":  "MIGRATION_UNAVAILABLE",
		})
		return
	}

	tenantID := c.Param("id")

	var req struct {
		ToRegion string `json:"to_region" binding:"required"`
		DryRun   bool   `json:"dry_run"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "to_region is required",
			"code":  "INVALID_REQUEST",
		})
		return
	}

	migration, err := h.migration.StartMigration(c.Request.Context(), tenantID, req.ToRegion, req.DryRun)
	if err != nil {
		switch {
		case err == ErrMigrationInProgress:
			c.JSON(http.StatusConflict, gin.H{
				"error": "migration already in progress for this tenant",
				"code":  "MIGRATION_IN_PROGRESS",
			})
		case err == ErrTargetRegionSameAsCurrent:
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "target region is same as current primary",
				"code":  "INVALID_TARGET_REGION",
			})
		case err == ErrInvalidPlacement:
			c.JSON(http.StatusNotFound, gin.H{
				"error": "tenant placement not found",
				"code":  "PLACEMENT_NOT_FOUND",
			})
		default:
			h.logger.Error("failed to start migration", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "failed to start migration",
				"code":  "INTERNAL_ERROR",
			})
		}
		return
	}

	status := http.StatusAccepted
	if req.DryRun {
		status = http.StatusOK
	}

	c.JSON(status, gin.H{
		"migration": migration,
	})
}

// getMigration returns a migration by ID.
func (h *Handler) getMigration(c *gin.Context) {
	if h.migration == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "migration service not available",
			"code":  "MIGRATION_UNAVAILABLE",
		})
		return
	}

	migrationID := c.Param("id")

	migration, err := h.migration.GetMigration(c.Request.Context(), migrationID)
	if err != nil {
		if err == ErrMigrationNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "migration not found",
				"code":  "MIGRATION_NOT_FOUND",
			})
			return
		}
		h.logger.Error("failed to get migration", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get migration",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"migration": migration,
	})
}

// cancelMigration cancels an in-progress migration.
func (h *Handler) cancelMigration(c *gin.Context) {
	if h.migration == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "migration service not available",
			"code":  "MIGRATION_UNAVAILABLE",
		})
		return
	}

	migrationID := c.Param("id")

	if err := h.migration.CancelMigration(c.Request.Context(), migrationID); err != nil {
		if err == ErrMigrationNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "migration not found",
				"code":  "MIGRATION_NOT_FOUND",
			})
			return
		}
		h.logger.Error("failed to cancel migration", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to cancel migration",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "migration cancelled",
	})
}

// listTenantMigrations returns migrations for a specific tenant.
func (h *Handler) listTenantMigrations(c *gin.Context) {
	if h.migration == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "migration service not available",
			"code":  "MIGRATION_UNAVAILABLE",
		})
		return
	}

	tenantID := c.Param("id")

	migrations, err := h.migration.ListMigrations(c.Request.Context(), tenantID)
	if err != nil {
		h.logger.Error("failed to list migrations", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to list migrations",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"migrations": migrations,
	})
}

// listAllMigrations returns all migrations.
func (h *Handler) listAllMigrations(c *gin.Context) {
	if h.migration == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "migration service not available",
			"code":  "MIGRATION_UNAVAILABLE",
		})
		return
	}

	migrations, err := h.migration.ListAllMigrations(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to list all migrations", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to list migrations",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"migrations": migrations,
	})
}
