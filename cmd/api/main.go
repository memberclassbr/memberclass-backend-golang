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
	"github.com/memberclass-backend-golang/internal/application/jobs"
	"github.com/memberclass-backend-golang/internal/application/jobs/transcription"
	"github.com/memberclass-backend-golang/internal/application/middlewares"
	"github.com/memberclass-backend-golang/internal/application/router"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/memberclass-backend-golang/internal/domain/usecases"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/cache"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/database"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/external_services/bunny"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/external_services/ilovepdf"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/logger"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/rate_limiter"
		"github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/comment"
		"github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/lesson"
		"github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/topic"
		"github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/tenant"
		"github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/user"
		user_activity "github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/user_activity"
		student_report "github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/student_report"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/storage"
	"go.uber.org/fx"
)

func main() {

	fx.New(
		fx.Provide(
			logger.NewLogger,
			database.NewDB,
			cache.NewRedisCache,
			storage.NewDigitalOceanSpaces,

			tenant.NewTenantRepository,
			user.NewUserRepository,
			lesson.NewLessonRepository,
			comment.NewCommentRepository,
			comment.NewSocialCommentRepository,
			topic.NewTopicRepository,
			user_activity.NewUserActivityRepository,
			student_report.NewStudentReportRepository,

			rate_limiter.NewRateLimiterUpload,
			rate_limiter.NewRateLimiterTenant,
			rate_limiter.NewRateLimiterIP,
			ilovepdf.NewIlovePdfService,
			bunny.NewBunnyService,

			usecases.NewValidateSessionUseCase,
			usecases.NewPdfProcessorUseCase,
			usecases.NewTenantGetTenantBunnyCredentialsUseCase,
			usecases.NewUploadVideoBunnyCdnUseCase,
			func(logger ports.Logger, commentRepo ports.CommentRepository, userRepo ports.UserRepository) ports.CommentUseCase {
				return usecases.NewCommentUseCase(logger, commentRepo, userRepo)
			},
			usecases.NewApiTokenTenantUseCase,
			usecases.NewUserActivityUseCase,
			usecases.NewUserPurchaseUseCase,
			usecases.NewUserInformationsUseCase,
			usecases.NewSocialCommentUseCase,
			usecases.NewActivitySummaryUseCase,
			usecases.NewLessonsCompletedUseCase,
			usecases.NewStudentReportUseCase,
			usecases.NewAuthUseCase,
			usecases.NewAILessonUseCase,
			usecases.NewAITenantUseCase,

			middlewares.NewRateLimitMiddleware,
			middlewares.NewRateLimitTenantMiddleware,
			middlewares.NewRateLimitIPMiddleware,
			middlewares.NewAuthMiddleware,
			middlewares.NewAuthExternalMiddleware,

			internalhttp.NewLessonHandler,
			internalhttp.NewVideoHandler,
			internalhttp.NewCommentHandler,
			internalhttp.NewUserActivityHandler,
			internalhttp.NewUserPurchaseHandler,
			internalhttp.NewUserInformationsHandler,
			internalhttp.NewSocialCommentHandler,
			internalhttp.NewActivitySummaryHandler,
			internalhttp.NewLessonsCompletedHandler,
			internalhttp.NewStudentReportHandler,
			internalhttp.NewSwaggerHandler,
			internalhttp.NewAuthHandler,
			internalhttp.NewAILessonHandler,
			internalhttp.NewAITenantHandler,

			router.NewRouter,
			jobs.NewScheduler,
			transcription.NewTranscriptionJob,
			transcription.NewTranscriptionStatusCheckerJob,
		),
		fx.Invoke(startApplication),
	)

}

func startApplication(
	log ports.Logger,
	db *sql.DB,
	cache ports.Cache,
	router *router.Router,
	scheduler *jobs.Scheduler,
	transcriptionJob *transcription.TranscriptionJob,
	statusCheckerJob *transcription.TranscriptionStatusCheckerJob,
) {
	router.SetupRoutes()

	if err := jobs.InitJobs(scheduler, transcriptionJob, statusCheckerJob); err != nil {
		log.Error("Error initializing jobs: " + err.Error())
	}

	scheduler.Start()

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

	scheduler.Stop()

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
