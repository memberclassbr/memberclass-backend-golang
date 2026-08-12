// Package database opens the service's SQL connections.
//
// A deployment owns exactly two databases and knows both by name:
//
//   - the tenant database (DB_DSN), which every feature reads and writes;
//   - an optional transcription database (DB_TRANSCRIPTION_DSN), a separate
//     Postgres with the pgvector extension, owned solely by the transcription
//     slice.
//
// This replaced a bucket map that opened one connection per tenant brand and
// resolved between them at request time. Deployments are now one-per-customer,
// each with its own database, so the routing had nothing left to route: the
// three tenant DSNs pointed at the same server, and a "search every database
// for this lesson" probe ran N queries to answer what one query answers.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/XSAM/otelsql"
	_ "github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"

	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/memberclass-backend-golang/internal/platform/logger"
)

// pingTimeout bounds the initial connection probe so an unreachable database
// cannot stall boot indefinitely.
const pingTimeout = 10 * time.Second

// TranscriptionDB is the optional pgvector connection. It is a distinct type so
// the two *sql.DB values can be told apart when wiring, and so a consumer that
// forgets to handle the disabled case fails to compile rather than at runtime.
//
// DB is nil when the deployment has no transcription database configured;
// callers must check before use.
type TranscriptionDB struct {
	*sql.DB
}

// Open connects to the tenant database. A failure here is fatal: every feature
// needs it.
func Open(cfg *config.Config, log logger.Logger) (*sql.DB, error) {
	db, err := open(cfg.DB.Driver, cfg.DB.DSN, "tenant")
	if err != nil {
		return nil, fmt.Errorf("tenant database: %w", err)
	}
	log.Info("Database connection established")
	return db, nil
}

// OpenTranscription connects to the pgvector database when one is configured.
// It returns a zero TranscriptionDB rather than an error when the feature is
// off, and also when the connection fails: transcription is a background
// pipeline, and a customer without it should still get a working API.
func OpenTranscription(cfg *config.Config, log logger.Logger) TranscriptionDB {
	if !cfg.Transcription.Enabled {
		return TranscriptionDB{}
	}

	db, err := open(cfg.DB.Driver, cfg.Transcription.DSN, "transcription")
	if err != nil {
		log.Warn("transcription database unavailable, slice will stay inert: " + err.Error())
		return TranscriptionDB{}
	}

	log.Info("Transcription database connection established")
	return TranscriptionDB{DB: db}
}

// open connects and instruments in one step. otelsql wraps the driver so every
// query carries a span, and RegisterDBStatsMetrics publishes the pool gauges —
// the pair that separates "the query is slow" from "the query waited for a
// connection", which look the same from the outside.
//
// role distinguishes the two pools in the resulting telemetry. Without it both
// databases report under one name and a slow pgvector query is indistinguishable
// from a slow tenant query.
func open(driver, dsn, role string) (*sql.DB, error) {
	attrs := otelsql.WithAttributes(
		attribute.String("db.system.name", "postgresql"),
		attribute.String("db.role", role),
	)

	db, err := otelsql.Open(driver, dsn, attrs)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	// Pool metrics are a nice-to-have; a failure to register them is not a
	// reason to refuse a working database connection.
	if _, err := otelsql.RegisterDBStatsMetrics(db, attrs); err != nil {
		return db, nil
	}

	return db, nil
}
