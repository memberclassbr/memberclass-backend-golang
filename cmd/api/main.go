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
	ai3 "github.com/memberclass-backend-golang/internal/application/handlers/http/ai"
	auth2 "github.com/memberclass-backend-golang/internal/application/handlers/http/auth"
	comment4 "github.com/memberclass-backend-golang/internal/application/handlers/http/comment"
	lesson2 "github.com/memberclass-backend-golang/internal/application/handlers/http/lesson"
	sso2 "github.com/memberclass-backend-golang/internal/application/handlers/http/sso"
	student2 "github.com/memberclass-backend-golang/internal/application/handlers/http/student"
	user4 "github.com/memberclass-backend-golang/internal/application/handlers/http/user"
	purchase2 "github.com/memberclass-backend-golang/internal/application/handlers/http/user/purchase"
	"github.com/memberclass-backend-golang/internal/application/handlers/http/video"
	vitrine4 "github.com/memberclass-backend-golang/internal/application/handlers/http/vitrine"
	"github.com/memberclass-backend-golang/internal/application/jobs"
	"github.com/memberclass-backend-golang/internal/application/jobs/transcription"
	auth3 "github.com/memberclass-backend-golang/internal/application/middlewares/auth"
	"github.com/memberclass-backend-golang/internal/application/middlewares/rate_limit"
	"github.com/memberclass-backend-golang/internal/application/router"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/memberclass-backend-golang/internal/domain/ports/ai"
	comment2 "github.com/memberclass-backend-golang/internal/domain/ports/comment"
	sso3 "github.com/memberclass-backend-golang/internal/domain/ports/sso"
	tenant2 "github.com/memberclass-backend-golang/internal/domain/ports/tenant"
	user2 "github.com/memberclass-backend-golang/internal/domain/ports/user"
	vitrine2 "github.com/memberclass-backend-golang/internal/domain/ports/vitrine"
	ai2 "github.com/memberclass-backend-golang/internal/domain/usecases/ai"
	"github.com/memberclass-backend-golang/internal/domain/usecases/auth"
	bunny2 "github.com/memberclass-backend-golang/internal/domain/usecases/bunny"
	comment3 "github.com/memberclass-backend-golang/internal/domain/usecases/comment"
	"github.com/memberclass-backend-golang/internal/domain/usecases/lessons"
	sso4 "github.com/memberclass-backend-golang/internal/domain/usecases/sso"
	"github.com/memberclass-backend-golang/internal/domain/usecases/student"
	user3 "github.com/memberclass-backend-golang/internal/domain/usecases/user"
	vitrine3 "github.com/memberclass-backend-golang/internal/domain/usecases/vitrine"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/cache"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/database"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/external_services/bunny"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/external_services/ilovepdf"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/logger"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/rate_limiter"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/comment"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/lesson"
	sso_repository "github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/sso"
	student_report "github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/student_report"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/tenant"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/topic"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/user"
	user_activity "github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/user_activity"
	vitrine_repository "github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/vitrine"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/storage"
	"go.uber.org/fx"
)

func main() {

	fx.New(
		fx.Provide(
			logger.NewLogger,
			database.NewDB,
			database.NewMigrationService,
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
			sso_repository.NewSSORepository,
			vitrine_repository.NewVitrineRepository,

			rate_limiter.NewRateLimiterUpload,
			rate_limiter.NewRateLimiterTenant,
			rate_limiter.NewRateLimiterIP,
			ilovepdf.NewIlovePdfService,
			bunny.NewBunnyService,

			user3.NewValidateSessionUseCase,
			lessons.NewPdfProcessorUseCase,
			bunny2.NewTenantGetTenantBunnyCredentialsUseCase,
			bunny2.NewUploadVideoBunnyCdnUseCase,
			func(logger ports.Logger, commentRepo comment2.CommentRepository, userRepo user2.UserRepository) comment2.CommentUseCase {
				return comment3.NewCommentUseCase(logger, commentRepo, userRepo)
			},
			auth.NewApiTokenTenantUseCase,
			user3.NewUserActivityUseCase,
			user3.NewUserPurchaseUseCase,
			user3.NewUserInformationsUseCase,
			comment3.NewSocialCommentUseCase,
			user3.NewActivitySummaryUseCase,
			lessons.NewLessonsCompletedUseCase,
			student.NewStudentReportUseCase,
			auth.NewAuthUseCase,
			ai2.NewAILessonUseCase,
			func(tenantRepo tenant2.TenantRepository, aiLessonUseCase ai.AILessonUseCase, logger ports.Logger) ai.AITenantUseCase {
				return ai2.NewAITenantUseCase(tenantRepo, aiLessonUseCase, logger)
			},
			func(ssoRepo sso3.SSORepository, userRepo user2.UserRepository, logger ports.Logger) sso3.SSOUseCase {
				return sso4.NewSSOUseCase(ssoRepo, userRepo, logger)
			},
			func(vitrineRepo vitrine2.VitrineRepository) vitrine2.VitrineUseCase {
				return vitrine3.NewVitrineUseCase(vitrineRepo)
			},

			rate_limit.NewRateLimitMiddleware,
			rate_limit.NewRateLimitTenantMiddleware,
			rate_limit.NewRateLimitIPMiddleware,
			auth3.NewAuthMiddleware,
			auth3.NewAuthExternalMiddleware,

			lesson2.NewLessonHandler,
			video.NewVideoHandler,
			comment4.NewCommentHandler,
			user4.NewUserActivityHandler,
			purchase2.NewUserPurchaseHandler,
			user4.NewUserInformationsHandler,
			comment4.NewSocialCommentHandler,
			user4.NewActivitySummaryHandler,
			lesson2.NewLessonsCompletedHandler,
			student2.NewStudentReportHandler,
			internalhttp.NewSwaggerHandler,
			auth2.NewAuthHandler,
			sso2.NewSSOHandler,
			ai3.NewAILessonHandler,
			ai3.NewAITenantHandler,
			vitrine4.NewVitrineHandler,

			router.NewRouter,
			jobs.NewScheduler,
			transcription.NewTranscriptionJob,
		),
		fx.Invoke(startApplication),
	)

}

func startApplication(
	log ports.Logger,
	db *sql.DB,
	cache ports.Cache,
	migrationService *database.MigrationService,
	router *router.Router,
	scheduler *jobs.Scheduler,
	transcriptionJob *transcription.TranscriptionJob,
) {
	router.SetupRoutes()

	if err := jobs.InitJobs(scheduler, transcriptionJob); err != nil {
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
