package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Token types carried in the "typ" claim to stop a refresh token being used as
// an access token and vice versa.
const (
	typeAccess  = "access"
	typeRefresh = "refresh"
)

// ErrInvalidToken is returned for any malformed, expired, or wrong-type token.
var ErrInvalidToken = errors.New("invalid token")

// TokenPair is an access + refresh token issued together. RefreshJTI is the
// refresh token's unique id (embedded as the "jti" claim); the auth service
// stores it so the token can be rotated and revoked server-side. RefreshExpiry
// is when the refresh token expires.
type TokenPair struct {
	Access        string    `json:"access"`
	Refresh       string    `json:"refresh"`
	RefreshJTI    uuid.UUID `json:"-"`
	RefreshExpiry time.Time `json:"-"`
}

// Claims is the JWT payload.
type Claims struct {
	Type string `json:"typ"`
	jwt.RegisteredClaims
}

// Manager issues and verifies JWTs. Access and refresh use separate secrets.
type Manager struct {
	accessSecret  []byte
	refreshSecret []byte
	accessTTL     time.Duration
	refreshTTL    time.Duration
}

// NewManager builds a token manager.
func NewManager(accessSecret, refreshSecret string, accessTTL, refreshTTL time.Duration) *Manager {
	return &Manager{
		accessSecret:  []byte(accessSecret),
		refreshSecret: []byte(refreshSecret),
		accessTTL:     accessTTL,
		refreshTTL:    refreshTTL,
	}
}

// Issue returns a fresh access+refresh pair for the given user. The refresh token
// carries a unique jti (also returned as TokenPair.RefreshJTI) so the caller can
// persist it for rotation/revocation.
func (m *Manager) Issue(userID uuid.UUID) (TokenPair, error) {
	access, err := m.sign(userID, typeAccess, uuid.Nil, m.accessSecret, m.accessTTL)
	if err != nil {
		return TokenPair{}, err
	}
	jti := uuid.New()
	exp := time.Now().Add(m.refreshTTL)
	refresh, err := m.sign(userID, typeRefresh, jti, m.refreshSecret, m.refreshTTL)
	if err != nil {
		return TokenPair{}, err
	}
	return TokenPair{Access: access, Refresh: refresh, RefreshJTI: jti, RefreshExpiry: exp}, nil
}

// ParseAccess validates an access token and returns the user id.
func (m *Manager) ParseAccess(token string) (uuid.UUID, error) {
	id, _, err := m.parse(token, typeAccess, m.accessSecret)
	return id, err
}

// ParseRefresh validates a refresh token and returns the user id and its jti.
func (m *Manager) ParseRefresh(token string) (uuid.UUID, uuid.UUID, error) {
	return m.parse(token, typeRefresh, m.refreshSecret)
}

func (m *Manager) sign(userID uuid.UUID, typ string, jti uuid.UUID, secret []byte, ttl time.Duration) (string, error) {
	now := time.Now()
	rc := jwt.RegisteredClaims{
		Subject:   userID.String(),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
	}
	if jti != uuid.Nil {
		rc.ID = jti.String()
	}
	claims := Claims{Type: typ, RegisteredClaims: rc}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
}

// parse validates the token and returns (userID, jti). jti is uuid.Nil when the
// token carries no id (e.g. access tokens).
func (m *Manager) parse(token, wantType string, secret []byte) (uuid.UUID, uuid.UUID, error) {
	var claims Claims
	_, err := jwt.ParseWithClaims(token, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return secret, nil
	})
	if err != nil || claims.Type != wantType {
		return uuid.Nil, uuid.Nil, ErrInvalidToken
	}
	id, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, uuid.Nil, ErrInvalidToken
	}
	var jti uuid.UUID
	if claims.ID != "" {
		if jti, err = uuid.Parse(claims.ID); err != nil {
			return uuid.Nil, uuid.Nil, ErrInvalidToken
		}
	}
	return id, jti, nil
}
