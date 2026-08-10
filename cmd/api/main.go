package main

import (
	"context"
	"database/sql"
	"fmt"
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
	user4 "github.com/memberclass-backend-golang/internal/application/handlers/http/user"
	purchase2 "github.com/memberclass-backend-golang/internal/application/handlers/http/user/purchase"
	"github.com/memberclass-backend-golang/internal/application/handlers/http/video"
	"github.com/memberclass-backend-golang/internal/application/jobs"
	analyticsjobs "github.com/memberclass-backend-golang/internal/application/jobs/analytics"
	auth3 "github.com/memberclass-backend-golang/internal/application/middlewares/auth"
	"github.com/memberclass-backend-golang/internal/application/middlewares/rate_limit"
	"github.com/memberclass-backend-golang/internal/application/router"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/memberclass-backend-golang/internal/domain/ports/ai"
	bunnyport "github.com/memberclass-backend-golang/internal/domain/ports/bunny"
	comment2 "github.com/memberclass-backend-golang/internal/domain/ports/comment"
	sso3 "github.com/memberclass-backend-golang/internal/domain/ports/sso"
	tenant2 "github.com/memberclass-backend-golang/internal/domain/ports/tenant"
	user2 "github.com/memberclass-backend-golang/internal/domain/ports/user"
	ai2 "github.com/memberclass-backend-golang/internal/domain/usecases/ai"
	"github.com/memberclass-backend-golang/internal/domain/usecases/auth"
	bunny2 "github.com/memberclass-backend-golang/internal/domain/usecases/bunny"
	comment3 "github.com/memberclass-backend-golang/internal/domain/usecases/comment"
	"github.com/memberclass-backend-golang/internal/domain/usecases/lessons"
	sso4 "github.com/memberclass-backend-golang/internal/domain/usecases/sso"
	user3 "github.com/memberclass-backend-golang/internal/domain/usecases/user"
	"github.com/memberclass-backend-golang/internal/features/admin/member_import"
	"github.com/memberclass-backend-golang/internal/features/api/activity_summary"
	studentfeat "github.com/memberclass-backend-golang/internal/features/api/student"
	"github.com/memberclass-backend-golang/internal/features/api/user_activities"
	vitrinefeat "github.com/memberclass-backend-golang/internal/features/api/vitrine"
	notificationsworker "github.com/memberclass-backend-golang/internal/features/workers/notifications"
	transcriptionworker "github.com/memberclass-backend-golang/internal/features/workers/transcription"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/external_services/bunny"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/external_services/ilovepdf"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/external_services/resend"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/comment"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/lesson"
	sso_repository "github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/sso"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/tenant"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/topic"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/repository/user"
	"github.com/memberclass-backend-golang/internal/platform/cache"
	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/memberclass-backend-golang/internal/platform/database"
	"github.com/memberclass-backend-golang/internal/platform/logger"
	"github.com/memberclass-backend-golang/internal/platform/ratelimit"
	"github.com/memberclass-backend-golang/internal/platform/storage"
	"go.uber.org/fx"
)

func main() {
	_ = godotenv.Load()

	// Config is resolved before anything else so a deployment with a missing
	// variable dies here, naming every offender at once, instead of booting
	// into a half-working state. Logging is not up yet, hence stderr.
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid configuration: %v\n", err)
		os.Exit(1)
	}

	fx.New(
		fx.Supply(cfg),
		fx.Provide(
			logger.NewLogger,
			database.Open,
			database.OpenTranscription,
			database.NewMigrationService,
			cache.NewRedisCache,
			storage.NewDigitalOceanSpaces,

			tenant.NewTenantRepository,
			user.NewUserRepository,
			lesson.NewLessonRepository,
			comment.NewCommentRepository,
			comment.NewSocialCommentRepository,
			topic.NewTopicRepository,
			sso_repository.NewSSORepository,

			ratelimit.NewRateLimiterUpload,
			ratelimit.NewRateLimiterTenant,
			ratelimit.NewRateLimiterIP,
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
			studentfeat.New,
			vitrinefeat.New,
			user_activities.New,
			member_import.New,
			notificationsworker.New,
			// Transcription slice owns the entire pipeline (Bunny → Whisper →
			// chunk → embed → Railway pgvector). It reads the tenant database
			// for lesson metadata and writes vectors to its own. No cron — the
			// internal admin UI POSTs the explicit list of lessonIds.
			func(txDB database.TranscriptionDB, db *sql.DB, log ports.Logger, bunnySvc bunnyport.BunnyService) *transcriptionworker.Feature {
				return transcriptionworker.New(txDB.DB, db, log, bunnySvc)
			},
			lessons.NewLessonsCompletedUseCase,
			auth.NewAuthUseCase,
			ai2.NewAILessonUseCase,
			func(tenantRepo tenant2.TenantRepository, logger ports.Logger) ai.AITenantUseCase {
				return ai2.NewAITenantUseCase(tenantRepo, logger)
			},
			func(ssoRepo sso3.SSORepository, userRepo user2.UserRepository, logger ports.Logger) sso3.SSOUseCase {
				return sso4.NewSSOUseCase(ssoRepo, userRepo, logger)
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
			internalhttp.NewSwaggerHandler,
			auth2.NewAuthHandler,
			sso2.NewSSOHandler,
			ai3.NewAILessonHandler,
			ai3.NewAITenantHandler,

			router.NewRouter,
			jobs.NewScheduler,
		),
		fx.Invoke(startApplication),
	)

}

func startApplication(
	log ports.Logger,
	cfg *config.Config,
	db *sql.DB,
	txDB database.TranscriptionDB,
	cache ports.Cache,
	migrationService *database.MigrationService,
	router *router.Router,
	scheduler *jobs.Scheduler,
	transcriptionFeat *transcriptionworker.Feature,
	memberImport *member_import.Feature,
	notifWorker *notificationsworker.Feature,
) {
	// One line per feature this deployment is running without. Missing
	// optional variables are legitimate, but they must be visible: a silent
	// warning is how a customer ends up with transcription switched off for
	// a week before anyone notices.
	for _, warning := range cfg.Warnings() {
		log.Warn(warning)
	}

	router.SetupRoutes()

	// Analytics rollup jobs. Scheduler uses WithSeconds() (6 fields).
	if err := scheduler.AddJob(analyticsjobs.NewDailyRollupJob(db, log), "0 0 8 * * *"); err != nil {
		log.Error("failed to register analytics.daily_rollup", "err", err.Error())
	}
	if err := scheduler.AddJob(analyticsjobs.NewMonthlyRollupJob(db, log), "0 0 9 1 * *"); err != nil {
		log.Error("failed to register analytics.monthly_rollup", "err", err.Error())
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

	// Transcription worker: poll the Railway pgvector jobs table, process
	// VIDEO_PROCESSING jobs end-to-end (Bunny → Whisper → chunk → embed →
	// pgvector). Started here so freshly enqueued lessons begin processing
	// as soon as the HTTP server is up.
	txCtx, stopTxWorker := context.WithCancel(context.Background())
	defer stopTxWorker()
	transcriptionFeat.Start(txCtx)

	server := &http.Server{
		Addr:    ":" + cfg.App.Port,
		Handler: router,
	}

	go func() {
		log.Info("Application started successfully")
		log.Info("Server running on :" + cfg.App.Port)

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
	// Order matters: stop every worker that owns a goroutine pool BEFORE
	// dbMap.CloseAll() runs — an in-flight query against a closed *sql.DB
	// panics.
	notifWorker.Stop(10 * time.Second)
	stopNotifWorker()
	transcriptionFeat.Stop(15 * time.Second)
	stopTxWorker()

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

	if txDB.DB != nil {
		if err := txDB.Close(); err != nil {
			log.Error("Error closing transcription database: " + err.Error())
		}
	}

	if err := db.Close(); err != nil {
		log.Error("Error closing database: " + err.Error())
	}

	log.Info("Server exited")
}
