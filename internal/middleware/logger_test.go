package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// Request logs carry the route template, never the raw path: paths like
// /invite/:token hold bearer secrets that must not land in log storage.
func TestLoggerRecordsRouteTemplateNotRawPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	r := gin.New()
	r.Use(Logger(log))
	r.GET("/api/v1/invite/:token", func(c *gin.Context) { c.Status(http.StatusOK) })

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/invite/MFRGGZDFMZTWQ2LKNNWG23TP", nil))
	if line := buf.String(); strings.Contains(line, "MFRGGZDFMZTWQ2LKNNWG23TP") || !strings.Contains(line, `"/api/v1/invite/:token"`) {
		t.Fatalf("request log must hold the route template, not the token: %s", line)
	}

	buf.Reset()
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/no/such/SECRET", nil))
	if line := buf.String(); strings.Contains(line, "SECRET") || !strings.Contains(line, `"unmatched"`) {
		t.Fatalf("unmatched routes must not echo the raw path: %s", line)
	}
}

// A panic's log line must also hold the route template, never the token.
func TestRecoverLogsRouteTemplateNotRawPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	r := gin.New()
	r.Use(Recover(log))
	r.GET("/api/v1/invite/:token", func(c *gin.Context) { panic("boom") })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/invite/MFRGGZDFMZTWQ2LKNNWG23TP", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("panic: want 500, got %d", rec.Code)
	}
	if line := buf.String(); strings.Contains(line, "MFRGGZDFMZTWQ2LKNNWG23TP") || !strings.Contains(line, `"/api/v1/invite/:token"`) {
		t.Fatalf("panic log must hold the route template, not the token: %s", line)
	}
}
