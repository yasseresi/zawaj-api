// Package response provides the uniform JSON envelope and stable error codes
// shared by every handler. The Flutter client depends on error.code being stable.
package response

import "github.com/gin-gonic/gin"

// Stable machine-readable error codes (see SYSTEM_DESIGN.md §4 error taxonomy).
const (
	CodeValidation      = "validation_error"
	CodeUnauthenticated = "unauthenticated"
	CodeForbidden       = "forbidden"
	CodeNotFound        = "not_found"
	CodeConflict        = "conflict"
	CodeLocked          = "locked"
	CodeRateLimited     = "rate_limited"
	CodeInternal        = "internal"
	CodeUsernameTaken   = "username_taken"
	CodeInvitePending   = "invite_pending"
	CodeAlreadyMember   = "already_member"
	CodeSelfInvite      = "self_invite"
	// CodeRemovedFromWedding: the owner removed this user; only a new owner
	// invite (by username) can re-admit them, not an invite link.
	CodeRemovedFromWedding = "removed_from_wedding"
	// CodeJoinRequestDeclined: the owner recently declined this user's join
	// request; they can ask again after a cooldown.
	CodeJoinRequestDeclined = "join_request_declined"
)

// Envelope is the single response shape for the whole API.
type Envelope struct {
	Data  any       `json:"data"`
	Error *APIError `json:"error"`
}

// APIError is the machine-readable error body.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// JSON writes a success envelope with the given status and data.
func JSON(c *gin.Context, status int, data any) {
	c.JSON(status, Envelope{Data: data})
}

// Error writes an error envelope with the given status and code.
func Error(c *gin.Context, status int, code, message string) {
	c.JSON(status, Envelope{Error: &APIError{Code: code, Message: message}})
}

// ErrorDetails writes an error envelope with extra machine-readable details
// (e.g. per-field validation failures).
func ErrorDetails(c *gin.Context, status int, code, message string, details any) {
	c.JSON(status, Envelope{Error: &APIError{Code: code, Message: message, Details: details}})
}
