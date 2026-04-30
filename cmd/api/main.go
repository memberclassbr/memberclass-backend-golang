package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
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
	"github.com/memberclass-backend-golang/internal/features/api/activity_summary"
	"github.com/memberclass-backend-golang/internal/features/admin/member_import"
	"github.com/memberclass-backend-golang/internal/features/api/user_activities"
	notificationsworker "github.com/memberclass-backend-golang/internal/features/workers/notifications"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/cache"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/database"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/external_services/bunny"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/external_services/ilovepdf"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/external_services/resend"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/logger"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/rate_limiter"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/comment"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/lesson"
	sso_repository "github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/sso"
	student_report "github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/student_report"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/tenant"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/topic"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/user"
	vitrine_repository "github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/vitrine"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/storage"
	"go.uber.org/fx"
)

func main() {
	_ = godotenv.Load()

	fx.New(
		fx.Provide(
			logger.NewLogger,
			database.NewMultiDB,
			database.DefaultDB,
			database.NewMigrationService,
			cache.NewRedisCache,
			storage.NewDigitalOceanSpaces,

			tenant.NewTenantRepository,
			user.NewUserRepository,
			lesson.NewLessonRepoResolver,
			lesson.NewLessonRepository,
			comment.NewCommentRepository,
			comment.NewSocialCommentRepository,
			topic.NewTopicRepository,
			student_report.NewStudentReportRepository,
			sso_repository.NewSSORepository,
			vitrine_repository.NewVitrineRepository,

			rate_limiter.NewRateLimiterUpload,
			rate_limiter.NewRateLimiterTenant,
			rate_limiter.NewRateLimiterIP,
			ilovepdf.NewIlovePdfService,
			bunny.NewBunnyService,
			resend.New,

			user3.NewValidateSessionUseCase,
			lessons.NewPdfProcessorUseCase,
			bunny2.NewTenantGetTenantBunnyCredentialsUseCase,
			bunny2.NewUploadVideoBunnyCdnUseCase,
			func(logger ports.Logger, commentRepo comment2.CommentRepository, userRepo user2.UserRepository) comment2.CommentUseCase {
				return comment3.NewCommentUseCase(logger, commentRepo, userRepo)
			},
			auth.NewApiTokenTenantUseCase,
			user3.NewUserPurchaseUseCase,
			user3.NewUserInformationsUseCase,
			comment3.NewSocialCommentUseCase,
			activity_summary.New,
			user_activities.New,
			member_import.New,
			notificationsworker.New,
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
			auth3.NewBearerMiddleware,

			lesson2.NewLessonHandler,
			video.NewVideoHandler,
			comment4.NewCommentHandler,
			purchase2.NewUserPurchaseHandler,
			user4.NewUserInformationsHandler,
			comment4.NewSocialCommentHandler,
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
	dbMap database.DBMap,
	db *sql.DB,
	cache ports.Cache,
	migrationService *database.MigrationService,
	router *router.Router,
	scheduler *jobs.Scheduler,
	transcriptionJob *transcription.TranscriptionJob,
	memberImport *member_import.Feature,
	notifWorker *notificationsworker.Feature,
) {
	router.SetupRoutes()

	if err := jobs.InitJobs(scheduler, transcriptionJob); err != nil {
		log.Error("Error initializing jobs: " + err.Error())
	}

	scheduler.Start()

	// Member-import slice: clear orphaned "processing" imports on startup,
	// then kick off the 24h retention goroutine for UserImportRow.
	member_import.StartupReset(db, log)
	importRetentionCtx, stopImportRetention := context.WithCancel(context.Background())
	defer stopImportRetention()
	member_import.StartRetentionJob(importRetentionCtx, db, log)

	// Notifications worker: poll the Notification table, dispatch FCM pushes,
	// run daily cleanup. Started here so push delivery is live as soon as
	// the HTTP server is.
	notifCtx, stopNotifWorker := context.WithCancel(context.Background())
	defer stopNotifWorker()
	notifWorker.Start(notifCtx)
	notifWorker.StartCleanupJob(notifCtx)

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
	// Order matters: stop the worker run loop, then cancel notifCtx so the
	// long-lived cleanup goroutine bails out — both must finish BEFORE
	// dbMap.CloseAll() below or an in-flight cleanup query hits a closed *sql.DB.
	notifWorker.Stop(10 * time.Second)
	stopNotifWorker()

	if err := server.Shutdown(ctx); err != nil {
		log.Error("Server forced to shutdown: " + err.Error())
	}

	// Drain in-flight member-import workers before the DB closes so their
	// UserImport rows reach a terminal state. Bounded by the same 30s
	// ctx deadline above; stragglers are recovered by StartupReset on the
	// next boot after a 5-min grace.
	memberImport.Wait(ctx)

	if err := cache.Close(); err != nil {
		log.Error("Error closing cache: " + err.Error())
	}

	if err := dbMap.CloseAll(); err != nil {
		log.Error("Error closing databases: " + err.Error())
	}

	log.Info("Server exited")
}
