// Package auth handles secret hashing, recovery-code generation, and JWT tokens.
package auth

import (
	"crypto/rand"
	"encoding/base32"

	"golang.org/x/crypto/bcrypt"
)

// HashSecret bcrypt-hashes a PIN or recovery code.
func HashSecret(secret string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// VerifySecret reports whether secret matches the stored bcrypt hash.
func VerifySecret(hash, secret string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(secret)) == nil
}

// GenerateRecoveryCode returns a random, human-storable recovery code
// (Crockford-ish base32, no padding). Shown to the user exactly once.
func GenerateRecoveryCode() (string, error) {
	buf := make([]byte, 10) // 80 bits -> 16 base32 chars
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	return enc.EncodeToString(buf), nil
}
