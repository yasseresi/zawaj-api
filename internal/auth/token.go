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

// TokenPair is an access + refresh token issued together.
type TokenPair struct {
	Access  string `json:"access"`
	Refresh string `json:"refresh"`
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

// Issue returns a fresh access+refresh pair for the given user.
func (m *Manager) Issue(userID uuid.UUID) (TokenPair, error) {
	access, err := m.sign(userID, typeAccess, m.accessSecret, m.accessTTL)
	if err != nil {
		return TokenPair{}, err
	}
	refresh, err := m.sign(userID, typeRefresh, m.refreshSecret, m.refreshTTL)
	if err != nil {
		return TokenPair{}, err
	}
	return TokenPair{Access: access, Refresh: refresh}, nil
}

// ParseAccess validates an access token and returns the user id.
func (m *Manager) ParseAccess(token string) (uuid.UUID, error) {
	return m.parse(token, typeAccess, m.accessSecret)
}

// ParseRefresh validates a refresh token and returns the user id.
func (m *Manager) ParseRefresh(token string) (uuid.UUID, error) {
	return m.parse(token, typeRefresh, m.refreshSecret)
}

func (m *Manager) sign(userID uuid.UUID, typ string, secret []byte, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		Type: typ,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
}

func (m *Manager) parse(token, wantType string, secret []byte) (uuid.UUID, error) {
	var claims Claims
	_, err := jwt.ParseWithClaims(token, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return secret, nil
	})
	if err != nil || claims.Type != wantType {
		return uuid.Nil, ErrInvalidToken
	}
	id, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, ErrInvalidToken
	}
	return id, nil
}
