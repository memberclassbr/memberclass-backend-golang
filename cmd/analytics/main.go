package main

import (
	"context"
	"flag"
	"log"
	"os"

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

	cmd := flag.String("cmd", "daily", "daily|monthly|backfill")
	from := flag.String("from", "", "YYYY-MM (backfill)")
	to := flag.String("to", "", "YYYY-MM (backfill)")
	tenantId := flag.String("tenantId", "", "scope backfill/daily/monthly to a single tenant (empty = all)")
	flag.Parse()

	logr := logger.NewLogger()

	db, err := database.NewDB()
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

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
		if err := analyticsjobs.Backfill(ctx, db, logr, *from, *to, *tenantId); err != nil {
			log.Fatalf("backfill: %v", err)
		}
	default:
		log.Fatalf("unknown cmd: %s", *cmd)
	}
	os.Exit(0)
}
