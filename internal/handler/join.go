package handler

import (
	"errors"
	"io"
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

// JoinRequest is the join-approval module: accepting an invite link creates a
// request that the wedding owner approves or declines.
type JoinRequest struct {
	svc    *service.JoinService
	audit  *service.AuditService
	repo   *repository.WeddingRepo // used by RequireRole middleware
	tokens *auth.Manager
}

// NewJoinRequest builds the module.
func NewJoinRequest(svc *service.JoinService, audit *service.AuditService, repo *repository.WeddingRepo, tokens *auth.Manager) *JoinRequest {
	return &JoinRequest{svc: svc, audit: audit, repo: repo, tokens: tokens}
}

// Register mounts the routes. Reviewing requests is owner-only (ADR-004).
func (h *JoinRequest) Register(rg *gin.RouterGroup) {
	a := rg.Group("", middleware.RequireAuth(h.tokens))
	a.POST("/invite/:token/accept", h.accept)

	owner := middleware.RequireRole(h.repo, models.RoleOwner)
	w := a.Group("/weddings/:id/join-requests", owner)
	w.GET("", h.list)
	w.POST("/:requestId/approve", h.approve)
	w.POST("/:requestId/decline", h.decline)
}

// accept godoc
// @Summary  Accept an invite link (request to join)
// @Description Existing members get 200 with their current role (status "member").
// @Description Otherwise a join request is created (or the pending one returned) and the owner is notified: 202, status "pending".
// @Description The role is the link's, capped by the owner's last decision; a user the owner removed gets 403 removed_from_wedding.
// @Description A request declined less than 24h ago gets 409 join_request_declined; a wedding with 50 pending requests gets 429 join_queue_full.
// @Description The owner is pushed only when the queue was empty (later requests are stored in-app).
// @Tags     invites
// @Produce  json
// @Security BearerAuth
// @Param    token path string true "Invite token"
// @Success  200 {object} response.Envelope{data=service.JoinResult} "already a member"
// @Success  202 {object} response.Envelope{data=service.JoinResult} "join request pending"
// @Failure  403 {object} response.Envelope{error=response.APIError} "removed_from_wedding"
// @Failure  404 {object} response.Envelope{error=response.APIError}
// @Failure  401 {object} response.Envelope{error=response.APIError}
// @Failure  409 {object} response.Envelope{error=response.APIError} "join_request_declined"
// @Failure  429 {object} response.Envelope{error=response.APIError} "join_queue_full"
// @Router   /invite/{token}/accept [post]
func (h *JoinRequest) accept(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	res, err := h.svc.RequestFromLink(c.Request.Context(), c.Param("token"), userID)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	status := http.StatusAccepted
	if res.Status == service.JoinStatusMember {
		status = http.StatusOK
	}
	response.JSON(c, status, res)
}

// list godoc
// @Summary  List pending join requests (owner only)
// @Description Oldest first, at most 100.
// @Tags     members
// @Produce  json
// @Security BearerAuth
// @Param    id path string true "Wedding ID"
// @Success  200 {object} response.Envelope{data=object} "{ items: JoinRequestView[] }"
// @Failure  403 {object} response.Envelope{error=response.APIError}
// @Failure  404 {object} response.Envelope{error=response.APIError}
// @Router   /weddings/{id}/join-requests [get]
func (h *JoinRequest) list(c *gin.Context) {
	items, err := h.svc.ListPending(c.Request.Context(), middleware.WeddingID(c))
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"items": items})
}

// approve godoc
// @Summary  Approve a join request (owner only)
// @Description Creates the membership. The optional role may lower the requested role, never raise it.
// @Description A user the owner removed meanwhile gets 403 removed_from_wedding (the request is closed); one who joined another way is closed quietly.
// @Tags     members
// @Accept   json
// @Produce  json
// @Security BearerAuth
// @Param    id path string true "Wedding ID"
// @Param    requestId path string true "Join request ID"
// @Param    body body dto.ApproveJoinRequest false "Optional lower role"
// @Success  200 {object} response.Envelope{data=models.JoinRequest}
// @Failure  400 {object} response.Envelope{error=response.APIError}
// @Failure  403 {object} response.Envelope{error=response.APIError} "not the owner, or removed_from_wedding"
// @Failure  404 {object} response.Envelope{error=response.APIError} "not pending in this wedding"
// @Router   /weddings/{id}/join-requests/{requestId}/approve [post]
func (h *JoinRequest) approve(c *gin.Context) {
	rid, ok := requestID(c)
	if !ok {
		return
	}
	var req dto.ApproveJoinRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		apperr.Write(c, apperr.Validation(err.Error()))
		return
	}
	owner, _ := middleware.UserID(c)
	wid := middleware.WeddingID(c)
	decided, err := h.svc.Approve(c.Request.Context(), wid, rid, owner, req.Role)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	h.audit.Record(c.Request.Context(), &owner, "join_request_approved", "wedding", &wid, c.ClientIP(),
		service.Meta(map[string]any{"request_id": rid.String(), "user_id": decided.UserID.String(), "role": decided.Role}))
	response.JSON(c, http.StatusOK, decided)
}

// decline godoc
// @Summary  Decline a join request (owner only)
// @Description The requester may ask again after 24 hours.
// @Tags     members
// @Produce  json
// @Security BearerAuth
// @Param    id path string true "Wedding ID"
// @Param    requestId path string true "Join request ID"
// @Success  200 {object} response.Envelope{data=models.JoinRequest}
// @Failure  403 {object} response.Envelope{error=response.APIError}
// @Failure  404 {object} response.Envelope{error=response.APIError} "not pending in this wedding"
// @Router   /weddings/{id}/join-requests/{requestId}/decline [post]
func (h *JoinRequest) decline(c *gin.Context) {
	rid, ok := requestID(c)
	if !ok {
		return
	}
	owner, _ := middleware.UserID(c)
	wid := middleware.WeddingID(c)
	decided, err := h.svc.Decline(c.Request.Context(), wid, rid, owner)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	h.audit.Record(c.Request.Context(), &owner, "join_request_declined", "wedding", &wid, c.ClientIP(),
		service.Meta(map[string]any{"request_id": rid.String(), "user_id": decided.UserID.String()}))
	response.JSON(c, http.StatusOK, decided)
}

// requestID parses :requestId, writing a 404 for a malformed one.
func requestID(c *gin.Context) (uuid.UUID, bool) {
	rid, err := uuid.Parse(c.Param("requestId"))
	if err != nil {
		apperr.Write(c, apperr.NotFound("join request not found"))
		return uuid.Nil, false
	}
	return rid, true
}
