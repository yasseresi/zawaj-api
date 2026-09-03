package handler

import (
	"net/http"

	"zawaj/internal/apperr"
	"zawaj/internal/auth"
	"zawaj/internal/middleware"
	"zawaj/internal/models"
	"zawaj/internal/repository"
	"zawaj/internal/service"
	"zawaj/pkg/response"

	"github.com/gin-gonic/gin"
)

// Stats is the analytics module, mounted under /weddings/:id/stats (screen 8).
type Stats struct {
	svc    *service.StatsService
	repo   *repository.WeddingRepo // for RequireRole
	tokens *auth.Manager
}

// NewStats builds the module.
func NewStats(svc *service.StatsService, repo *repository.WeddingRepo, tokens *auth.Manager) *Stats {
	return &Stats{svc: svc, repo: repo, tokens: tokens}
}

// Register mounts the stats route. Reads require membership (viewer).
func (h *Stats) Register(rg *gin.RouterGroup) {
	viewer := models.RoleViewer

	g := rg.Group("/weddings/:id/stats", middleware.RequireAuth(h.tokens))
	g.GET("", middleware.RequireRole(h.repo, viewer), h.get)
}

// get godoc
// @Summary  Guest statistics
// @Description Counts by RSVP status, companions, and totals for a wedding.
// @Tags     stats
// @Produce  json
// @Security BearerAuth
// @Param    id path string true "Wedding ID"
// @Success  200 {object} response.Envelope
// @Failure  403 {object} response.Envelope{error=response.APIError}
// @Router   /weddings/{id}/stats [get]
func (h *Stats) get(c *gin.Context) {
	res, err := h.svc.Guests(c.Request.Context(), middleware.WeddingID(c))
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, res)
}

