package main

import (
	"database/sql"
	"net/http"

	internalhttp "github.com/memberclass-backend-golang/internal/application/handlers/http"
	"github.com/memberclass-backend-golang/internal/application/router"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/memberclass-backend-golang/internal/domain/usecases"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/database"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/external_services/bunny"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/logger"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/tenant"
	"go.uber.org/fx"
)

func main() {

	fx.New(
		fx.Provide(
			logger.NewLogger,
			database.NewDB,
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

func startApplication(log ports.Logger, db *sql.DB, router *router.Router) {

	defer db.Close()

	router.SetupRoutes()

	if err := http.ListenAndServe(":8080", router); err != nil {
		log.Error("Failed to start server: " + err.Error())
	}

	log.Info("Application started successfully")
	log.Info("Server running on :8080")
}
