package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	internalhttp "github.com/memberclass-backend-golang/internal/application/handlers/http"
	"github.com/memberclass-backend-golang/internal/application/router"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/memberclass-backend-golang/internal/domain/usecases"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/cache"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/database"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/external_services/bunny"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/logger"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/rate_limiter"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/tenant"
	"go.uber.org/fx"
)

func main() {

	fx.New(
		fx.Provide(
			logger.NewLogger,
			database.NewDB,
			cache.NewRedisCache,
			rate_limiter.NewRateLimiterUpload,
			tenant.NewTenantRepository,
			bunny.NewBunnyService,
			usecases.NewTenantGetTenantBunnyCredentialsUseCase,
			usecases.NewUploadVideoBunnyCdnUseCase,
			internalhttp.NewVideoHandler,
			router.NewRouter,
		),
		fx.Invoke(startApplication),
	)

}

func startApplication(log ports.Logger, db *sql.DB, cache ports.Cache, router *router.Router) {
	router.SetupRoutes()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8181"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	go func() {
		log.Info("Application started successfully")
		log.Info("Server running on :" + port)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("Failed to start server: " + err.Error())
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Error("Server forced to shutdown: " + err.Error())
	}

	if err := cache.Close(); err != nil {
		log.Error("Error closing cache: " + err.Error())
	}

	if err := db.Close(); err != nil {
		log.Error("Error closing database: " + err.Error())
	}

	log.Info("Server exited")
}
