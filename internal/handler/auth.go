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
	me.PATCH("/password", h.changePassword)
	me.PATCH("/settings", h.updateSettings)
	me.DELETE("", h.deleteMe)
}

// register godoc
// @Summary  Register a new account
// @Description Creates a user and returns tokens plus a one-time recovery code (shown once).
// @Tags     auth
// @Accept   json
// @Produce  json
// @Param    body body dto.RegisterRequest true "Registration"
// @Success  201 {object} response.Envelope{data=dto.AuthResponse}
// @Failure  400 {object} response.Envelope{error=response.APIError}
// @Failure  409 {object} response.Envelope{error=response.APIError}
// @Router   /auth/register [post]
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

// login godoc
// @Summary  Log in
// @Tags     auth
// @Accept   json
// @Produce  json
// @Param    body body dto.LoginRequest true "Credentials"
// @Success  200 {object} response.Envelope{data=dto.AuthResponse}
// @Failure  400 {object} response.Envelope{error=response.APIError}
// @Failure  401 {object} response.Envelope{error=response.APIError}
// @Failure  423 {object} response.Envelope{error=response.APIError} "Account locked (too many attempts)"
// @Router   /auth/login [post]
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

// refresh godoc
// @Summary  Refresh token pair
// @Tags     auth
// @Accept   json
// @Produce  json
// @Param    body body dto.RefreshRequest true "Refresh token"
// @Success  200 {object} response.Envelope{data=dto.AuthResponse}
// @Failure  400 {object} response.Envelope{error=response.APIError}
// @Failure  401 {object} response.Envelope{error=response.APIError}
// @Router   /auth/refresh [post]
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

// recover godoc
// @Summary  Recover account with one-time code
// @Description Resets the password using the recovery code issued at registration; returns a fresh code.
// @Tags     auth
// @Accept   json
// @Produce  json
// @Param    body body dto.RecoverRequest true "Recovery"
// @Success  200 {object} response.Envelope{data=dto.AuthResponse}
// @Failure  400 {object} response.Envelope{error=response.APIError}
// @Failure  401 {object} response.Envelope{error=response.APIError}
// @Failure  423 {object} response.Envelope{error=response.APIError} "Locked (too many attempts)"
// @Router   /auth/recover [post]
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

// me godoc
// @Summary  Get current user
// @Tags     me
// @Produce  json
// @Security BearerAuth
// @Success  200 {object} response.Envelope{data=dto.UserResponse}
// @Failure  401 {object} response.Envelope{error=response.APIError}
// @Router   /me [get]
func (h *Auth) me(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	resp, err := h.svc.Me(c.Request.Context(), userID)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, resp)
}

// updateMe godoc
// @Summary  Update current user profile
// @Tags     me
// @Accept   json
// @Produce  json
// @Security BearerAuth
// @Param    body body dto.UpdateMeRequest true "Profile fields"
// @Success  200 {object} response.Envelope{data=dto.UserResponse}
// @Failure  400 {object} response.Envelope{error=response.APIError}
// @Failure  401 {object} response.Envelope{error=response.APIError}
// @Router   /me [patch]
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

// changePassword godoc
// @Summary  Change password
// @Tags     me
// @Accept   json
// @Produce  json
// @Security BearerAuth
// @Param    body body dto.ChangePasswordRequest true "Old and new password"
// @Success  200 {object} response.Envelope{data=object} "{ changed: true }"
// @Failure  400 {object} response.Envelope{error=response.APIError}
// @Failure  401 {object} response.Envelope{error=response.APIError}
// @Router   /me/password [patch]
func (h *Auth) changePassword(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	var req dto.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperr.Write(c, apperr.Validation(err.Error()))
		return
	}
	if err := h.svc.ChangePassword(c.Request.Context(), userID, req); err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"changed": true})
}

// deleteMe godoc
// @Summary  Delete account
// @Description Permanently deletes the account and cascades all owned weddings and their data.
// @Tags     me
// @Produce  json
// @Security BearerAuth
// @Success  200 {object} response.Envelope{data=object} "{ deleted: true }"
// @Failure  401 {object} response.Envelope{error=response.APIError}
// @Router   /me [delete]
func (h *Auth) deleteMe(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	if err := h.svc.DeleteAccount(c.Request.Context(), userID); err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"deleted": true})
}

// updateSettings godoc
// @Summary  Update settings/preferences
// @Tags     me
// @Accept   json
// @Produce  json
// @Security BearerAuth
// @Param    body body dto.UpdateSettingsRequest true "Settings"
// @Success  200 {object} response.Envelope{data=object}
// @Failure  400 {object} response.Envelope{error=response.APIError}
// @Failure  401 {object} response.Envelope{error=response.APIError}
// @Router   /me/settings [patch]
func (h *Auth) updateSettings(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	var req dto.UpdateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperr.Write(c, apperr.Validation(err.Error()))
		return
	}
	user, err := h.svc.UpdateSettings(c.Request.Context(), userID, req)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, user)
}
