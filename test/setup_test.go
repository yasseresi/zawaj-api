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
	"sync"
	"testing"
	"time"

	"zawaj/internal/auth"
	"zawaj/internal/database"
	"zawaj/internal/handler"
	"zawaj/internal/push"
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
	return newAppWithPusher(t, nil)
}

// newAppWithPusher is newApp with a custom push sender (nil = no-op), for
// tests that assert on what gets pushed.
func newAppWithPusher(t *testing.T, pusher push.Sender) *gin.Engine {
	t.Helper()
	e, _ := newAppWithDB(t, pusher)
	return e
}

// newAppWithDB is newAppWithPusher that also returns the database handle, for
// tests that exercise repositories directly or inspect stored rows.
func newAppWithDB(t *testing.T, pusher push.Sender) (*gin.Engine, *gorm.DB) {
	t.Helper()
	a := newTestApp(t, pusher)
	return a.e, a.db
}

// testApp is the fully wired engine plus the handles tests reach into.
type testApp struct {
	e     *gin.Engine
	db    *gorm.DB
	notif *service.NotificationService // Wait() blocks until pushes are sent
}

// newTestApp builds the engine against a clean test database (skipping when
// TEST_DATABASE_URL is unset) and closes its connection pool when the test
// ends, so long runs don't exhaust Postgres connections.
func newTestApp(t *testing.T, pusher push.Sender) *testApp {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	gin.SetMode(gin.TestMode)

	db, err := database.New(dsn, true, database.PoolConfig{MaxOpenConns: 10, MaxIdleConns: 5, ConnMaxLifetime: time.Hour})
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	migrateOnce.Do(func() { migrateErr = resetSchema(db, dsn) })
	if migrateErr != nil {
		t.Fatalf("migrate: %v", migrateErr)
	}
	cleanDB(t, db)

	tokens := auth.NewManager("test-access", "test-refresh", 15*time.Minute, 720*time.Hour)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	userRepo := repository.NewUserRepo(db)
	weddingRepo := repository.NewWeddingRepo(db)
	guestRepo := repository.NewGuestRepo(db)
	notifRepo := repository.NewNotificationRepo(db)
	statsRepo := repository.NewStatsRepo(db)
	deviceRepo := repository.NewDeviceTokenRepo(db)
	auditSvc := service.NewAuditService(repository.NewAuditRepo(db), log)
	activitySvc := service.NewActivityService(repository.NewActivityRepo(db), log)
	if pusher == nil {
		pusher = push.NewNoop(log)
	}
	notifSvc := service.NewNotificationService(notifRepo, weddingRepo, deviceRepo, pusher, log)
	guestSvc := service.NewGuestService(guestRepo, activitySvc, notifSvc)

	t.Cleanup(notifSvc.Wait) // let async pushes finish before the pool closes
	e := router.New(db, log, true, []string{"*"}, 0, 0, tokens, repository.NewIdempotencyRepo(db), // rate limiting disabled in tests
		handler.NewAuth(service.NewAuthService(userRepo, repository.NewRefreshTokenRepo(db), tokens, 5, time.Minute), auditSvc, tokens),
		handler.NewWedding(service.NewWeddingService(weddingRepo), auditSvc, weddingRepo, tokens),
		handler.NewGuest(guestSvc, activitySvc, weddingRepo, tokens),
		handler.NewActivity(activitySvc, weddingRepo, tokens),
		handler.NewStats(service.NewStatsService(statsRepo), weddingRepo, tokens),
		handler.NewNotification(notifSvc, tokens),
		handler.NewExport(guestRepo, weddingRepo, tokens, activitySvc),
		handler.NewDevice(deviceRepo, tokens),
		handler.NewInvite(
			service.NewInviteService(repository.NewInviteRepo(db), weddingRepo, userRepo, activitySvc, notifSvc),
			weddingRepo, tokens),
		handler.NewJoinRequest(
			service.NewJoinService(weddingRepo, repository.NewJoinRequestRepo(db), userRepo, activitySvc, notifSvc),
			auditSvc, weddingRepo, tokens),
	)
	return &testApp{e: e, db: db, notif: notifSvc}
}

var (
	migrateOnce sync.Once
	migrateErr  error
)

// resetSchema rebuilds the test database from the production SQL migrations
// (once per test process), so integration tests run against exactly the
// schema that ships — indexes, partial indexes and constraints included —
// rather than a GORM AutoMigrate approximation.
func resetSchema(db *gorm.DB, dsn string) error {
	if err := db.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public;").Error; err != nil {
		return err
	}
	return database.RunMigrations(dsn)
}

// cleanDB truncates all tables so each test starts from empty.
func cleanDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	err := db.Exec("TRUNCATE users, weddings, memberships, membership_ceilings, join_requests, wedding_invites, invite_links, guests, guest_notes, activity_logs, notifications, device_tokens, refresh_tokens, idempotency_keys, audit_logs RESTART IDENTITY CASCADE").Error
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

// newReq builds an HTTP request with optional JSON body and bearer token.
func newReq(method, path, token string, body any) *http.Request {
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
	return req
}

// serve runs a request through the engine and returns the raw recorder.
func serve(e *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
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

// register creates a user and returns its access token and user id.
func register(t *testing.T, e *gin.Engine, username string) (access, userID string) {
	t.Helper()
	code, body := do(t, e, "POST", "/api/v1/auth/register", "", map[string]any{
		"username": username, "display_name": username, "password": "Password123!",
	})
	if code != 201 {
		t.Fatalf("register %s: want 201, got %d (%v)", username, code, body)
	}
	d := dataOf(body)
	access, _ = d["access"].(string)
	if u, ok := d["user"].(map[string]any); ok {
		userID, _ = u["id"].(string)
	}
	return access, userID
}

// createWedding makes a wedding owned by token's user and returns its id.
func createWedding(t *testing.T, e *gin.Engine, token, name string) string {
	t.Helper()
	code, body := do(t, e, "POST", "/api/v1/weddings", token, map[string]any{"name": name})
	if code != 201 {
		t.Fatalf("create wedding: want 201, got %d (%v)", code, body)
	}
	id, _ := dataOf(body)["id"].(string)
	return id
}

// joinAs mints a link of the given role and has token's user accept it.
func joinAs(t *testing.T, e *gin.Engine, ownerTok, wid, role, memberTok string) {
	t.Helper()
	_, lb := do(t, e, "POST", "/api/v1/weddings/"+wid+"/invite-links", ownerTok, map[string]any{"role": role})
	tok, _ := dataOf(lb)["token"].(string)
	if got := joinViaLink(t, e, ownerTok, wid, tok, memberTok); got != role {
		t.Fatalf("join as %s: got role %v", role, got)
	}
}

// dataOf returns the "data" object from a response envelope.
func dataOf(m map[string]any) map[string]any {
	if d, ok := m["data"].(map[string]any); ok {
		return d
	}
	return nil
}

var _ = http.StatusOK
