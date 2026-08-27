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

func (h *Activity) feed(c *gin.Context) {
	page, size := paging(c)
	items, err := h.svc.Feed(c.Request.Context(), middleware.WeddingID(c), size, (page-1)*size)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"items": items})
}
