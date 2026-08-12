// Package app is the composition root: it opens every external connection,
// builds every slice, and returns something main can run and shut down.
//
// The wiring is written out by hand and read top to bottom — infrastructure,
// then middleware, then slices, then the router. A missing dependency is a
// compile error here rather than a panic at boot, and the order things are
// created in is the order they appear.
package app

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	lessonpdf "github.com/memberclass-backend-golang/internal/features/admin/lesson_pdf"
	"github.com/memberclass-backend-golang/internal/features/admin/member_import"
	"github.com/memberclass-backend-golang/internal/features/api/activity_summary"
	aifeat "github.com/memberclass-backend-golang/internal/features/api/ai"
	authfeat "github.com/memberclass-backend-golang/internal/features/api/auth"
	commentfeat "github.com/memberclass-backend-golang/internal/features/api/comment"
	healthfeat "github.com/memberclass-backend-golang/internal/features/api/health"
	socialfeat "github.com/memberclass-backend-golang/internal/features/api/social"
	ssofeat "github.com/memberclass-backend-golang/internal/features/api/sso"
	studentfeat "github.com/memberclass-backend-golang/internal/features/api/student"
	userfeat "github.com/memberclass-backend-golang/internal/features/api/user"
	"github.com/memberclass-backend-golang/internal/features/api/user_activities"
	videofeat "github.com/memberclass-backend-golang/internal/features/api/video"
	vitrinefeat "github.com/memberclass-backend-golang/internal/features/api/vitrine"
	"github.com/memberclass-backend-golang/internal/features/workers/analytics"
	notificationsworker "github.com/memberclass-backend-golang/internal/features/workers/notifications"
	transcriptionworker "github.com/memberclass-backend-golang/internal/features/workers/transcription"
	"github.com/memberclass-backend-golang/internal/platform/bunny"
	"github.com/memberclass-backend-golang/internal/platform/cache"
	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/memberclass-backend-golang/internal/platform/database"
	"github.com/memberclass-backend-golang/internal/platform/ilovepdf"
	"github.com/memberclass-backend-golang/internal/platform/logger"
	"github.com/memberclass-backend-golang/internal/platform/ratelimit"
	"github.com/memberclass-backend-golang/internal/platform/resend"
	"github.com/memberclass-backend-golang/internal/platform/storage"

	"github.com/memberclass-backend-golang/internal/features/api/docs"
	mw "github.com/memberclass-backend-golang/internal/shared/middleware"
)

// shutdownTimeout bounds the whole graceful shutdown, from the first worker
// stop to the last connection close.
const shutdownTimeout = 30 * time.Second

// App is a fully wired service. Run serves until the context is cancelled,
// then shuts everything down in reverse order of startup.
type App struct {
	cfg *config.Config
	log logger.Logger

	db    *sql.DB
	txDB  database.TranscriptionDB
	cache cache.Cache

	router    *Router
	scheduler *analytics.Scheduler

	memberImport  *member_import.Feature
	notifications *notificationsworker.Feature
	transcription *transcriptionworker.Feature
}

// New opens every connection and builds every slice. The caller owns the
// returned App and must call Run, which handles shutdown.
//
// The logger is passed in rather than built here: main needs one before this
// point, to report a telemetry setup that failed, and building a second would
// reinstall the global slog handler behind the first one's back.
func New(cfg *config.Config, log logger.Logger) (*App, error) {
	// One line per feature this deployment is running without. Missing
	// optional variables are legitimate, but they must be visible: a silent
	// warning is how a customer ends up with transcription switched off for a
	// week before anyone notices.
	for _, warning := range cfg.Warnings() {
		log.Warn(warning)
	}

	// ---------- infrastructure ----------

	db, err := database.Open(cfg, log)
	if err != nil {
		return nil, err
	}

	// Optional: a deployment without transcription still serves the API.
	txDB := database.OpenTranscription(cfg, log)

	// The pgvector schema ships with the binary, so a fresh deployment does
	// not need anyone to remember to run four SQL files by hand.
	if err := database.MigrateTranscription(context.Background(), txDB.DB, log); err != nil {
		return nil, fmt.Errorf("transcription migrations: %w", err)
	}

	redis, err := cache.NewRedisCache(cfg, log)
	if err != nil {
		return nil, fmt.Errorf("cache: %w", err)
	}

	spaces, err := storage.NewDigitalOceanSpaces(cfg, log)
	if err != nil {
		return nil, fmt.Errorf("storage: %w", err)
	}

	bunnySvc := bunny.NewBunnyService(cfg, log)
	resendSvc := resend.New(cfg, log)

	// PDF processing is optional. A deployment without iLovePDF keys boots
	// with the endpoints returning an error rather than failing to start.
	var pdfSvc ilovepdf.Service
	if cfg.IlovePDF.Enabled {
		pdfSvc, err = ilovepdf.NewIlovePdfService(cfg, log, redis)
		if err != nil {
			return nil, fmt.Errorf("ilovepdf: %w", err)
		}
	}

	// ---------- middleware ----------

	rateLimitUpload := mw.NewRateLimitMiddleware(ratelimit.NewRateLimiterUpload(redis, log), log)
	rateLimitTenant := mw.NewRateLimitTenantMiddleware(ratelimit.NewRateLimiterTenant(redis, log), log)
	rateLimitIP := mw.NewRateLimitIPMiddleware(ratelimit.NewRateLimiterIP(redis, log), log)

	authSession := mw.NewAuthMiddleware(db, cfg, log)
	authExternal := mw.NewAuthExternalMiddleware(db, log)
	authBearer := mw.NewBearerMiddleware(cfg, log)

	// ---------- slices ----------

	memberImport := member_import.New(db, log, resendSvc)
	notifications := notificationsworker.New(db, log)
	transcription := transcriptionworker.New(txDB.DB, db, log, bunnySvc)

	r := newRouter(
		log,
		videofeat.New(db, bunnySvc, log),
		lessonpdf.New(db, pdfSvc, spaces, cfg, log),
		commentfeat.New(db, log),
		user_activities.New(db, redis, log),
		userfeat.New(db, redis, log),
		socialfeat.New(db, log),
		activity_summary.New(db, redis, log),
		memberImport,
		transcription,
		studentfeat.New(db, redis, log),
		docs.New(),
		authfeat.New(db, redis, cfg, log),
		ssofeat.New(db, log),
		aifeat.New(db, cfg, log),
		vitrinefeat.New(db, log),
		healthfeat.New(db, redis, log),
		rateLimitUpload,
		rateLimitTenant,
		rateLimitIP,
		authSession,
		authExternal,
		authBearer,
	)
	r.SetupRoutes()

	// ---------- scheduled work ----------

	scheduler := analytics.NewScheduler(log)

	// Cron expressions carry seconds: the scheduler uses WithSeconds().
	if err := scheduler.AddJob(analytics.NewDailyRollupJob(db, log), "0 0 8 * * *"); err != nil {
		return nil, fmt.Errorf("analytics.daily_rollup: %w", err)
	}
	if err := scheduler.AddJob(analytics.NewMonthlyRollupJob(db, log, cfg.Analytics.DeleteEnabled), "0 0 9 1 * *"); err != nil {
		return nil, fmt.Errorf("analytics.monthly_rollup: %w", err)
	}

	return &App{
		cfg:           cfg,
		log:           log,
		db:            db,
		txDB:          txDB,
		cache:         redis,
		router:        r,
		scheduler:     scheduler,
		memberImport:  memberImport,
		notifications: notifications,
		transcription: transcription,
	}, nil
}

// Run starts the background workers and the HTTP server, then blocks until ctx
// is cancelled. On cancellation it shuts everything down and returns once the
// last connection is closed.
func (a *App) Run(ctx context.Context) error {
	// Each worker owns a goroutine pool tied to this context, so cancelling it
	// is what stops them.
	workerCtx, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()

	a.scheduler.Start()

	// Clear imports left in "processing" by a previous crash, then start the
	// retention job that trims UserImportRow after 24h.
	member_import.StartupReset(a.db, a.log)
	member_import.StartRetentionJob(workerCtx, a.db, a.log)

	a.notifications.Start(workerCtx)
	a.notifications.StartCleanupJob(workerCtx)
	a.transcription.Start(workerCtx)

	server := &http.Server{
		Addr:    ":" + a.cfg.App.Port,
		Handler: a.router,
	}

	serverErr := make(chan error, 1)
	go func() {
		a.log.Info("Application started successfully")
		a.log.Info("Server running on :" + a.cfg.App.Port)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
	}

	return a.shutdown(server, stopWorkers)
}

// shutdown stops the workers before closing the connections they use: an
// in-flight query against a closed *sql.DB panics.
func (a *App) shutdown(server *http.Server, stopWorkers context.CancelFunc) error {
	a.log.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	a.scheduler.Stop()

	a.notifications.Stop(10 * time.Second)
	a.transcription.Stop(15 * time.Second)
	stopWorkers()

	if err := server.Shutdown(ctx); err != nil {
		a.log.Error("Server forced to shutdown: " + err.Error())
	}

	// Drain in-flight member imports so their UserImport rows reach a terminal
	// state before the database closes. Bounded by the same deadline;
	// stragglers are recovered by StartupReset on the next boot.
	a.memberImport.Wait(ctx)

	if err := a.cache.Close(); err != nil {
		a.log.Error("Error closing cache: " + err.Error())
	}
	if a.txDB.DB != nil {
		if err := a.txDB.Close(); err != nil {
			a.log.Error("Error closing transcription database: " + err.Error())
		}
	}
	if err := a.db.Close(); err != nil {
		a.log.Error("Error closing database: " + err.Error())
	}

	a.log.Info("Server exited")
	return nil
}
