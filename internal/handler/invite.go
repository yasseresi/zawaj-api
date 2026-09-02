package handler

import (
	"net/http"

	"zawaj/internal/apperr"
	"zawaj/internal/auth"
	"zawaj/internal/dto"
	"zawaj/internal/middleware"
	"zawaj/internal/models"
	"zawaj/internal/repository"
	"zawaj/internal/service"
	"zawaj/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Invite is the targeted-invites module (screen 10). Creating an invite is
// wedding-scoped (owner only); listing/accepting/declining are per-user.
type Invite struct {
	svc    *service.InviteService
	repo   *repository.WeddingRepo // for RequireRole on the create route
	tokens *auth.Manager
}

// NewInvite builds the module.
func NewInvite(svc *service.InviteService, repo *repository.WeddingRepo, tokens *auth.Manager) *Invite {
	return &Invite{svc: svc, repo: repo, tokens: tokens}
}

// Register mounts the invite routes.
func (h *Invite) Register(rg *gin.RouterGroup) {
	// Owner invites a user to a specific wedding.
	w := rg.Group("/weddings/:id/invites", middleware.RequireAuth(h.tokens))
	w.POST("", middleware.RequireRole(h.repo, models.RoleOwner), h.create)

	// The invitee's own inbox.
	me := rg.Group("/invites", middleware.RequireAuth(h.tokens))
	me.GET("", h.listMine)
	me.POST("/:inviteId/accept", h.accept)
	me.POST("/:inviteId/decline", h.decline)
}

// create godoc
// @Summary  Invite a user to a wedding
// @Description Owner invites a registered user (by username) with a role (editor|viewer).
// @Tags     invites
// @Accept   json
// @Produce  json
// @Security BearerAuth
// @Param    id path string true "Wedding ID"
// @Param    body body dto.InviteUserRequest true "Invite"
// @Success  201 {object} response.Envelope
// @Failure  403 {object} response.Envelope{error=response.APIError}
// @Failure  404 {object} response.Envelope{error=response.APIError}
// @Failure  409 {object} response.Envelope{error=response.APIError}
// @Router   /weddings/{id}/invites [post]
func (h *Invite) create(c *gin.Context) {
	inviter, _ := middleware.UserID(c)
	var req dto.InviteUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperr.Write(c, apperr.Validation(err.Error()))
		return
	}
	inv, err := h.svc.Invite(c.Request.Context(), middleware.WeddingID(c), inviter, req.Username, models.Role(req.Role))
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, inv)
}

// listMine godoc
// @Summary  My pending invites
// @Tags     invites
// @Produce  json
// @Security BearerAuth
// @Success  200 {object} response.Envelope "{ items: [...] }"
// @Router   /invites [get]
func (h *Invite) listMine(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	items, err := h.svc.ListMine(c.Request.Context(), userID)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"items": items})
}

// accept godoc
// @Summary  Accept an invite
// @Description Grants the caller membership per the invite's role.
// @Tags     invites
// @Produce  json
// @Security BearerAuth
// @Param    inviteId path string true "Invite ID"
// @Success  200 {object} response.Envelope
// @Failure  404 {object} response.Envelope{error=response.APIError}
// @Router   /invites/{inviteId}/accept [post]
func (h *Invite) accept(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	id, err := uuid.Parse(c.Param("inviteId"))
	if err != nil {
		apperr.Write(c, apperr.NotFound("invite not found"))
		return
	}
	w, serr := h.svc.Accept(c.Request.Context(), id, userID)
	if serr != nil {
		apperr.Write(c, serr)
		return
	}
	response.JSON(c, http.StatusOK, w)
}

// decline godoc
// @Summary  Decline an invite
// @Tags     invites
// @Produce  json
// @Security BearerAuth
// @Param    inviteId path string true "Invite ID"
// @Success  204 "No Content"
// @Failure  404 {object} response.Envelope{error=response.APIError}
// @Router   /invites/{inviteId}/decline [post]
func (h *Invite) decline(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	id, err := uuid.Parse(c.Param("inviteId"))
	if err != nil {
		apperr.Write(c, apperr.NotFound("invite not found"))
		return
	}
	if serr := h.svc.Decline(c.Request.Context(), id, userID); serr != nil {
		apperr.Write(c, serr)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"status": "declined"})
}
