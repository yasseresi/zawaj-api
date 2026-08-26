// Package dto holds request/response shapes with validation tags. Handlers bind
// these; GORM models are never bound directly from request bodies.
package dto

import "zawaj/internal/models"

// RegisterRequest creates a new account.
type RegisterRequest struct {
	Username    string `json:"username" binding:"required,min=3,max=30,alphanum"`
	DisplayName string `json:"display_name" binding:"required,max=80"`
	PIN         string `json:"pin" binding:"required,min=4,max=6,numeric"`
}

// LoginRequest authenticates with username + PIN.
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	PIN      string `json:"pin" binding:"required"`
}

// RefreshRequest exchanges a refresh token for a new pair.
type RefreshRequest struct {
	Refresh string `json:"refresh" binding:"required"`
}

// RecoverRequest resets the PIN using the one-time recovery code.
type RecoverRequest struct {
	Username     string `json:"username" binding:"required"`
	RecoveryCode string `json:"recovery_code" binding:"required"`
	NewPIN       string `json:"new_pin" binding:"required,min=4,max=6,numeric"`
}

// UpdateMeRequest edits the current user's profile.
type UpdateMeRequest struct {
	DisplayName *string `json:"display_name" binding:"omitempty,max=80"`
}

// UserResponse is the public view of a user.
type UserResponse struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

// NewUserResponse maps a model to its public view.
func NewUserResponse(u *models.User) UserResponse {
	return UserResponse{ID: u.ID.String(), Username: u.Username, DisplayName: u.DisplayName}
}

// AuthResponse is returned on register/login/refresh/recover. RecoveryCode is
// only populated at register and recover (shown once).
type AuthResponse struct {
	User         *UserResponse `json:"user,omitempty"`
	Access       string        `json:"access"`
	Refresh      string        `json:"refresh"`
	RecoveryCode string        `json:"recovery_code,omitempty"`
}
