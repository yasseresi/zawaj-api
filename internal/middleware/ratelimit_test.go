package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newLimitEngine(rps float64, burst int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.Use(RateLimit(rps, burst))
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func hit(r *gin.Engine, ip string) int {
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = ip + ":12345"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func TestRateLimit_BlocksOverBurst(t *testing.T) {
	r := newLimitEngine(1, 2) // 1 rps, burst 2

	if c := hit(r, "1.2.3.4"); c != http.StatusOK {
		t.Fatalf("req 1: want 200, got %d", c)
	}
	if c := hit(r, "1.2.3.4"); c != http.StatusOK {
		t.Fatalf("req 2 (burst): want 200, got %d", c)
	}
	if c := hit(r, "1.2.3.4"); c != http.StatusTooManyRequests {
		t.Fatalf("req 3: want 429, got %d", c)
	}
}

func TestRateLimit_IsolatesByIP(t *testing.T) {
	r := newLimitEngine(1, 1) // burst 1

	if c := hit(r, "10.0.0.1"); c != http.StatusOK {
		t.Fatalf("ip1 req1: want 200, got %d", c)
	}
	if c := hit(r, "10.0.0.1"); c != http.StatusTooManyRequests {
		t.Fatalf("ip1 req2: want 429, got %d", c)
	}
	// A different IP has its own bucket.
	if c := hit(r, "10.0.0.2"); c != http.StatusOK {
		t.Fatalf("ip2 req1: want 200, got %d", c)
	}
}

func TestRateLimit_DisabledWhenRPSZero(t *testing.T) {
	r := newLimitEngine(0, 0)
	for i := 0; i < 50; i++ {
		if c := hit(r, "9.9.9.9"); c != http.StatusOK {
			t.Fatalf("disabled limiter req %d: want 200, got %d", i, c)
		}
	}
}
