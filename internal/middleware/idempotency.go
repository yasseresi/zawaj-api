package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"zawaj/internal/auth"
	"zawaj/internal/repository"
	"zawaj/pkg/response"

	"github.com/gin-gonic/gin"
)

// maxIdempotentBody caps the response size cached for replay (larger responses
// are served normally but not stored).
const maxIdempotentBody = 64 * 1024

// bodyCaptureWriter tees the response body into a buffer while still writing it
// to the client, so a successful response can be stored for idempotent replay.
type bodyCaptureWriter struct {
	gin.ResponseWriter
	buf bytes.Buffer
}

func (w *bodyCaptureWriter) Write(b []byte) (int, error) {
	w.buf.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *bodyCaptureWriter) WriteString(s string) (int, error) {
	w.buf.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

// Idempotency makes mutating (POST) requests safe to retry SEQUENTIALLY. When a
// request carries an Idempotency-Key header and a valid bearer token, the first
// completed response is stored per (user, key); a later retry with the same key
// replays it verbatim. Reusing a key for a materially different request
// (different path or body) is rejected with 409. Requests without the header, or
// from unauthenticated callers, pass through unchanged (downstream auth applies).
//
// Guarantee and its limit: this de-duplicates retries that arrive AFTER the first
// request finished. It does NOT serialize two identical requests that are truly
// in flight at the same time — both miss the store and execute, and only the
// second store loses on the unique index. For strict once-only semantics under
// concurrency, reserve the key with a pending row before c.Next() instead.
func Idempotency(tokens *auth.Manager, repo *repository.IdempotencyRepo) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}
		key := c.GetHeader("Idempotency-Key")
		if key == "" {
			c.Next()
			return
		}
		bearer, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !ok || bearer == "" {
			c.Next()
			return
		}
		userID, err := tokens.ParseAccess(bearer)
		if err != nil {
			c.Next() // let the route's own auth reject it
			return
		}

		// Read + restore the body so we can hash it and the handler can still bind it.
		body, _ := io.ReadAll(c.Request.Body)
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		sum := sha256.Sum256(append([]byte(c.Request.URL.Path+"\n"), body...))
		reqHash := hex.EncodeToString(sum[:])

		existing, err := repo.Get(c.Request.Context(), userID, key)
		switch {
		case err == nil:
			if existing.RequestHash != reqHash {
				response.Error(c, http.StatusConflict, response.CodeConflict,
					"Idempotency-Key reused with a different request")
				c.Abort()
				return
			}
			c.Header("Idempotent-Replayed", "true")
			c.Data(existing.StatusCode, "application/json; charset=utf-8", existing.Response)
			c.Abort()
			return
		case !errors.Is(err, repository.ErrNotFound):
			// Store lookup failed; fail open (process normally, do not cache).
			c.Next()
			return
		}

		// First time for this key: capture the response, then store it if successful.
		cap := &bodyCaptureWriter{ResponseWriter: c.Writer}
		c.Writer = cap
		c.Next()

		status := c.Writer.Status()
		if status >= 200 && status < 300 && cap.buf.Len() <= maxIdempotentBody {
			// The request context is already cancelled here (Timeout middleware),
			// so persist the outcome with a fresh, short-lived context.
			storeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = repo.Put(storeCtx, userID, key, reqHash, status, cap.buf.Bytes())
			cancel()
		}
	}
}
