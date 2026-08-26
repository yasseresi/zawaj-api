// Package test holds black-box integration tests that exercise the wired router
// against a real Postgres. Set TEST_DATABASE_URL to run them; they skip otherwise.
package test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"zawaj/internal/auth"
	"zawaj/internal/database"
	"zawaj/internal/handler"
	"zawaj/internal/repository"
	"zawaj/internal/router"
	"zawaj/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// newApp builds the fully wired engine against a clean test database. It skips
// the test when TEST_DATABASE_URL is not set.
func newApp(t *testing.T) *gin.Engine {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	gin.SetMode(gin.TestMode)

	db, err := database.New(dsn, true)
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cleanDB(t, db)

	tokens := auth.NewManager("test-access", "test-refresh", 15*time.Minute, 720*time.Hour)
	authModule := handler.NewAuth(service.NewAuthService(repository.NewUserRepo(db), tokens), tokens)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return router.New(db, log, true, authModule)
}

// cleanDB truncates all tables so each test starts from empty.
func cleanDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec("TRUNCATE users RESTART IDENTITY CASCADE").Error; err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

// do performs a JSON request against the engine and returns status + decoded body.
func do(t *testing.T, e *gin.Engine, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	var buf io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	var out map[string]any
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return rec.Code, out
}

// dataOf returns the "data" object from a response envelope.
func dataOf(m map[string]any) map[string]any {
	if d, ok := m["data"].(map[string]any); ok {
		return d
	}
	return nil
}

var _ = http.StatusOK
