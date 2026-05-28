package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/joho/godotenv"

	analyticsjobs "github.com/memberclass-backend-golang/internal/application/jobs/analytics"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/database"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/logger"
)

func main() {
	_ = godotenv.Load()

	cmd := flag.String("cmd", "daily", "daily|monthly|backfill")
	from := flag.String("from", "", "YYYY-MM (backfill)")
	to := flag.String("to", "", "YYYY-MM (backfill)")
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
		if err := analyticsjobs.NewDailyRollupJob(db, logr).Execute(ctx); err != nil {
			log.Fatalf("daily: %v", err)
		}
	case "monthly":
		if err := analyticsjobs.NewMonthlyRollupJob(db, logr).Execute(ctx); err != nil {
			log.Fatalf("monthly: %v", err)
		}
	case "backfill":
		if *from == "" || *to == "" {
			log.Fatal("backfill requires --from=YYYY-MM --to=YYYY-MM")
		}
		if err := analyticsjobs.Backfill(ctx, db, logr, *from, *to); err != nil {
			log.Fatalf("backfill: %v", err)
		}
	default:
		log.Fatalf("unknown cmd: %s", *cmd)
	}
	os.Exit(0)
}
