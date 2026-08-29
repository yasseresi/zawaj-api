package config

import (
	"os"
	"path/filepath"
	"testing"
)

// setProdBase sets a valid production baseline; individual tests override pieces.
func setProdBase(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://u:p@db:5432/zawaj?sslmode=require")
	t.Setenv("JWT_ACCESS_SECRET", "an-access-secret-that-is-long-enough-xx")
	t.Setenv("JWT_REFRESH_SECRET", "a-different-refresh-secret-long-enough-y")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.example")
}

func TestLoad_ProductionValid(t *testing.T) {
	setProdBase(t)
	if _, err := Load(); err != nil {
		t.Fatalf("valid prod config: unexpected error: %v", err)
	}
}

func TestLoad_ProductionRejectsWeakSecret(t *testing.T) {
	setProdBase(t)
	t.Setenv("JWT_ACCESS_SECRET", "short")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for short secret in production")
	}
}

func TestLoad_ProductionRejectsPlaceholderSecret(t *testing.T) {
	setProdBase(t)
	t.Setenv("JWT_ACCESS_SECRET", "dev-access-secret-change-me-please-000")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for placeholder secret in production")
	}
}

func TestLoad_ProductionRejectsEqualSecrets(t *testing.T) {
	setProdBase(t)
	same := "identical-secret-value-long-enough-for-32b"
	t.Setenv("JWT_ACCESS_SECRET", same)
	t.Setenv("JWT_REFRESH_SECRET", same)
	if _, err := Load(); err == nil {
		t.Fatal("expected error when access and refresh secrets are equal")
	}
}

func TestLoad_ProductionRejectsInsecureDB(t *testing.T) {
	setProdBase(t)
	t.Setenv("DATABASE_URL", "postgres://u:p@db:5432/zawaj?sslmode=disable")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for sslmode=disable in production")
	}
}

func TestLoad_ProductionRejectsWildcardCORS(t *testing.T) {
	setProdBase(t)
	t.Setenv("CORS_ALLOWED_ORIGINS", "*")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for wildcard CORS in production")
	}
}

func TestLoad_DevDefaultsCORSWildcard(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost/zawaj?sslmode=disable")
	t.Setenv("JWT_ACCESS_SECRET", "dev-access")
	t.Setenv("JWT_REFRESH_SECRET", "dev-refresh")
	t.Setenv("CORS_ALLOWED_ORIGINS", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("dev config: %v", err)
	}
	if len(cfg.CORSAllowedOrigins) != 1 || cfg.CORSAllowedOrigins[0] != "*" {
		t.Fatalf("dev CORS: want [*], got %v", cfg.CORSAllowedOrigins)
	}
}

func TestSecretEnv_ReadsFromFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "secret")
	if err := os.WriteFile(p, []byte("  file-secret-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MY_SECRET_FILE", p)
	t.Setenv("MY_SECRET", "env-value") // _FILE takes precedence
	if got := secretEnv("MY_SECRET"); got != "file-secret-value" {
		t.Fatalf("secretEnv from file: want %q, got %q", "file-secret-value", got)
	}
}
