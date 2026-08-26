// Package service holds business logic. Services depend on repositories and
// return *apperr.Error; they never import gin or gorm directly.
package service

import (
	"context"
	"errors"

	"zawaj/internal/apperr"
	"zawaj/internal/auth"
	"zawaj/internal/dto"
	"zawaj/internal/models"
	"zawaj/internal/repository"

	"github.com/google/uuid"
)

// AuthService implements registration, login, refresh, and recovery.
type AuthService struct {
	users  *repository.UserRepo
	tokens *auth.Manager
}

// NewAuthService builds the service.
func NewAuthService(users *repository.UserRepo, tokens *auth.Manager) *AuthService {
	return &AuthService{users: users, tokens: tokens}
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

	pinHash, err := auth.HashSecret(req.PIN)
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
		PINHash:      pinHash,
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

// Login authenticates with username + PIN.
func (s *AuthService) Login(ctx context.Context, req dto.LoginRequest) (*dto.AuthResponse, error) {
	u, err := s.users.ByUsername(ctx, req.Username)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, apperr.Unauthenticated("invalid username or PIN")
	}
	if err != nil {
		return nil, apperr.Internal("login failed")
	}
	if !auth.VerifySecret(u.PINHash, req.PIN) {
		return nil, apperr.Unauthenticated("invalid username or PIN")
	}
	pair, err := s.tokens.Issue(u.ID)
	if err != nil {
		return nil, apperr.Internal("token issue failed")
	}
	resp := dto.NewUserResponse(u)
	return &dto.AuthResponse{User: &resp, Access: pair.Access, Refresh: pair.Refresh}, nil
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
		return nil, apperr.Unauthenticated("invalid username or recovery code")
	}
	if err != nil {
		return nil, apperr.Internal("recover failed")
	}
	if !auth.VerifySecret(u.RecoveryHash, req.RecoveryCode) {
		return nil, apperr.Unauthenticated("invalid username or recovery code")
	}

	pinHash, err := auth.HashSecret(req.NewPIN)
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
	u.PINHash = pinHash
	u.RecoveryHash = newCodeHash
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
