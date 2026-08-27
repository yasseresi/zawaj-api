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
	repo   *repository.WeddingRepo // used by RequireRole middleware
	tokens *auth.Manager
}

// NewWedding builds the module.
func NewWedding(svc *service.WeddingService, repo *repository.WeddingRepo, tokens *auth.Manager) *Wedding {
	return &Wedding{svc: svc, repo: repo, tokens: tokens}
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

	w.POST("/invite-links", middleware.RequireRole(h.repo, owner), h.createLink)
	w.GET("/invite-links", middleware.RequireRole(h.repo, owner), h.listLinks)
	w.DELETE("/invite-links/:linkId", middleware.RequireRole(h.repo, owner), h.revokeLink)
}

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

func (h *Wedding) list(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	items, err := h.svc.List(c.Request.Context(), userID)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"items": items})
}

func (h *Wedding) get(c *gin.Context) {
	w, err := h.svc.Get(c.Request.Context(), middleware.WeddingID(c))
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"wedding": w, "my_role": middleware.CurrentRole(c)})
}

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

func (h *Wedding) remove(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), middleware.WeddingID(c)); err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"deleted": true})
}

func (h *Wedding) members(c *gin.Context) {
	members, err := h.svc.Members(c.Request.Context(), middleware.WeddingID(c))
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"items": members})
}

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
	if err := h.svc.SetMemberRole(c.Request.Context(), middleware.WeddingID(c), targetID, req.Role); err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"updated": true})
}

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
	if err := h.svc.RemoveMember(c.Request.Context(), middleware.WeddingID(c), targetID); err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"removed": true})
}

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

func (h *Wedding) listLinks(c *gin.Context) {
	links, err := h.svc.ListInviteLinks(c.Request.Context(), middleware.WeddingID(c))
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"items": links})
}

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

func (h *Wedding) previewInvite(c *gin.Context) {
	p, err := h.svc.PreviewInvite(c.Request.Context(), c.Param("token"))
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, p)
}

func (h *Wedding) acceptInvite(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	p, err := h.svc.AcceptInvite(c.Request.Context(), c.Param("token"), userID)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, p)
}
