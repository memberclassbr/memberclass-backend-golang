package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	analyticsjobs "github.com/memberclass-backend-golang/internal/features/workers/analytics"
	bunnyusage "github.com/memberclass-backend-golang/internal/features/workers/bunny_usage"
	"github.com/memberclass-backend-golang/internal/platform/bunny"
	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/memberclass-backend-golang/internal/platform/database"
	"github.com/memberclass-backend-golang/internal/platform/logger"
	"github.com/memberclass-backend-golang/internal/platform/telemetry"
)

func nowMinus24() time.Time { return time.Now().UTC().Add(-24 * time.Hour) }
func prevMonthStart() time.Time {
	now := time.Now().UTC()
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	return first.AddDate(0, -1, 0)
}

// main only picks the exit code. Everything else is in run so the deferred
// telemetry flush actually happens: this used to call log.Fatalf on every error
// path, and log.Fatalf calls os.Exit, which skips defers. A backfill that died
// halfway would take its spans with it — exactly the run worth looking at.
func main() {
	os.Exit(run())
}

func run() int {
	_ = godotenv.Load()

	cmd := flag.String("cmd", "daily", "daily|monthly|backfill|backfill-extras|bunny-usage|bunny-backfill|bunny-close")
	from := flag.String("from", "", "YYYY-MM (backfill)")
	to := flag.String("to", "", "YYYY-MM (backfill)")
	tenantId := flag.String("tenantId", "", "scope backfill/daily/monthly/bunny-* to a single tenant (empty = all)")
	skipUserEvent := flag.Bool("skipUserEvent", false, "skip Read fixups + UserEvent migration; only run daily/monthly rollup")
	months := flag.Int("months", bunnyusage.MaxBackfillMonths, "how many months back bunny-backfill reaches (max 12: Bunny keeps one rolling year)")
	month := flag.String("month", "", "YYYY-MM (bunny-close): the closed month to reprocess")
	flag.Parse()

	logr := logger.NewLogger()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		return 1
	}

	shutdownTelemetry, err := telemetry.Init(context.Background(), cfg, logr)
	if err != nil {
		logr.Error("telemetry init failed, continuing uninstrumented: " + err.Error())
		shutdownTelemetry = func(context.Context) error { return nil }
	}
	defer func() {
		if err := shutdownTelemetry(context.Background()); err != nil {
			logr.Error("telemetry shutdown: " + err.Error())
		}
	}()

	db, err := database.Open(cfg, logr)
	if err != nil {
		logr.Error("db: " + err.Error())
		return 1
	}
	defer db.Close()

	// One root span per invocation. Rollups and backfills are the longest-running
	// code in the system and, until now, reported nothing but a final log line.
	attrs := []attribute.KeyValue{attribute.String("analytics.command", *cmd)}
	if *tenantId != "" {
		attrs = append(attrs, attribute.String("analytics.tenant_id", *tenantId))
	}
	ctx, span := otel.Tracer("cmd/analytics").Start(context.Background(), "analytics."+*cmd,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attrs...))
	defer span.End()

	if err := execute(ctx, *cmd, db, logr, cfg, *from, *to, *tenantId, *skipUserEvent, *months, *month); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logr.Error(*cmd + ": " + err.Error())
		return 1
	}

	return 0
}

func execute(
	ctx context.Context,
	cmd string,
	db *sql.DB,
	logr logger.Logger,
	cfg *config.Config,
	from, to, tenantID string,
	skipUserEvent bool,
	months int,
	month string,
) error {
	switch cmd {
	case "daily":
		if tenantID != "" {
			return analyticsjobs.NewDailyRollupJob(db, logr).RunForUTCInstantForTenant(ctx, nowMinus24(), tenantID)
		}
		return analyticsjobs.NewDailyRollupJob(db, logr).Execute(ctx)

	case "monthly":
		if tenantID != "" {
			return analyticsjobs.NewMonthlyRollupJob(db, logr, cfg.Analytics.DeleteEnabled).
				RunForMonthForTenant(ctx, prevMonthStart(), tenantID)
		}
		return analyticsjobs.NewMonthlyRollupJob(db, logr, cfg.Analytics.DeleteEnabled).Execute(ctx)

	case "backfill":
		if from == "" || to == "" {
			return fmt.Errorf("backfill requires --from=YYYY-MM --to=YYYY-MM")
		}
		return analyticsjobs.Backfill(ctx, db, logr, from, to, tenantID, skipUserEvent)

	case "backfill-extras":
		return analyticsjobs.BackfillExtras(ctx, db, logr, tenantID)

	// The three Bunny commands run the same code the scheduler does, with the
	// daily lock granted rather than contended: an operator running one by hand
	// has already decided it should run, and being silently skipped because a
	// replica took today's lock would look exactly like success.
	case "bunny-usage":
		return bunnyJob(cfg, db, logr).Execute(ctx)

	case "bunny-backfill":
		return bunnyJob(cfg, db, logr).Backfill(ctx, months, tenantID)

	case "bunny-close":
		if tenantID == "" || month == "" {
			return fmt.Errorf("bunny-close requires --tenantId and --month=YYYY-MM")
		}
		parsed, err := time.Parse("2006-01", month)
		if err != nil {
			return fmt.Errorf("bunny-close --month must be YYYY-MM: %w", err)
		}
		// Reprocessing a month that is already closed is the one write that
		// ignores closedAt, which is why it is a command someone types rather
		// than anything a schedule can reach.
		return bunnyJob(cfg, db, logr).ReopenAndCloseMonth(ctx, tenantID, parsed.Year(), int(parsed.Month()))

	default:
		return fmt.Errorf("unknown cmd: %s", cmd)
	}
}

// bunnyJob builds the usage worker for a one-off run. It takes no Redis: the
// lock exists to keep several replicas of the API off the same day, and a
// command invoked by hand is not one of them.
func bunnyJob(cfg *config.Config, db *sql.DB, logr logger.Logger) *bunnyusage.Job {
	return bunnyusage.New(db, bunny.NewAccountService(cfg, logr), bunnyusage.Unlocked{}, logr, "cmd/analytics")
}
