// Package service holds business logic. Services depend on repositories and
// return *apperr.Error; they never import gin or gorm directly.
package service

import (
	"context"
	"errors"
	"time"

	"zawaj/internal/apperr"
	"zawaj/internal/auth"
	"zawaj/internal/dto"
	"zawaj/internal/models"
	"zawaj/internal/repository"

	"github.com/google/uuid"
)

// dummyHash is a valid bcrypt hash used to equalize login timing when a username
// does not exist, preventing a timing oracle for username enumeration.
const dummyHash = "$2a$10$N9qo8uLOickgx2ZMRZoMye.IjZAgcfl7p92ldGxad68LJZdL17lhWy"

// AuthService implements registration, login, refresh, and recovery.
type AuthService struct {
	users       *repository.UserRepo
	tokens      *auth.Manager
	maxAttempts int
	lockout     time.Duration
}

// NewAuthService builds the service. maxAttempts/lockout govern login brute-force
// protection (0 attempts disables lockout).
func NewAuthService(users *repository.UserRepo, tokens *auth.Manager, maxAttempts int, lockout time.Duration) *AuthService {
	return &AuthService{users: users, tokens: tokens, maxAttempts: maxAttempts, lockout: lockout}
}

// Register creates a new account and returns tokens plus the one-time recovery code.
func (s *AuthService) Register(ctx context.Context, req dto.RegisterRequest) (*dto.AuthResponse, error) {
	exists, err := s.users.ExistsByUsername(ctx, req.Username)
	if err != nil {
		return nil, apperr.Internal("register failed")
	}
	if exists {
		return nil, apperr.Conflict("username already taken")
	}

	passwordHash, err := auth.HashSecret(req.Password)
	if err != nil {
		return nil, apperr.Internal("register failed")
	}
	recoveryCode, err := auth.GenerateRecoveryCode()
	if err != nil {
		return nil, apperr.Internal("register failed")
	}
	recoveryHash, err := auth.HashSecret(recoveryCode)
	if err != nil {
		return nil, apperr.Internal("register failed")
	}

	u := &models.User{
		Username:     req.Username,
		DisplayName:  req.DisplayName,
		PasswordHash: passwordHash,
		RecoveryHash: recoveryHash,
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, apperr.Internal("register failed")
	}

	pair, err := s.tokens.Issue(u.ID)
	if err != nil {
		return nil, apperr.Internal("token issue failed")
	}
	resp := dto.NewUserResponse(u)
	return &dto.AuthResponse{User: &resp, Access: pair.Access, Refresh: pair.Refresh, RecoveryCode: recoveryCode}, nil
}

// Login authenticates with username + password. It enforces an account lockout
// after too many failed attempts and equalizes timing for unknown usernames.
func (s *AuthService) Login(ctx context.Context, req dto.LoginRequest) (*dto.AuthResponse, error) {
	u, err := s.users.ByUsername(ctx, req.Username)
	if errors.Is(err, repository.ErrNotFound) {
		// Spend the same work as a real verify so response time doesn't reveal
		// whether the username exists.
		auth.VerifySecret(dummyHash, req.Password)
		return nil, apperr.Unauthenticated("invalid username or password")
	}
	if err != nil {
		return nil, apperr.Internal("login failed")
	}

	if u.LockedUntil != nil && u.LockedUntil.After(time.Now()) {
		return nil, apperr.Locked("account temporarily locked due to failed attempts")
	}

	if !auth.VerifySecret(u.PasswordHash, req.Password) {
		s.registerFailure(ctx, u)
		return nil, apperr.Unauthenticated("invalid username or password")
	}

	// Success: clear any failure state.
	if u.FailedAttempts != 0 || u.LockedUntil != nil {
		u.FailedAttempts = 0
		u.LockedUntil = nil
		_ = s.users.Update(ctx, u)
	}

	pair, err := s.tokens.Issue(u.ID)
	if err != nil {
		return nil, apperr.Internal("token issue failed")
	}
	resp := dto.NewUserResponse(u)
	return &dto.AuthResponse{User: &resp, Access: pair.Access, Refresh: pair.Refresh}, nil
}

// registerFailure increments the failed-attempt counter and locks the account
// once it reaches maxAttempts.
func (s *AuthService) registerFailure(ctx context.Context, u *models.User) {
	if s.maxAttempts <= 0 {
		return
	}
	u.FailedAttempts++
	if u.FailedAttempts >= s.maxAttempts {
		until := time.Now().Add(s.lockout)
		u.LockedUntil = &until
		u.FailedAttempts = 0
	}
	_ = s.users.Update(ctx, u)
}

// Refresh exchanges a valid refresh token for a new token pair.
func (s *AuthService) Refresh(ctx context.Context, req dto.RefreshRequest) (*dto.AuthResponse, error) {
	userID, err := s.tokens.ParseRefresh(req.Refresh)
	if err != nil {
		return nil, apperr.Unauthenticated("invalid refresh token")
	}
	u, err := s.users.ByID(ctx, userID)
	if err != nil {
		return nil, apperr.Unauthenticated("invalid refresh token")
	}
	pair, err := s.tokens.Issue(u.ID)
	if err != nil {
		return nil, apperr.Internal("token issue failed")
	}
	return &dto.AuthResponse{Access: pair.Access, Refresh: pair.Refresh}, nil
}

// Recover resets the PIN using the one-time recovery code and issues a fresh code.
func (s *AuthService) Recover(ctx context.Context, req dto.RecoverRequest) (*dto.AuthResponse, error) {
	u, err := s.users.ByUsername(ctx, req.Username)
	if errors.Is(err, repository.ErrNotFound) {
		auth.VerifySecret(dummyHash, req.RecoveryCode) // equalize timing
		return nil, apperr.Unauthenticated("invalid username or recovery code")
	}
	if err != nil {
		return nil, apperr.Internal("recover failed")
	}
	if u.LockedUntil != nil && u.LockedUntil.After(time.Now()) {
		return nil, apperr.Locked("account temporarily locked due to failed attempts")
	}
	if !auth.VerifySecret(u.RecoveryHash, req.RecoveryCode) {
		s.registerFailure(ctx, u)
		return nil, apperr.Unauthenticated("invalid username or recovery code")
	}

	passwordHash, err := auth.HashSecret(req.NewPassword)
	if err != nil {
		return nil, apperr.Internal("recover failed")
	}
	newCode, err := auth.GenerateRecoveryCode()
	if err != nil {
		return nil, apperr.Internal("recover failed")
	}
	newCodeHash, err := auth.HashSecret(newCode)
	if err != nil {
		return nil, apperr.Internal("recover failed")
	}
	u.PasswordHash = passwordHash
	u.RecoveryHash = newCodeHash
	u.FailedAttempts = 0
	u.LockedUntil = nil
	if err := s.users.Update(ctx, u); err != nil {
		return nil, apperr.Internal("recover failed")
	}

	pair, err := s.tokens.Issue(u.ID)
	if err != nil {
		return nil, apperr.Internal("token issue failed")
	}
	return &dto.AuthResponse{Access: pair.Access, Refresh: pair.Refresh, RecoveryCode: newCode}, nil
}

// Me returns the current user.
func (s *AuthService) Me(ctx context.Context, userID uuid.UUID) (*dto.UserResponse, error) {
	u, err := s.users.ByID(ctx, userID)
	if err != nil {
		return nil, apperr.NotFound("user not found")
	}
	resp := dto.NewUserResponse(u)
	return &resp, nil
}

// ChangePassword verifies the current password and sets a new one.
func (s *AuthService) ChangePassword(ctx context.Context, userID uuid.UUID, req dto.ChangePasswordRequest) error {
	u, err := s.users.ByID(ctx, userID)
	if err != nil {
		return apperr.NotFound("user not found")
	}
	if !auth.VerifySecret(u.PasswordHash, req.OldPassword) {
		return apperr.Unauthenticated("current password is incorrect")
	}
	hash, err := auth.HashSecret(req.NewPassword)
	if err != nil {
		return apperr.Internal("change password failed")
	}
	u.PasswordHash = hash
	if err := s.users.Update(ctx, u); err != nil {
		return apperr.Internal("change password failed")
	}
	return nil
}

// UpdateSettings applies preference changes and returns the updated user.
func (s *AuthService) UpdateSettings(ctx context.Context, userID uuid.UUID, req dto.UpdateSettingsRequest) (*models.User, error) {
	u, err := s.users.ByID(ctx, userID)
	if err != nil {
		return nil, apperr.NotFound("user not found")
	}
	if req.Email != nil {
		u.Email = req.Email
	}
	if req.DarkMode != nil {
		u.DarkMode = *req.DarkMode
	}
	if req.NotifPush != nil {
		u.NotifPush = *req.NotifPush
	}
	if req.NotifEmail != nil {
		u.NotifEmail = *req.NotifEmail
	}
	if req.NotifRSVP != nil {
		u.NotifRSVP = *req.NotifRSVP
	}
	if err := s.users.Update(ctx, u); err != nil {
		return nil, apperr.Internal("update settings failed")
	}
	return u, nil
}

// UpdateMe edits the current user's profile.
func (s *AuthService) UpdateMe(ctx context.Context, userID uuid.UUID, req dto.UpdateMeRequest) (*dto.UserResponse, error) {
	u, err := s.users.ByID(ctx, userID)
	if err != nil {
		return nil, apperr.NotFound("user not found")
	}
	if req.DisplayName != nil {
		u.DisplayName = *req.DisplayName
	}
	if err := s.users.Update(ctx, u); err != nil {
		return nil, apperr.Internal("update failed")
	}
	resp := dto.NewUserResponse(u)
	return &resp, nil
}
