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

// Logger emits one structured line per request after it completes.
func Logger(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Info("request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", c.GetString(ctxRequestID),
		)
	}
}

// Recover converts a panic into a 500 envelope instead of crashing the process.
func Recover(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Error("panic recovered",
					"error", r,
					"path", c.Request.URL.Path,
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

// CORS applies permissive CORS suitable for a mobile client and local dev.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
