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

// Activity is the activity-feed module, mounted under /weddings/:id/activity.
// It exposes the existing ActivityService.Feed as a paginated, newest-first list
// (screen 9); the client does the day-grouping.
type Activity struct {
	svc    *service.ActivityService
	repo   *repository.WeddingRepo // for RequireRole
	tokens *auth.Manager
}

// NewActivity builds the module.
func NewActivity(svc *service.ActivityService, repo *repository.WeddingRepo, tokens *auth.Manager) *Activity {
	return &Activity{svc: svc, repo: repo, tokens: tokens}
}

// Register mounts the activity feed route. Requires viewer membership.
func (h *Activity) Register(rg *gin.RouterGroup) {
	g := rg.Group("/weddings/:id/activity", middleware.RequireAuth(h.tokens))
	g.GET("", middleware.RequireRole(h.repo, models.RoleViewer), h.feed)
}

// feed godoc
// @Summary  Activity feed
// @Description Paginated, newest-first activity for a wedding.
// @Tags     activity
// @Produce  json
// @Security BearerAuth
// @Param    id path string true "Wedding ID"
// @Param    cursor query string false "Opaque keyset cursor from a previous response's next_cursor"
// @Param    page_size query int false "Page size (default 50, max 200)"
// @Success  200 {object} response.Envelope{data=object} "{ items: [...], next_cursor: string|null }"
// @Failure  403 {object} response.Envelope{error=response.APIError}
// @Router   /weddings/{id}/activity [get]
func (h *Activity) feed(c *gin.Context) {
	_, size := paging(c)
	items, next, err := h.svc.FeedCursor(c.Request.Context(), middleware.WeddingID(c), c.Query("cursor"), size)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"items": items, "next_cursor": nullable(next)})
}
