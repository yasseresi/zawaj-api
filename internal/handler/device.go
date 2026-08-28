package handler

import (
	"errors"
	"net/http"

	"zawaj/internal/apperr"
	"zawaj/internal/auth"
	"zawaj/internal/dto"
	"zawaj/internal/middleware"
	"zawaj/internal/models"
	"zawaj/internal/repository"
	"zawaj/pkg/response"

	"github.com/gin-gonic/gin"
)

// Device is the push-registration module, mounted under /me/devices. Routes are
// per-user (the owner is always the authenticated caller).
type Device struct {
	devices *repository.DeviceTokenRepo
	tokens  *auth.Manager
}

// NewDevice builds the module.
func NewDevice(devices *repository.DeviceTokenRepo, tokens *auth.Manager) *Device {
	return &Device{devices: devices, tokens: tokens}
}

// Register mounts device routes. All require an authenticated user.
func (h *Device) Register(rg *gin.RouterGroup) {
	g := rg.Group("/me/devices", middleware.RequireAuth(h.tokens))
	g.POST("", h.register)
	g.DELETE("", h.unregister)
}

// register godoc
// @Summary  Register a device for push
// @Description Upserts an FCM token for the current user. Call after obtaining/refreshing the FCM token.
// @Tags     devices
// @Accept   json
// @Produce  json
// @Security BearerAuth
// @Param    body body dto.RegisterDeviceRequest true "Token + platform"
// @Success  201 {object} response.Envelope{data=object} "{ registered: true }"
// @Failure  400 {object} response.Envelope{error=response.APIError}
// @Failure  401 {object} response.Envelope{error=response.APIError}
// @Router   /me/devices [post]
func (h *Device) register(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	var req dto.RegisterDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperr.Write(c, apperr.Validation(err.Error()))
		return
	}
	if err := h.devices.Upsert(c.Request.Context(), userID, req.Token, models.DevicePlatform(req.Platform)); err != nil {
		apperr.Write(c, apperr.Internal("register device failed"))
		return
	}
	response.JSON(c, http.StatusCreated, gin.H{"registered": true})
}

// unregister godoc
// @Summary  Unregister a device
// @Description Removes an FCM token (e.g. on logout). 404 if the token is not registered to this user.
// @Tags     devices
// @Accept   json
// @Produce  json
// @Security BearerAuth
// @Param    body body dto.UnregisterDeviceRequest true "Token"
// @Success  200 {object} response.Envelope{data=object} "{ unregistered: true }"
// @Failure  400 {object} response.Envelope{error=response.APIError}
// @Failure  401 {object} response.Envelope{error=response.APIError}
// @Failure  404 {object} response.Envelope{error=response.APIError}
// @Router   /me/devices [delete]
func (h *Device) unregister(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	var req dto.UnregisterDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperr.Write(c, apperr.Validation(err.Error()))
		return
	}
	if err := h.devices.Delete(c.Request.Context(), userID, req.Token); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			apperr.Write(c, apperr.NotFound("device token not found"))
			return
		}
		apperr.Write(c, apperr.Internal("unregister device failed"))
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"unregistered": true})
}
