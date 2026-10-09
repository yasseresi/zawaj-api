// Package middleware holds cross-cutting Gin middleware: request id, structured
// logging, panic recovery, and CORS. Auth/role middleware live alongside in P1/P3.
package middleware

import (
	"context"
	"log/slog"
	"time"

	"zawaj/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

const (
	headerRequestID = "X-Request-ID"
	ctxRequestID    = "request_id"
)

// RequestID assigns (or honors an inbound) request id and echoes it back.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(headerRequestID)
		if id == "" {
			id = uuid.NewString()
		}
		c.Set(ctxRequestID, id)
		c.Writer.Header().Set(headerRequestID, id)
		c.Next()
	}
}

// routeOf is the matched route template (e.g. /api/v1/invite/:token), never
// the raw path: some paths carry bearer secrets (invite tokens) that must not
// reach log storage. Unmatched requests log "unmatched".
func routeOf(c *gin.Context) string {
	if p := c.FullPath(); p != "" {
		return p
	}
	return "unmatched"
}

// Logger emits one structured line per request after it completes.
func Logger(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		attrs := []any{
			"method", c.Request.Method,
			"path", routeOf(c),
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", c.GetString(ctxRequestID),
		}
		// Correlate logs with traces when tracing is active.
		if sc := trace.SpanContextFromContext(c.Request.Context()); sc.HasTraceID() {
			attrs = append(attrs, "trace_id", sc.TraceID().String())
		}
		log.Info("request", attrs...)
	}
}

// Recover converts a panic into a 500 envelope instead of crashing the process.
func Recover(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Error("panic recovered",
					"error", r,
					"path", routeOf(c),
					"request_id", c.GetString(ctxRequestID),
				)
				response.Error(c, 500, response.CodeInternal, "internal server error")
				c.Abort()
			}
		}()
		c.Next()
	}
}

// Timeout attaches a deadline to each request's context so slow DB queries are
// cancelled rather than piling up connections. Handlers propagate this via
// c.Request.Context().
func Timeout(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// SecurityHeaders sets baseline hardening headers on every response. HSTS is
// only sent in production (avoids pinning localhost to HTTPS during dev).
func SecurityHeaders(production bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		if production {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	}
}

// CORS reflects the request Origin only when it is in the allowlist. A single
// "*" entry allows any origin (dev). An empty allowlist sends no CORS headers,
// so browsers block cross-origin requests (native apps are unaffected). Requests
// without an Origin header (server-to-server, native clients) always pass.
func CORS(allowedOrigins []string) gin.HandlerFunc {
	wildcard := false
	allow := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o == "*" {
			wildcard = true
		}
		allow[o] = struct{}{}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			if wildcard {
				c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
			} else if _, ok := allow[origin]; ok {
				h := c.Writer.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Add("Vary", "Origin")
			}
		}
		h := c.Writer.Header()
		h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
