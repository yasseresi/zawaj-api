// Package router builds the Gin engine and wires global middleware. Feature
// modules register their own routes via the Module contract (see Register),
// so adding a module never edits this file — it only appends to the modules list
// passed by main. This is the seam that lets modules be built in parallel.
package router

import (
	"log/slog"

	"zawaj/internal/handler"
	"zawaj/internal/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Module is implemented by every feature package. Register attaches the module's
// routes under the given /api/v1 group. Modules must not touch global wiring.
type Module interface {
	Register(rg *gin.RouterGroup)
}

// New builds the engine with global middleware, health probes, and the given
// feature modules mounted under /api/v1.
func New(db *gorm.DB, log *slog.Logger, production bool, modules ...Module) *gin.Engine {
	if production {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(
		middleware.SecurityHeaders(production),
		middleware.CORS(),
		middleware.RequestID(),
		middleware.Logger(log),
		middleware.Recover(log),
	)

	health := handler.NewHealth(db)
	r.GET("/healthz", health.Live)
	r.GET("/readyz", health.Ready)

	api := r.Group("/api/v1")
	for _, m := range modules {
		m.Register(api)
	}

	return r
}
