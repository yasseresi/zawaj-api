// Package validation contains request rules shared by all API entry points.
package validation

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	UsernameMin = 3
	UsernameMax = 120
	PasswordMin = 8
	PasswordMax = 72
)

var (
	usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9._@-]+$`)
	letterPattern   = regexp.MustCompile(`[A-Za-z]`)
	lowerPattern    = regexp.MustCompile(`[a-z]`)
	upperPattern    = regexp.MustCompile(`[A-Z]`)
	numberPattern   = regexp.MustCompile(`[0-9]`)
	symbolPattern   = regexp.MustCompile(`[^A-Za-z0-9\s]`)
)

var commonPasswords = map[string]struct{}{
	"password": {}, "password1": {}, "password123": {},
	"12345678": {}, "qwerty123": {},
}

func NormalizeUsername(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func Username(value string) error {
	v := strings.TrimSpace(value)
	switch {
	case v == "":
		return fmt.Errorf("username is required")
	case len(v) < UsernameMin:
		return fmt.Errorf("username must be at least %d characters", UsernameMin)
	case len(v) > UsernameMax:
		return fmt.Errorf("username must be at most %d characters", UsernameMax)
	case strings.IndexFunc(v, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' }) >= 0:
		return fmt.Errorf("username cannot contain spaces")
	case !usernamePattern.MatchString(v):
		return fmt.Errorf("username contains invalid characters")
	default:
		return nil
	}
}

func Password(value string) error {
	switch {
	case value == "":
		return fmt.Errorf("password is required")
	case len(value) < PasswordMin:
		return fmt.Errorf("password must be at least %d characters", PasswordMin)
	case len(value) > PasswordMax:
		return fmt.Errorf("password must be at most %d characters", PasswordMax)
	case strings.IndexFunc(value, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' }) >= 0:
		return fmt.Errorf("password cannot contain spaces")
	case !letterPattern.MatchString(value):
		return fmt.Errorf("password must contain a letter")
	case !lowerPattern.MatchString(value):
		return fmt.Errorf("password must contain a lowercase letter")
	case !upperPattern.MatchString(value):
		return fmt.Errorf("password must contain an uppercase letter")
	case !numberPattern.MatchString(value):
		return fmt.Errorf("password must contain a number")
	case !symbolPattern.MatchString(value):
		return fmt.Errorf("password must contain a symbol")
	case hasCommonPassword(value):
		return fmt.Errorf("password is too common")
	default:
		return nil
	}
}

func hasCommonPassword(value string) bool {
	_, ok := commonPasswords[strings.ToLower(value)]
	return ok
}
