// Package token generates random, URL-safe short tokens for invite links.
package token

import (
	"crypto/rand"
	"encoding/base32"
)

var enc = base32.StdEncoding.WithPadding(base32.NoPadding)

// New returns a random token encoded from nBytes of entropy (base32, no padding).
// 10 bytes -> 16 chars; 15 bytes -> 24 chars.
func New(nBytes int) (string, error) {
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return enc.EncodeToString(buf), nil
}
