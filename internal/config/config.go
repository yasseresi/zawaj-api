// Package config loads and validates application configuration from the environment.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration, parsed once at startup.
type Config struct {
	Env  string
	Port string

	DatabaseURL string

	JWTAccessSecret  string
	JWTRefreshSecret string
	JWTAccessTTL     time.Duration
	JWTRefreshTTL    time.Duration

	LoginMaxAttempts int
	LoginLockout     time.Duration

	// CORSAllowedOrigins is the exact list of browser origins allowed to make
	// cross-origin requests. A single "*" allows any origin (dev only). Empty in
	// production means no cross-origin browser access (native apps are unaffected,
	// as they don't send an Origin header the browser enforces).
	CORSAllowedOrigins []string
}

// Load reads configuration from the environment (and an optional .env file for
// local development). It returns an error if any required value is missing.
func Load() (*Config, error) {
	// Best-effort: a missing .env is fine in real environments.
	_ = godotenv.Load()

	cfg := &Config{
		Env:              env("APP_ENV", "development"),
		Port:             env("APP_PORT", "8080"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		JWTAccessSecret:  os.Getenv("JWT_ACCESS_SECRET"),
		JWTRefreshSecret: os.Getenv("JWT_REFRESH_SECRET"),
		JWTAccessTTL:     durationEnv("JWT_ACCESS_TTL", 15*time.Minute),
		JWTRefreshTTL:    durationEnv("JWT_REFRESH_TTL", 720*time.Hour),
		LoginMaxAttempts: intEnv("LOGIN_MAX_ATTEMPTS", 5),
		LoginLockout:     durationEnv("LOGIN_LOCKOUT", 15*time.Minute),
	}

	// CORS: explicit allowlist from env; default to wildcard in dev, deny in prod.
	if raw := os.Getenv("CORS_ALLOWED_ORIGINS"); raw != "" {
		cfg.CORSAllowedOrigins = splitTrim(raw)
	} else if cfg.Env != "production" {
		cfg.CORSAllowedOrigins = []string{"*"}
	}
	// A wildcard in production is almost never intended — fail fast.
	if cfg.Env == "production" {
		for _, o := range cfg.CORSAllowedOrigins {
			if o == "*" {
				return nil, fmt.Errorf("config: CORS_ALLOWED_ORIGINS must not be '*' in production")
			}
		}
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("config: DATABASE_URL is required")
	}
	// JWT secrets are required for auth (P1) but not for P0 health checks; warn via
	// validation only when they are empty to keep the scaffold runnable.
	if cfg.JWTAccessSecret == "" || cfg.JWTRefreshSecret == "" {
		return nil, fmt.Errorf("config: JWT_ACCESS_SECRET and JWT_REFRESH_SECRET are required")
	}

	return cfg, nil
}

// IsProduction reports whether the app runs in a production environment.
func (c *Config) IsProduction() bool { return c.Env == "production" }

// splitTrim splits a comma-separated env value into trimmed, non-empty items.
func splitTrim(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func intEnv(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
