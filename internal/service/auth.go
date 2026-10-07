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
	"zawaj/internal/validation"

	"github.com/google/uuid"
)

// dummyHash is a valid bcrypt hash used to equalize login timing when a username
// does not exist, preventing a timing oracle for username enumeration.
const dummyHash = "$2a$10$N9qo8uLOickgx2ZMRZoMye.IjZAgcfl7p92ldGxad68LJZdL17lhWy"

// AuthService implements registration, login, refresh, and recovery.
type AuthService struct {
	users       *repository.UserRepo
	refresh     *repository.RefreshTokenRepo
	tokens      *auth.Manager
	maxAttempts int
	lockout     time.Duration
}

// NewAuthService builds the service. maxAttempts/lockout govern login brute-force
// protection (0 attempts disables lockout).
func NewAuthService(users *repository.UserRepo, refresh *repository.RefreshTokenRepo, tokens *auth.Manager, maxAttempts int, lockout time.Duration) *AuthService {
	return &AuthService{users: users, refresh: refresh, tokens: tokens, maxAttempts: maxAttempts, lockout: lockout}
}

// userWriteErr maps a failed user write: the user vanished (deleted
// concurrently) is a 404, anything else a 500 with msg.
func userWriteErr(err error, msg string) *apperr.Error {
	if errors.Is(err, repository.ErrNotFound) {
		return apperr.NotFound("user not found")
	}
	return apperr.Internal(msg)
}

// issuePair issues an access+refresh pair and records the refresh token's jti so
// it can be rotated/revoked later.
func (s *AuthService) issuePair(ctx context.Context, userID uuid.UUID) (auth.TokenPair, error) {
	pair, err := s.tokens.Issue(userID)
	if err != nil {
		return auth.TokenPair{}, apperr.Internal("token issue failed")
	}
	if err := s.refresh.Store(ctx, pair.RefreshJTI, userID, pair.RefreshExpiry); err != nil {
		return auth.TokenPair{}, apperr.Internal("token issue failed")
	}
	return pair, nil
}

// Register creates a new account and returns tokens plus the one-time recovery code.
func (s *AuthService) Register(ctx context.Context, req dto.RegisterRequest) (*dto.AuthResponse, error) {
	if err := validation.Username(req.Username); err != nil {
		return nil, apperr.Validation(err.Error())
	}
	if err := validation.Password(req.Password); err != nil {
		return nil, apperr.Validation(err.Error())
	}
	req.Username = validation.NormalizeUsername(req.Username)
	exists, err := s.users.ExistsByUsername(ctx, req.Username)
	if err != nil {
		return nil, apperr.Internal("register failed")
	}
	if exists {
		return nil, apperr.UsernameTaken("username already taken")
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
		if repository.IsUniqueViolation(err) {
			return nil, apperr.UsernameTaken("username already taken")
		}
		return nil, apperr.Internal("register failed")
	}

	pair, err := s.issuePair(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	resp := dto.NewUserResponse(u)
	return &dto.AuthResponse{User: &resp, Access: pair.Access, Refresh: pair.Refresh, RecoveryCode: recoveryCode}, nil
}

// Login authenticates with username + password. It enforces an account lockout
// after too many failed attempts and equalizes timing for unknown usernames.
func (s *AuthService) Login(ctx context.Context, req dto.LoginRequest) (*dto.AuthResponse, error) {
	if err := validation.Username(req.Username); err != nil {
		return nil, apperr.Validation(err.Error())
	}
	if req.Password == "" {
		return nil, apperr.Validation("password is required")
	}
	req.Username = validation.NormalizeUsername(req.Username)
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

	pair, err := s.issuePair(ctx, u.ID)
	if err != nil {
		return nil, err
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

// Refresh rotates a valid refresh token: it verifies the token, checks the
// server-side record, revokes it, and issues a new pair. If a token that was
// already revoked is presented (replay of a rotated/leaked token), it treats
// this as a breach and revokes every refresh token of the user.
func (s *AuthService) Refresh(ctx context.Context, req dto.RefreshRequest) (*dto.AuthResponse, error) {
	userID, jti, err := s.tokens.ParseRefresh(req.Refresh)
	if err != nil {
		return nil, apperr.Unauthenticated("invalid refresh token")
	}

	row, err := s.refresh.ByID(ctx, jti)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, apperr.Unauthenticated("invalid refresh token")
	}
	if err != nil {
		return nil, apperr.Internal("refresh failed")
	}
	if row.UserID != userID {
		return nil, apperr.Unauthenticated("invalid refresh token")
	}
	if row.RevokedAt != nil {
		// Reuse of a revoked token → assume compromise; kill the whole family.
		_ = s.refresh.RevokeAllForUser(ctx, userID)
		return nil, apperr.Unauthenticated("refresh token has been revoked")
	}
	if row.ExpiresAt.Before(time.Now()) {
		return nil, apperr.Unauthenticated("refresh token expired")
	}

	u, err := s.users.ByID(ctx, userID)
	if err != nil {
		return nil, apperr.Unauthenticated("invalid refresh token")
	}

	// Rotate: revoke the presented token, issue a new pair.
	if err := s.refresh.Revoke(ctx, jti); err != nil {
		return nil, apperr.Internal("refresh failed")
	}
	pair, err := s.issuePair(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	return &dto.AuthResponse{Access: pair.Access, Refresh: pair.Refresh}, nil
}

// Logout revokes the presented refresh token so it can no longer be rotated.
// Idempotent and safe to call with an already-invalid token.
func (s *AuthService) Logout(ctx context.Context, req dto.RefreshRequest) error {
	_, jti, err := s.tokens.ParseRefresh(req.Refresh)
	if err != nil {
		// Nothing to revoke for an unparseable token; report success (idempotent).
		return nil
	}
	if err := s.refresh.Revoke(ctx, jti); err != nil {
		return apperr.Internal("logout failed")
	}
	return nil
}

// Recover resets the PIN using the one-time recovery code and issues a fresh code.
func (s *AuthService) Recover(ctx context.Context, req dto.RecoverRequest) (*dto.AuthResponse, error) {
	if err := validation.Username(req.Username); err != nil {
		return nil, apperr.Validation(err.Error())
	}
	if err := validation.Password(req.NewPassword); err != nil {
		return nil, apperr.Validation(err.Error())
	}
	req.Username = validation.NormalizeUsername(req.Username)
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
		return nil, userWriteErr(err, "recover failed")
	}
	// Password was reset — invalidate any existing sessions.
	_ = s.refresh.RevokeAllForUser(ctx, u.ID)

	pair, err := s.issuePair(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	return &dto.AuthResponse{Access: pair.Access, Refresh: pair.Refresh, RecoveryCode: newCode}, nil
}

// DeleteAccount permanently deletes the user and every wedding they own, after
// re-verifying the current password. Wrong passwords count toward the login
// lockout so this endpoint can't be used to brute-force the password.
func (s *AuthService) DeleteAccount(ctx context.Context, userID uuid.UUID, password string) error {
	if password == "" {
		return apperr.Validation("password is required")
	}
	u, err := s.users.ByID(ctx, userID)
	if err != nil {
		return apperr.NotFound("user not found")
	}
	if u.LockedUntil != nil && u.LockedUntil.After(time.Now()) {
		return apperr.Locked("account temporarily locked due to failed attempts")
	}
	if !auth.VerifySecret(u.PasswordHash, password) {
		s.registerFailure(ctx, u)
		return apperr.Unauthenticated("password is incorrect")
	}
	if err := s.users.DeleteWithOwnedData(ctx, userID); err != nil {
		return apperr.Internal("delete account failed")
	}
	return nil
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
	if req.OldPassword == "" {
		return apperr.Validation("current password is required")
	}
	if err := validation.Password(req.NewPassword); err != nil {
		return apperr.Validation(err.Error())
	}
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
		return userWriteErr(err, "change password failed")
	}
	// Invalidate existing sessions after a password change.
	_ = s.refresh.RevokeAllForUser(ctx, userID)
	return nil
}

// UpdateSettings applies preference changes and returns the updated user.
func (s *AuthService) UpdateSettings(ctx context.Context, userID uuid.UUID, req dto.UpdateSettingsRequest) (*dto.UserResponse, error) {
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
		return nil, userWriteErr(err, "update settings failed")
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
	if req.Phone != nil {
		if *req.Phone == "" {
			u.Phone = nil
		} else {
			phone, err := validation.NormalizePhone(*req.Phone)
			if err != nil {
				return nil, apperr.Validation(err.Error())
			}
			u.Phone = &phone
		}
	}
	if err := s.users.Update(ctx, u); err != nil {
		return nil, userWriteErr(err, "update failed")
	}
	resp := dto.NewUserResponse(u)
	return &resp, nil
}
