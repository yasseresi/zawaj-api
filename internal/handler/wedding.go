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

// Wedding is the weddings/members/invite-links module.
type Wedding struct {
	svc    *service.WeddingService
	audit  *service.AuditService
	repo   *repository.WeddingRepo // used by RequireRole middleware
	tokens *auth.Manager
}

// NewWedding builds the module.
func NewWedding(svc *service.WeddingService, audit *service.AuditService, repo *repository.WeddingRepo, tokens *auth.Manager) *Wedding {
	return &Wedding{svc: svc, audit: audit, repo: repo, tokens: tokens}
}

// Register mounts wedding routes. Every route requires auth; wedding-scoped
// routes additionally pass RequireRole (default deny) — see ADR-004.
func (h *Wedding) Register(rg *gin.RouterGroup) {
	a := rg.Group("", middleware.RequireAuth(h.tokens))
	a.POST("/weddings", h.create)
	a.GET("/weddings", h.list)
	a.GET("/invite/:token", h.previewInvite)
	a.POST("/invite/:token/accept", h.acceptInvite)

	viewer := models.RoleViewer
	owner := models.RoleOwner

	w := a.Group("/weddings/:id")
	w.GET("", middleware.RequireRole(h.repo, viewer), h.get)
	w.PATCH("", middleware.RequireRole(h.repo, owner), h.update)
	w.DELETE("", middleware.RequireRole(h.repo, owner), h.remove)

	w.GET("/members", middleware.RequireRole(h.repo, viewer), h.members)
	w.PATCH("/members/:userId", middleware.RequireRole(h.repo, owner), h.setRole)
	w.DELETE("/members/:userId", middleware.RequireRole(h.repo, viewer), h.removeMember)
	w.POST("/transfer", middleware.RequireRole(h.repo, owner), h.transferOwnership)

	w.POST("/invite-links", middleware.RequireRole(h.repo, owner), h.createLink)
	w.GET("/invite-links", middleware.RequireRole(h.repo, owner), h.listLinks)
	w.DELETE("/invite-links/:linkId", middleware.RequireRole(h.repo, owner), h.revokeLink)
}

// create godoc
// @Summary  Create a wedding
// @Tags     weddings
// @Accept   json
// @Produce  json
// @Security BearerAuth
// @Param    body body dto.CreateWeddingRequest true "Wedding"
// @Success  201 {object} response.Envelope
// @Failure  400 {object} response.Envelope{error=response.APIError}
// @Failure  401 {object} response.Envelope{error=response.APIError}
// @Router   /weddings [post]
func (h *Wedding) create(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	var req dto.CreateWeddingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperr.Write(c, apperr.Validation(err.Error()))
		return
	}
	w, err := h.svc.Create(c.Request.Context(), userID, req)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, w)
}

// list godoc
// @Summary  List my weddings
// @Tags     weddings
// @Produce  json
// @Security BearerAuth
// @Success  200 {object} response.Envelope{data=object} "{ items: [...] }"
// @Failure  401 {object} response.Envelope{error=response.APIError}
// @Router   /weddings [get]
func (h *Wedding) list(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	items, err := h.svc.List(c.Request.Context(), userID)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"items": items})
}

// get godoc
// @Summary  Get a wedding
// @Tags     weddings
// @Produce  json
// @Security BearerAuth
// @Param    id path string true "Wedding ID"
// @Success  200 {object} response.Envelope{data=object} "{ wedding, my_role }"
// @Failure  401 {object} response.Envelope{error=response.APIError}
// @Failure  403 {object} response.Envelope{error=response.APIError}
// @Failure  404 {object} response.Envelope{error=response.APIError}
// @Router   /weddings/{id} [get]
func (h *Wedding) get(c *gin.Context) {
	w, err := h.svc.Get(c.Request.Context(), middleware.WeddingID(c))
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"wedding": w, "my_role": middleware.CurrentRole(c)})
}

// update godoc
// @Summary  Update a wedding (owner only)
// @Tags     weddings
// @Accept   json
// @Produce  json
// @Security BearerAuth
// @Param    id path string true "Wedding ID"
// @Param    body body dto.UpdateWeddingRequest true "Fields"
// @Success  200 {object} response.Envelope
// @Failure  400 {object} response.Envelope{error=response.APIError}
// @Failure  403 {object} response.Envelope{error=response.APIError}
// @Router   /weddings/{id} [patch]
func (h *Wedding) update(c *gin.Context) {
	var req dto.UpdateWeddingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperr.Write(c, apperr.Validation(err.Error()))
		return
	}
	w, err := h.svc.Update(c.Request.Context(), middleware.WeddingID(c), req)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, w)
}

// remove godoc
// @Summary  Delete a wedding (owner only)
// @Tags     weddings
// @Produce  json
// @Security BearerAuth
// @Param    id path string true "Wedding ID"
// @Success  200 {object} response.Envelope{data=object} "{ deleted: true }"
// @Failure  403 {object} response.Envelope{error=response.APIError}
// @Router   /weddings/{id} [delete]
func (h *Wedding) remove(c *gin.Context) {
	wid := middleware.WeddingID(c)
	if err := h.svc.Delete(c.Request.Context(), wid); err != nil {
		apperr.Write(c, err)
		return
	}
	caller, _ := middleware.UserID(c)
	h.audit.Record(c.Request.Context(), &caller, "wedding_deleted", "wedding", &wid, c.ClientIP(), nil)
	response.JSON(c, http.StatusOK, gin.H{"deleted": true})
}

// members godoc
// @Summary  List members
// @Tags     members
// @Produce  json
// @Security BearerAuth
// @Param    id path string true "Wedding ID"
// @Success  200 {object} response.Envelope{data=object} "{ items: [...] }"
// @Failure  403 {object} response.Envelope{error=response.APIError}
// @Router   /weddings/{id}/members [get]
func (h *Wedding) members(c *gin.Context) {
	members, err := h.svc.Members(c.Request.Context(), middleware.WeddingID(c))
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"items": members})
}

// setRole godoc
// @Summary  Change a member's role (owner only)
// @Tags     members
// @Accept   json
// @Produce  json
// @Security BearerAuth
// @Param    id path string true "Wedding ID"
// @Param    userId path string true "Target user ID"
// @Param    body body dto.SetRoleRequest true "Role"
// @Success  200 {object} response.Envelope{data=object} "{ updated: true }"
// @Failure  400 {object} response.Envelope{error=response.APIError}
// @Failure  403 {object} response.Envelope{error=response.APIError}
// @Failure  404 {object} response.Envelope{error=response.APIError}
// @Router   /weddings/{id}/members/{userId} [patch]
func (h *Wedding) setRole(c *gin.Context) {
	targetID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		apperr.Write(c, apperr.NotFound("member not found"))
		return
	}
	var req dto.SetRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperr.Write(c, apperr.Validation(err.Error()))
		return
	}
	wid := middleware.WeddingID(c)
	if err := h.svc.SetMemberRole(c.Request.Context(), wid, targetID, req.Role); err != nil {
		apperr.Write(c, err)
		return
	}
	caller, _ := middleware.UserID(c)
	h.audit.Record(c.Request.Context(), &caller, "role_changed", "wedding", &wid, c.ClientIP(),
		service.Meta(map[string]any{"target_user": targetID.String(), "role": req.Role}))
	response.JSON(c, http.StatusOK, gin.H{"updated": true})
}

// removeMember godoc
// @Summary  Remove a member or leave
// @Description Owner can remove anyone; any member can remove themselves (leave).
// @Description An owner removal is remembered: invite links can no longer re-admit that user (only a new username invite can).
// @Tags     members
// @Produce  json
// @Security BearerAuth
// @Param    id path string true "Wedding ID"
// @Param    userId path string true "Target user ID (self to leave)"
// @Success  200 {object} response.Envelope{data=object} "{ removed: true }"
// @Failure  403 {object} response.Envelope{error=response.APIError}
// @Failure  404 {object} response.Envelope{error=response.APIError}
// @Router   /weddings/{id}/members/{userId} [delete]
func (h *Wedding) removeMember(c *gin.Context) {
	callerID, _ := middleware.UserID(c)
	targetID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		apperr.Write(c, apperr.NotFound("member not found"))
		return
	}
	// Removing someone else requires owner; removing yourself (leave) is allowed
	// for any member.
	if targetID != callerID && middleware.CurrentRole(c) != models.RoleOwner {
		apperr.Write(c, apperr.Forbidden("only the owner can remove other members"))
		return
	}
	wid := middleware.WeddingID(c)
	if err := h.svc.RemoveMember(c.Request.Context(), wid, targetID, targetID != callerID); err != nil {
		apperr.Write(c, err)
		return
	}
	h.audit.Record(c.Request.Context(), &callerID, "member_removed", "wedding", &wid, c.ClientIP(),
		service.Meta(map[string]any{"target_user": targetID.String()}))
	response.JSON(c, http.StatusOK, gin.H{"removed": true})
}

// createLink godoc
// @Summary  Create an invite link (owner only)
// @Tags     invite-links
// @Accept   json
// @Produce  json
// @Security BearerAuth
// @Param    id path string true "Wedding ID"
// @Param    body body dto.CreateInviteLinkRequest true "Role scope"
// @Success  201 {object} response.Envelope
// @Failure  400 {object} response.Envelope{error=response.APIError}
// @Failure  403 {object} response.Envelope{error=response.APIError}
// @Router   /weddings/{id}/invite-links [post]
func (h *Wedding) createLink(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	var req dto.CreateInviteLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperr.Write(c, apperr.Validation(err.Error()))
		return
	}
	l, err := h.svc.CreateInviteLink(c.Request.Context(), middleware.WeddingID(c), userID, req.Role)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, l)
}

// listLinks godoc
// @Summary  List invite links (owner only)
// @Tags     invite-links
// @Produce  json
// @Security BearerAuth
// @Param    id path string true "Wedding ID"
// @Success  200 {object} response.Envelope{data=object} "{ items: [...] }"
// @Failure  403 {object} response.Envelope{error=response.APIError}
// @Router   /weddings/{id}/invite-links [get]
func (h *Wedding) listLinks(c *gin.Context) {
	links, err := h.svc.ListInviteLinks(c.Request.Context(), middleware.WeddingID(c))
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"items": links})
}

// revokeLink godoc
// @Summary  Revoke an invite link (owner only)
// @Tags     invite-links
// @Produce  json
// @Security BearerAuth
// @Param    id path string true "Wedding ID"
// @Param    linkId path string true "Invite link ID"
// @Success  200 {object} response.Envelope{data=object} "{ revoked: true }"
// @Failure  403 {object} response.Envelope{error=response.APIError}
// @Failure  404 {object} response.Envelope{error=response.APIError}
// @Router   /weddings/{id}/invite-links/{linkId} [delete]
func (h *Wedding) revokeLink(c *gin.Context) {
	linkID, err := uuid.Parse(c.Param("linkId"))
	if err != nil {
		apperr.Write(c, apperr.NotFound("invite link not found"))
		return
	}
	if err := h.svc.RevokeInviteLink(c.Request.Context(), middleware.WeddingID(c), linkID); err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"revoked": true})
}

// previewInvite godoc
// @Summary  Preview an invite
// @Description Returns wedding name and role for a share token before accepting.
// @Tags     invites
// @Produce  json
// @Security BearerAuth
// @Param    token path string true "Invite token"
// @Success  200 {object} response.Envelope
// @Failure  404 {object} response.Envelope{error=response.APIError}
// @Router   /invite/{token} [get]
// transferOwnership godoc
// @Summary  Transfer ownership (owner only)
// @Description Hands the wedding to an existing member; the new user becomes owner and the caller is demoted to editor.
// @Tags     members
// @Accept   json
// @Produce  json
// @Security BearerAuth
// @Param    id path string true "Wedding ID"
// @Param    body body dto.TransferOwnershipRequest true "New owner"
// @Success  200 {object} response.Envelope{data=object} "{ transferred: true }"
// @Failure  400 {object} response.Envelope{error=response.APIError}
// @Failure  403 {object} response.Envelope{error=response.APIError}
// @Failure  404 {object} response.Envelope{error=response.APIError}
// @Router   /weddings/{id}/transfer [post]
func (h *Wedding) transferOwnership(c *gin.Context) {
	callerID, _ := middleware.UserID(c)
	var req dto.TransferOwnershipRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperr.Write(c, apperr.Validation(err.Error()))
		return
	}
	newOwner, err := uuid.Parse(req.UserID)
	if err != nil {
		apperr.Write(c, apperr.Validation("invalid user_id"))
		return
	}
	wid := middleware.WeddingID(c)
	if err := h.svc.TransferOwnership(c.Request.Context(), wid, callerID, newOwner); err != nil {
		apperr.Write(c, err)
		return
	}
	h.audit.Record(c.Request.Context(), &callerID, "ownership_transferred", "wedding", &wid, c.ClientIP(),
		service.Meta(map[string]any{"new_owner": newOwner.String()}))
	response.JSON(c, http.StatusOK, gin.H{"transferred": true})
}

func (h *Wedding) previewInvite(c *gin.Context) {
	p, err := h.svc.PreviewInvite(c.Request.Context(), c.Param("token"))
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, p)
}

// acceptInvite godoc
// @Summary  Accept an invite
// @Description Joins the wedding at the link's role. An existing member keeps their current role (links never change a membership; owners use PATCH /members).
// @Description A former member rejoins at most at the role the owner last set; one the owner removed gets 403 removed_from_wedding.
// @Tags     invites
// @Produce  json
// @Security BearerAuth
// @Param    token path string true "Invite token"
// @Success  200 {object} response.Envelope
// @Failure  403 {object} response.Envelope{error=response.APIError} "removed_from_wedding"
// @Failure  404 {object} response.Envelope{error=response.APIError}
// @Failure  409 {object} response.Envelope{error=response.APIError}
// @Router   /invite/{token}/accept [post]
func (h *Wedding) acceptInvite(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	p, err := h.svc.AcceptInvite(c.Request.Context(), c.Param("token"), userID)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, p)
}
