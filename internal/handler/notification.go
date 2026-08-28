package handler

import (
	"net/http"

	"zawaj/internal/apperr"
	"zawaj/internal/auth"
	"zawaj/internal/middleware"
	"zawaj/internal/service"
	"zawaj/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Notification is the personal notifications module, mounted under /notifications.
// These routes are per-user (not wedding-scoped): the recipient is always the
// authenticated caller, so they require auth but no wedding role.
type Notification struct {
	svc    *service.NotificationService
	tokens *auth.Manager
}

// NewNotification builds the module.
func NewNotification(svc *service.NotificationService, tokens *auth.Manager) *Notification {
	return &Notification{svc: svc, tokens: tokens}
}

// Register mounts notification routes. All require an authenticated user.
func (h *Notification) Register(rg *gin.RouterGroup) {
	g := rg.Group("/notifications", middleware.RequireAuth(h.tokens))
	g.GET("", h.list)
	g.GET("/unread_count", h.unreadCount)
	g.PATCH("/:id/read", h.markRead)
	g.POST("/read_all", h.markAllRead)
}

// list godoc
// @Summary  List notifications
// @Tags     notifications
// @Produce  json
// @Security BearerAuth
// @Param    unread query bool false "Only unread when true"
// @Param    page query int false "Page (default 1)"
// @Param    page_size query int false "Page size (default 50, max 200)"
// @Success  200 {object} response.Envelope{data=object} "{ items: [...] }"
// @Failure  401 {object} response.Envelope{error=response.APIError}
// @Router   /notifications [get]
func (h *Notification) list(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	page, size := paging(c)
	unread := c.Query("unread") == "true"
	items, err := h.svc.List(c.Request.Context(), userID, unread, size, (page-1)*size)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"items": items})
}

// unreadCount godoc
// @Summary  Unread notification count
// @Tags     notifications
// @Produce  json
// @Security BearerAuth
// @Success  200 {object} response.Envelope{data=object} "{ count: N }"
// @Failure  401 {object} response.Envelope{error=response.APIError}
// @Router   /notifications/unread_count [get]
func (h *Notification) unreadCount(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	count, err := h.svc.UnreadCount(c.Request.Context(), userID)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"count": count})
}

// markRead godoc
// @Summary  Mark a notification read
// @Tags     notifications
// @Produce  json
// @Security BearerAuth
// @Param    id path string true "Notification ID"
// @Success  200 {object} response.Envelope{data=object} "{ read: true }"
// @Failure  401 {object} response.Envelope{error=response.APIError}
// @Failure  404 {object} response.Envelope{error=response.APIError}
// @Router   /notifications/{id}/read [patch]
func (h *Notification) markRead(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apperr.Write(c, apperr.NotFound("notification not found"))
		return
	}
	if err := h.svc.MarkRead(c.Request.Context(), userID, id); err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"read": true})
}

// markAllRead godoc
// @Summary  Mark all notifications read
// @Tags     notifications
// @Produce  json
// @Security BearerAuth
// @Success  200 {object} response.Envelope{data=object} "{ read_all: true }"
// @Failure  401 {object} response.Envelope{error=response.APIError}
// @Router   /notifications/read_all [post]
func (h *Notification) markAllRead(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	if err := h.svc.MarkAllRead(c.Request.Context(), userID); err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"read_all": true})
}
