// Package router builds the Gin engine and wires global middleware. Feature
// modules register their own routes via the Module contract (see Register),
// so adding a module never edits this file — it only appends to the modules list
// passed by main. This is the seam that lets modules be built in parallel.
package router

import (
	"log/slog"
	"time"

	"zawaj/internal/auth"
	"zawaj/internal/handler"
	"zawaj/internal/middleware"
	"zawaj/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"gorm.io/gorm"
)

// serviceName labels traces for this service.
const serviceName = "zawaj-api"

// requestTimeout bounds how long any single request may run before its context
// is cancelled (propagated to DB queries). Well above p95 for v1 workloads.
const requestTimeout = 15 * time.Second

// Module is implemented by every feature package. Register attaches the module's
// routes under the given /api/v1 group. Modules must not touch global wiring.
type Module interface {
	Register(rg *gin.RouterGroup)
}

// New builds the engine with global middleware, health probes, and the given
// feature modules mounted under /api/v1. corsOrigins is the CORS allowlist;
// rateRPS/rateBurst configure the per-IP rate limiter (rateRPS <= 0 disables it).
func New(db *gorm.DB, log *slog.Logger, production bool, corsOrigins []string, rateRPS float64, rateBurst int, tokens *auth.Manager, idem *repository.IdempotencyRepo, modules ...Module) *gin.Engine {
	if production {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	// Do not trust X-Forwarded-For by default: ClientIP() then reflects the direct
	// peer, so a client cannot spoof its rate-limit key. Behind a known proxy/LB,
	// set trusted proxies explicitly in deployment.
	_ = r.SetTrustedProxies(nil)

	r.Use(
		otelgin.Middleware(serviceName), // no-op unless a tracer provider is configured
		middleware.Metrics(),
		middleware.SecurityHeaders(production),
		middleware.CORS(corsOrigins),
		middleware.RateLimit(rateRPS, rateBurst),
		middleware.Idempotency(tokens, idem),
		middleware.RequestID(),
		middleware.Logger(log),
		middleware.Recover(log),
		middleware.Timeout(requestTimeout),
	)

	health := handler.NewHealth(db)
	r.GET("/healthz", health.Live)
	r.GET("/readyz", health.Ready)
	// Prometheus scrape endpoint. Restrict access at the network/proxy layer
	// (internal-only) rather than exposing it publicly.
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	api := r.Group("/api/v1")
	for _, m := range modules {
		m.Register(api)
	}

	return r
}
