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
	apikeyusageworker "github.com/memberclass-backend-golang/internal/features/workers/api_key_usage"
	bunnyusageworker "github.com/memberclass-backend-golang/internal/features/workers/bunny_usage"
	notificationsworker "github.com/memberclass-backend-golang/internal/features/workers/notifications"
	transcriptionworker "github.com/memberclass-backend-golang/internal/features/workers/transcription"
	"github.com/memberclass-backend-golang/internal/platform/apikeyusage"
	"github.com/memberclass-backend-golang/internal/platform/bunny"
	"github.com/memberclass-backend-golang/internal/platform/cache"
	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/memberclass-backend-golang/internal/platform/database"
	"github.com/memberclass-backend-golang/internal/platform/ilovepdf"
	"github.com/memberclass-backend-golang/internal/platform/logger"
	"github.com/memberclass-backend-golang/internal/platform/ratelimit"
	"github.com/memberclass-backend-golang/internal/platform/resend"
	"github.com/memberclass-backend-golang/internal/platform/storage"
	"github.com/memberclass-backend-golang/internal/platform/telemetry"

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

	// The account-level client is a second client on a second host with a
	// second credential: the Stream API key a tenant holds cannot read a
	// library's usage or a pull zone's statistics.
	bunnyAccount := bunny.NewAccountService(cfg, log)

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

	// Named tenant API keys live in "TenantApiKey"; a deployment where nobody
	// has created one yet still authenticates, through the legacy fallback, and
	// gets no usage panel. See the function comment.
	warnIfLegacyAPIKeysRemain(context.Background(), db, log)

	usageRecorder := apikeyusage.New(redis, log)
	authExternal := mw.NewAuthExternalMiddleware(db, log, usageRecorder)
	authBearer := mw.NewBearerMiddleware(cfg, redis, log)

	// ---------- slices ----------

	memberImport := member_import.New(db, log, resendSvc, cfg)
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
		docs.New(cfg, log),
		authfeat.New(db, redis, cfg, log),
		ssofeat.New(db, log),
		aifeat.New(db, cfg, log),
		vitrinefeat.New(db, log),
		healthfeat.New(db, redis, log),
		rateLimitUpload,
		rateLimitTenant,
		rateLimitIP,
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

	// Hourly, at five past. The offset keeps it off the hour, where the
	// rollups and every other cron in the fleet are, and the day's first run
	// at 00:05 UTC is the one that closes the previous day out. The instance
	// id is the lock owner: every replica runs this scheduler, and only one of
	// them may drain a day's counters.
	usageFlush := apikeyusageworker.New(db, redis, log, telemetry.ServiceInstanceID(cfg))
	if err := scheduler.AddJob(usageFlush, "0 5 * * * *"); err != nil {
		return nil, fmt.Errorf("api_key_usage.flush: %w", err)
	}

	// Bunny usage, once a day at 05:30 UTC. The hour does not matter for the
	// current month — traffic is a running total and storage is a sample — and
	// it does not matter for a finished one either, because the closing pass
	// reads /statistics over the closed period rather than the library counter
	// Bunny resets at midnight UTC on the 1st.
	//
	// Gated on the account key: without it every call would answer 401, and a
	// deployment that never had one would take a failing job's alert every day
	// for a feature it does not run.
	if cfg.Bunny.UsageEnabled {
		bunnyUsage := bunnyusageworker.New(db, bunnyAccount, redis, log, telemetry.ServiceInstanceID(cfg))
		if err := scheduler.AddJob(bunnyUsage, "0 30 5 * * *"); err != nil {
			return nil, fmt.Errorf("bunny_usage.sync: %w", err)
		}
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

// sqlLegacyKeyCheck answers, in one round trip, whether every tenant on this
// deployment is still on the single pre-"TenantApiKey" key.
const sqlLegacyKeyCheck = `
	SELECT
		(SELECT count(*) FROM "TenantApiKey"),
		(SELECT count(*) FROM "Tenant" WHERE token_api_auth IS NOT NULL)
`

// warnIfLegacyAPIKeysRemain says so at boot when the new table is empty and the
// old column is not.
//
// Nothing migrates a legacy key: the panel does not copy token_api_auth into
// "TenantApiKey", so a tenant leaves the old column behind only by creating a
// named key there. Until it does, it authenticates through the fallback, and
// the symptom is not an outage but a silence: no key id, so the usage panel
// stays empty, and no expiry, so nothing in the panel can retire that key.
// Neither is visible from the outside, which is why this is said at boot.
//
// It warns rather than aborts: a customer created after this shipped has both
// counts at zero legitimately, and refusing to boot would take that deployment
// down for being new.
//
// It goes with the fallback itself — see issue #38.
func warnIfLegacyAPIKeysRemain(ctx context.Context, db *sql.DB, log logger.Logger) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var newKeys, legacyKeys int
	if err := db.QueryRowContext(ctx, sqlLegacyKeyCheck).Scan(&newKeys, &legacyKeys); err != nil {
		log.Warn("Could not verify tenant API key migration: " + err.Error())
		return
	}

	if newKeys == 0 && legacyKeys > 0 {
		log.Warn(fmt.Sprintf(
			"Tenant API keys not migrated: \"TenantApiKey\" is empty while %d tenant(s) still hold token_api_auth. "+
				"This deployment authenticates through the legacy fallback, which no expiry and no usage "+
				"panel reaches; create named keys in the panel to leave it.",
			legacyKeys,
		))
	}
}
