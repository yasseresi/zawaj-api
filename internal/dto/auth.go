// Package dto holds request/response shapes with validation tags. Handlers bind
// these; GORM models are never bound directly from request bodies.
package dto

import "zawaj/internal/models"

// RegisterRequest creates a new account. Password is length-only (min 8); bcrypt
// ignores bytes past 72, so we cap there.
type RegisterRequest struct {
	Username    string `json:"username" binding:"required,min=3,max=120,excludesall= "` // may be an email
	DisplayName string `json:"display_name" binding:"required,max=80"`
	Password    string `json:"password" binding:"required,min=8,max=72"`
}

// LoginRequest authenticates with username + password.
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// RefreshRequest exchanges a refresh token for a new pair.
type RefreshRequest struct {
	Refresh string `json:"refresh" binding:"required"`
}

// RecoverRequest resets the password using the one-time recovery code.
type RecoverRequest struct {
	Username     string `json:"username" binding:"required"`
	RecoveryCode string `json:"recovery_code" binding:"required"`
	NewPassword  string `json:"new_password" binding:"required,min=8,max=72"`
}

// UpdateMeRequest edits the current user's profile. Phone is normalized and
// validated in the service (Algerian national format); "" clears it.
type UpdateMeRequest struct {
	DisplayName *string `json:"display_name" binding:"omitempty,max=80"`
	Phone       *string `json:"phone" binding:"omitempty,max=20"`
}

// ChangePasswordRequest changes the password (settings screen).
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=8,max=72"`
}

// UpdateSettingsRequest edits preferences (settings screen). All fields optional.
type UpdateSettingsRequest struct {
	Email      *string `json:"email" binding:"omitempty,email,max=120"`
	DarkMode   *bool   `json:"dark_mode"`
	NotifPush  *bool   `json:"notif_push"`
	NotifEmail *bool   `json:"notif_email"`
	NotifRSVP  *bool   `json:"notif_rsvp"`
}

// UserResponse is the public view of a user, including the preferences the
// settings screen reads back.
type UserResponse struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"display_name"`
	Phone       *string `json:"phone,omitempty"`
	DarkMode    bool    `json:"dark_mode"`
	NotifPush   bool    `json:"notif_push"`
	NotifRSVP   bool    `json:"notif_rsvp"`
}

// NewUserResponse maps a model to its public view.
func NewUserResponse(u *models.User) UserResponse {
	return UserResponse{
		ID: u.ID.String(), Username: u.Username, DisplayName: u.DisplayName,
		Phone: u.Phone, DarkMode: u.DarkMode, NotifPush: u.NotifPush, NotifRSVP: u.NotifRSVP,
	}
}

// AuthResponse is returned on register/login/refresh/recover. RecoveryCode is
// only populated at register and recover (shown once).
type AuthResponse struct {
	User         *UserResponse `json:"user,omitempty"`
	Access       string        `json:"access"`
	Refresh      string        `json:"refresh"`
	RecoveryCode string        `json:"recovery_code,omitempty"`
}
