package handler

import (
	"net/http"

	"zawaj/internal/apperr"
	"zawaj/internal/auth"
	"zawaj/internal/dto"
	"zawaj/internal/middleware"
	"zawaj/internal/service"
	"zawaj/pkg/response"

	"github.com/gin-gonic/gin"
)

// Auth is the auth module: it owns /auth/* and /me routes and implements
// router.Module so main can mount it without editing router.go.
type Auth struct {
	svc    *service.AuthService
	tokens *auth.Manager
}

// NewAuth builds the auth handler/module.
func NewAuth(svc *service.AuthService, tokens *auth.Manager) *Auth {
	return &Auth{svc: svc, tokens: tokens}
}

// Register mounts the auth routes under the given group.
func (h *Auth) Register(rg *gin.RouterGroup) {
	rg.POST("/auth/register", h.register)
	rg.POST("/auth/login", h.login)
	rg.POST("/auth/refresh", h.refresh)
	rg.POST("/auth/recover", h.recover)

	me := rg.Group("/me", middleware.RequireAuth(h.tokens))
	me.GET("", h.me)
	me.PATCH("", h.updateMe)
}

func (h *Auth) register(c *gin.Context) {
	var req dto.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperr.Write(c, apperr.Validation(err.Error()))
		return
	}
	resp, err := h.svc.Register(c.Request.Context(), req)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, resp)
}

func (h *Auth) login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperr.Write(c, apperr.Validation(err.Error()))
		return
	}
	resp, err := h.svc.Login(c.Request.Context(), req)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, resp)
}

func (h *Auth) refresh(c *gin.Context) {
	var req dto.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperr.Write(c, apperr.Validation(err.Error()))
		return
	}
	resp, err := h.svc.Refresh(c.Request.Context(), req)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, resp)
}

func (h *Auth) recover(c *gin.Context) {
	var req dto.RecoverRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperr.Write(c, apperr.Validation(err.Error()))
		return
	}
	resp, err := h.svc.Recover(c.Request.Context(), req)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, resp)
}

func (h *Auth) me(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	resp, err := h.svc.Me(c.Request.Context(), userID)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, resp)
}

func (h *Auth) updateMe(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	var req dto.UpdateMeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperr.Write(c, apperr.Validation(err.Error()))
		return
	}
	resp, err := h.svc.UpdateMe(c.Request.Context(), userID, req)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, resp)
}
