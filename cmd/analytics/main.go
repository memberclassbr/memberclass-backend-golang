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

	cmd := flag.String("cmd", "daily", "daily|monthly|backfill|backfill-extras")
	from := flag.String("from", "", "YYYY-MM (backfill)")
	to := flag.String("to", "", "YYYY-MM (backfill)")
	tenantId := flag.String("tenantId", "", "scope backfill/daily/monthly to a single tenant (empty = all)")
	skipUserEvent := flag.Bool("skipUserEvent", false, "skip Read fixups + UserEvent migration; only run daily/monthly rollup")
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

	if err := execute(ctx, *cmd, db, logr, cfg, *from, *to, *tenantId, *skipUserEvent); err != nil {
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

	default:
		return fmt.Errorf("unknown cmd: %s", cmd)
	}
}
