package database

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

// DBMap maps bucket names to their respective database connections.
type DBMap map[string]*sql.DB

// bucketDSNMapping maps bucket names to environment variable names containing DSNs.
var bucketDSNMapping = map[string]string{
	"memberclass":  "DB_DSN",
	"ephra":        "DB_EPHRA_DSN",
	"celetusclass": "DB_CELETUS_DSN",
}

const defaultBucket = "memberclass"

// NewMultiDB creates database connections for all configured buckets.
// It skips buckets whose DSN env var is not set.
func NewMultiDB(logger ports.Logger) (DBMap, error) {
	driver := os.Getenv("DB_DRIVER")
	if driver == "" {
		driver = "postgres"
	}

	dbs := make(DBMap)

	for bucket, envVar := range bucketDSNMapping {
		dsn := os.Getenv(envVar)
		if dsn == "" {
			logger.Warn(fmt.Sprintf("No DSN configured for bucket '%s' (env: %s), skipping", bucket, envVar))
			continue
		}

		db, err := sql.Open(driver, dsn)
		if err != nil {
			// Close already opened connections
			for _, openDB := range dbs {
				openDB.Close()
			}
			return nil, fmt.Errorf("failed to open database for bucket '%s': %w", bucket, err)
		}

		if err := db.Ping(); err != nil {
			db.Close()
			for _, openDB := range dbs {
				openDB.Close()
			}
			return nil, fmt.Errorf("failed to ping database for bucket '%s': %w", bucket, err)
		}

		logger.Info(fmt.Sprintf("Database connection established for bucket '%s'", bucket))
		dbs[bucket] = db
	}

	if len(dbs) == 0 {
		return nil, fmt.Errorf("no database connections configured, check environment variables: DB_DSN, DB_EPHRA_DSN, DB_CELETUS_DSN")
	}

	return dbs, nil
}

// DefaultDB extracts the default (memberclass) *sql.DB from DBMap.
// This avoids opening a duplicate connection via NewDB.
func DefaultDB(dbMap DBMap) (*sql.DB, error) {
	db, ok := dbMap[defaultBucket]
	if !ok {
		// Fallback to first available
		for _, db := range dbMap {
			return db, nil
		}
		return nil, fmt.Errorf("no database connections available in DBMap")
	}
	return db, nil
}

// CloseAll closes all database connections in the DBMap.
func (m DBMap) CloseAll() error {
	var lastErr error
	for bucket, db := range m {
		if err := db.Close(); err != nil {
			lastErr = fmt.Errorf("failed to close database for bucket '%s': %w", bucket, err)
		}
	}
	return lastErr
}
