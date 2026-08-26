package middleware

import (
	"strings"

	"zawaj/internal/apperr"
	"zawaj/internal/auth"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ctxUserID is the gin context key for the authenticated user id.
const ctxUserID = "user_id"

// RequireAuth validates the Bearer access token and stores the user id in context.
func RequireAuth(tokens *auth.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || token == "" {
			apperr.Write(c, apperr.Unauthenticated("missing bearer token"))
			c.Abort()
			return
		}
		userID, err := tokens.ParseAccess(token)
		if err != nil {
			apperr.Write(c, apperr.Unauthenticated("invalid or expired token"))
			c.Abort()
			return
		}
		c.Set(ctxUserID, userID)
		c.Next()
	}
}

// UserID returns the authenticated user id set by RequireAuth. The bool is false
// if the request did not pass through RequireAuth.
func UserID(c *gin.Context) (uuid.UUID, bool) {
	v, ok := c.Get(ctxUserID)
	if !ok {
		return uuid.Nil, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok
}
