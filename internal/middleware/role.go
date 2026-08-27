package middleware

import (
	"errors"

	"zawaj/internal/apperr"
	"zawaj/internal/models"
	"zawaj/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	ctxWeddingID = "wedding_id"
	ctxRole      = "role"
)

// RequireRole ensures the caller is a member of the wedding named by the ":id"
// path param with at least `min` role. Missing membership -> 404 (hide existence);
// insufficient role -> 403. On success it stores the wedding id and role in ctx.
func RequireRole(repo *repository.WeddingRepo, min models.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := UserID(c)
		if !ok {
			apperr.Write(c, apperr.Unauthenticated("authentication required"))
			c.Abort()
			return
		}
		weddingID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			apperr.Write(c, apperr.NotFound("wedding not found"))
			c.Abort()
			return
		}
		role, err := repo.GetRole(c.Request.Context(), weddingID, userID)
		if errors.Is(err, repository.ErrNotFound) {
			apperr.Write(c, apperr.NotFound("wedding not found"))
			c.Abort()
			return
		}
		if err != nil {
			apperr.Write(c, apperr.Internal("authorization check failed"))
			c.Abort()
			return
		}
		if role.Rank() < min.Rank() {
			apperr.Write(c, apperr.Forbidden("insufficient permissions"))
			c.Abort()
			return
		}
		c.Set(ctxWeddingID, weddingID)
		c.Set(ctxRole, role)
		c.Next()
	}
}

// WeddingID returns the wedding id validated by RequireRole.
func WeddingID(c *gin.Context) uuid.UUID {
	v, _ := c.Get(ctxWeddingID)
	id, _ := v.(uuid.UUID)
	return id
}

// CurrentRole returns the caller's role set by RequireRole.
func CurrentRole(c *gin.Context) models.Role {
	v, _ := c.Get(ctxRole)
	r, _ := v.(models.Role)
	return r
}
