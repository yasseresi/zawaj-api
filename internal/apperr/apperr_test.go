package apperr

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"zawaj/pkg/response"

	"github.com/gin-gonic/gin"
)

func TestConstructorsMapToStatusAndCode(t *testing.T) {
	cases := []struct {
		err        *Error
		wantStatus int
		wantCode   string
	}{
		{Validation("x"), http.StatusBadRequest, response.CodeValidation},
		{Unauthenticated("x"), http.StatusUnauthorized, response.CodeUnauthenticated},
		{Forbidden("x"), http.StatusForbidden, response.CodeForbidden},
		{NotFound("x"), http.StatusNotFound, response.CodeNotFound},
		{Conflict("x"), http.StatusConflict, response.CodeConflict},
		{Locked("x"), http.StatusLocked, response.CodeLocked},
		{RateLimited("x"), http.StatusTooManyRequests, response.CodeRateLimited},
		{Internal("x"), http.StatusInternalServerError, response.CodeInternal},
	}
	for _, c := range cases {
		if c.err.Status != c.wantStatus || c.err.Code != c.wantCode {
			t.Fatalf("%s: got (%d,%s) want (%d,%s)", c.err.Code, c.err.Status, c.err.Code, c.wantStatus, c.wantCode)
		}
	}
}

func TestWriteTypedError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	Write(c, NotFound("gone"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("typed error status: want 404, got %d", w.Code)
	}
}

func TestWriteUntypedErrorIs500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	Write(c, errors.New("boom")) // not an *Error
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("untyped error status: want 500, got %d", w.Code)
	}
	// Internal details must not leak the raw message.
	if body := w.Body.String(); body == "" || contains(body, "boom") {
		t.Fatalf("internal error leaked raw message: %s", body)
	}
}

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
