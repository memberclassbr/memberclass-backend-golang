package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"time"

	"github.com/joho/godotenv"

	analyticsjobs "github.com/memberclass-backend-golang/internal/application/jobs/analytics"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/database"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/logger"
)

func nowMinus24() time.Time { return time.Now().UTC().Add(-24 * time.Hour) }
func prevMonthStart() time.Time {
	now := time.Now().UTC()
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	return first.AddDate(0, -1, 0)
}

func main() {
	_ = godotenv.Load()

	cmd := flag.String("cmd", "daily", "daily|monthly|backfill|backfill-extras")
	from := flag.String("from", "", "YYYY-MM (backfill)")
	to := flag.String("to", "", "YYYY-MM (backfill)")
	tenantId := flag.String("tenantId", "", "scope backfill/daily/monthly to a single tenant (empty = all)")
	skipUserEvent := flag.Bool("skipUserEvent", false, "skip Read fixups + UserEvent migration; only run daily/monthly rollup")
	concurrency := flag.Int("concurrency", 2, "all-tenants backfill: tenants processed in parallel")
	chunk := flag.Int("chunk", 5000, "backfill: rows per set-based INSERT...SELECT chunk")
	sleepMs := flag.Int("sleepMs", 100, "backfill: pause between chunks (ms) to ease DB load")
	flag.Parse()

	logr := logger.NewLogger()

	db, err := database.NewDB()
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	// Cancel on Ctrl-C / SIGTERM so a long backfill stops cleanly between chunks.
	// The work is idempotent (ON CONFLICT), so a stopped run can just be re-run.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch *cmd {
	case "daily":
		if *tenantId != "" {
			if err := analyticsjobs.NewDailyRollupJob(db, logr).RunForUTCInstantForTenant(ctx, nowMinus24(), *tenantId); err != nil {
				log.Fatalf("daily: %v", err)
			}
		} else {
			if err := analyticsjobs.NewDailyRollupJob(db, logr).Execute(ctx); err != nil {
				log.Fatalf("daily: %v", err)
			}
		}
	case "monthly":
		if *tenantId != "" {
			if err := analyticsjobs.NewMonthlyRollupJob(db, logr).RunForMonthForTenant(ctx, prevMonthStart(), *tenantId); err != nil {
				log.Fatalf("monthly: %v", err)
			}
		} else {
			if err := analyticsjobs.NewMonthlyRollupJob(db, logr).Execute(ctx); err != nil {
				log.Fatalf("monthly: %v", err)
			}
		}
	case "backfill":
		if *from == "" || *to == "" {
			log.Fatal("backfill requires --from=YYYY-MM --to=YYYY-MM")
		}
		sleep := time.Duration(*sleepMs) * time.Millisecond
		if *tenantId != "" {
			if err := analyticsjobs.Backfill(ctx, db, logr, *from, *to, *tenantId, *skipUserEvent, *chunk, sleep); err != nil {
				log.Fatalf("backfill: %v", err)
			}
		} else {
			if err := analyticsjobs.BackfillAllTenants(ctx, db, logr, *from, *to, *skipUserEvent, *concurrency, *chunk, sleep); err != nil {
				log.Fatalf("backfill: %v", err)
			}
		}
	case "backfill-extras":
		if err := analyticsjobs.BackfillExtras(ctx, db, logr, *tenantId); err != nil {
			log.Fatalf("backfill-extras: %v", err)
		}
	default:
		log.Fatalf("unknown cmd: %s", *cmd)
	}
	os.Exit(0)
}
