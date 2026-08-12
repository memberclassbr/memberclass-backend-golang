package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/memberclass-backend-golang/internal/app"
	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/memberclass-backend-golang/internal/platform/logger"
	"github.com/memberclass-backend-golang/internal/platform/telemetry"
)

// main does nothing but choose the exit code. All the work is in run, because
// os.Exit skips deferred calls: with the body inline, a failure to open the
// database would exit before the telemetry shutdown ran, and the spans
// describing that failure would die with the process.
func main() {
	os.Exit(run())
}

func run() int {
	_ = godotenv.Load()

	// Config is resolved before anything else, so a deployment with missing
	// variables dies here naming every offender at once instead of booting
	// into a half-working state. Logging is not up yet, hence stderr.
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid configuration: %v\n", err)
		return 1
	}

	log := logger.NewLogger()

	// Telemetry is installed before the first connection is opened, so the
	// failures below are instrumented too. It never blocks boot: a deployment
	// with no collector configured runs uninstrumented and says so.
	shutdownTelemetry, err := telemetry.Init(context.Background(), cfg, log)
	if err != nil {
		log.Error("telemetry init failed, continuing uninstrumented: " + err.Error())
		shutdownTelemetry = func(context.Context) error { return nil }
	}
	defer func() {
		if err := shutdownTelemetry(context.Background()); err != nil {
			log.Error("telemetry shutdown: " + err.Error())
		}
	}()

	application, err := app.New(cfg, log)
	if err != nil {
		log.Error("failed to start: " + err.Error())
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := application.Run(ctx); err != nil {
		log.Error("server error: " + err.Error())
		return 1
	}

	return 0
}
