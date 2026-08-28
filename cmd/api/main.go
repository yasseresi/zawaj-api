// Command api is the Zawaj backend entrypoint: load config, connect the database,
// build the router, and serve HTTP with graceful shutdown.
//
// @title           Zawaj API
// @version         1.0
// @description     Wedding guest-list backend. Collaborative RSVP management with
// @description     role-based access (owner/editor/viewer), invite links, stats,
// @description     activity feed, notifications, and CSV export.
// @description
// @description     All responses use the envelope `{ "data": ..., "error": null }`.
// @description     On error, `data` is null and `error` is `{ "code", "message", "details" }`.
// @description     Authenticate via `POST /auth/login`, then send `Authorization: Bearer <access>`.
//
// @contact.name    Zawaj
// @BasePath        /api/v1
//
// @securityDefinitions.apikey  BearerAuth
// @in                          header
// @name                        Authorization
// @description                 Type "Bearer" followed by a space and the access JWT.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"zawaj/internal/auth"
	"zawaj/internal/config"
	"zawaj/internal/database"
	"zawaj/internal/handler"
	"zawaj/internal/push"
	"zawaj/internal/repository"
	"zawaj/internal/router"
	"zawaj/internal/service"

	_ "zawaj/docs/swagger" // generated OpenAPI spec (swag init)

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		log.Error("config load failed", "error", err)
		os.Exit(1)
	}

	db, err := database.New(cfg.DatabaseURL, cfg.IsProduction())
	if err != nil {
		log.Error("database connect failed", "error", err)
		os.Exit(1)
	}

	if err := database.RunMigrations(cfg.DatabaseURL); err != nil {
		log.Error("migration failed", "error", err)
		os.Exit(1)
	}

	// Dependency wiring. Feature modules are appended to the router as phases land.
	tokens := auth.NewManager(cfg.JWTAccessSecret, cfg.JWTRefreshSecret, cfg.JWTAccessTTL, cfg.JWTRefreshTTL)
	userRepo := repository.NewUserRepo(db)
	weddingRepo := repository.NewWeddingRepo(db)
	guestRepo := repository.NewGuestRepo(db)
	activityRepo := repository.NewActivityRepo(db)

	notifRepo := repository.NewNotificationRepo(db)
	statsRepo := repository.NewStatsRepo(db)
	deviceRepo := repository.NewDeviceTokenRepo(db)

	// FCM push sender. Falls back to a no-op when Firebase credentials are absent
	// (dev/CI); set GOOGLE_APPLICATION_CREDENTIALS to enable real delivery.
	pusher := push.New(context.Background(), log)

	activitySvc := service.NewActivityService(activityRepo, log)
	notifSvc := service.NewNotificationService(notifRepo, weddingRepo, deviceRepo, pusher, log)
	guestSvc := service.NewGuestService(guestRepo, activitySvc, notifSvc)

	authModule := handler.NewAuth(service.NewAuthService(userRepo, tokens, cfg.LoginMaxAttempts, cfg.LoginLockout), tokens)
	weddingModule := handler.NewWedding(service.NewWeddingService(weddingRepo), weddingRepo, tokens)
	guestModule := handler.NewGuest(guestSvc, activitySvc, weddingRepo, tokens)
	activityModule := handler.NewActivity(activitySvc, weddingRepo, tokens)
	statsModule := handler.NewStats(service.NewStatsService(statsRepo), weddingRepo, tokens)
	notifModule := handler.NewNotification(notifSvc, tokens)
	exportModule := handler.NewExport(guestRepo, weddingRepo, tokens)
	deviceModule := handler.NewDevice(deviceRepo, tokens)

	r := router.New(db, log, cfg.IsProduction(),
		authModule, weddingModule, guestModule,
		activityModule, statsModule, notifModule, exportModule,
		deviceModule,
	)

	// Swagger UI at /swagger/index.html. Off in production unless ENABLE_SWAGGER=true,
	// so the API surface isn't advertised on public deployments by default.
	if !cfg.IsProduction() || os.Getenv("ENABLE_SWAGGER") == "true" {
		r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
		log.Info("swagger UI enabled", "url", "/swagger/index.html")
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Serve in the background so we can wait for a shutdown signal.
	go func() {
		log.Info("server starting", "port", cfg.Port, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for SIGINT/SIGTERM, then drain in-flight requests.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutdown signal received, draining")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("graceful shutdown failed", "error", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
	log.Info("stopped")
}
