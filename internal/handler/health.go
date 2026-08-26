// Package handler contains the HTTP handlers, grouped by resource. Health checks
// live here; resource handlers (auth, wedding, guest) are added in later phases.
package handler

import (
	"net/http"

	"zawaj/internal/database"
	"zawaj/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Health serves liveness and readiness probes.
type Health struct {
	db *gorm.DB
}

// NewHealth builds the health handler.
func NewHealth(db *gorm.DB) *Health { return &Health{db: db} }

// Live reports the process is up (no dependencies checked).
func (h *Health) Live(c *gin.Context) {
	response.JSON(c, http.StatusOK, gin.H{"status": "ok"})
}

// Ready reports the process is up AND the database is reachable.
func (h *Health) Ready(c *gin.Context) {
	if err := database.Ping(h.db); err != nil {
		response.Error(c, http.StatusServiceUnavailable, response.CodeInternal, "database unreachable")
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"status": "ready"})
}
