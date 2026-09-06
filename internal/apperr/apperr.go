// Package apperr defines the shared application-error type and the single helper
// that maps errors to the JSON envelope. Every service returns *apperr.Error;
// every handler calls Write. Defining this in the spine gives the swarm modules
// (guests, notifications, stats) one consistent error contract.
package apperr

import (
	"errors"
	"net/http"

	"zawaj/pkg/response"

	"github.com/gin-gonic/gin"
)

// Error is a typed application error carrying an HTTP status and a stable code.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

// New builds a typed error.
func New(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// Constructors for the common cases (codes match response.Code* + SYSTEM_DESIGN §4).
func Validation(msg string) *Error { return New(http.StatusBadRequest, response.CodeValidation, msg) }
func Unauthenticated(msg string) *Error {
	return New(http.StatusUnauthorized, response.CodeUnauthenticated, msg)
}
func Forbidden(msg string) *Error { return New(http.StatusForbidden, response.CodeForbidden, msg) }
func NotFound(msg string) *Error  { return New(http.StatusNotFound, response.CodeNotFound, msg) }
func Conflict(msg string) *Error  { return New(http.StatusConflict, response.CodeConflict, msg) }
func UsernameTaken(msg string) *Error {
	return New(http.StatusConflict, response.CodeUsernameTaken, msg)
}
func InvitePending(msg string) *Error {
	return New(http.StatusConflict, response.CodeInvitePending, msg)
}
func AlreadyMember(msg string) *Error {
	return New(http.StatusConflict, response.CodeAlreadyMember, msg)
}
func SelfInvite(msg string) *Error {
	return New(http.StatusBadRequest, response.CodeSelfInvite, msg)
}
func Locked(msg string) *Error { return New(http.StatusLocked, response.CodeLocked, msg) }
func RateLimited(msg string) *Error {
	return New(http.StatusTooManyRequests, response.CodeRateLimited, msg)
}
func Internal(msg string) *Error {
	return New(http.StatusInternalServerError, response.CodeInternal, msg)
}

// Write maps err to the JSON envelope. Typed *Error is used as-is; anything else
// becomes a 500 (details never leaked to the client).
func Write(c *gin.Context, err error) {
	var ae *Error
	if errors.As(err, &ae) {
		response.Error(c, ae.Status, ae.Code, ae.Message)
		return
	}
	response.Error(c, http.StatusInternalServerError, response.CodeInternal, "internal server error")
}
