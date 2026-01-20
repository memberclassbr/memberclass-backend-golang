package database

import (
	"database/sql"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

type MigrationService struct {
	db     *sql.DB
	logger ports.Logger
}

func NewMigrationService(db *sql.DB, logger ports.Logger) *MigrationService {
	return &MigrationService{
		db:     db,
		logger: logger,
	}
}

func (m *MigrationService) RunMigrations(migrationsPath string) error {
	driver, err := postgres.WithInstance(m.db, &postgres.Config{})
	if err != nil {
		m.logger.Error("Failed to create postgres driver: " + err.Error())
		return fmt.Errorf("failed to create postgres driver: %w", err)
	}

	migration, err := migrate.NewWithDatabaseInstance(
		fmt.Sprintf("file://%s", migrationsPath),
		"postgres",
		driver,
	)
	if err != nil {
		m.logger.Error("Failed to create migration instance: " + err.Error())
		return fmt.Errorf("failed to create migration instance: %w", err)
	}

	version, dirty, err := migration.Version()
	if err != nil && err != migrate.ErrNilVersion {
		m.logger.Error("Failed to get migration version: " + err.Error())
		return fmt.Errorf("failed to get migration version: %w", err)
	}

	if dirty {
		m.logger.Error(fmt.Sprintf("Database is in dirty state at version %d", version))
		return fmt.Errorf("database is in dirty state at version %d", version)
	}

	if err := migration.Up(); err != nil && err != migrate.ErrNoChange {
		m.logger.Error("Failed to run migrations: " + err.Error())
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	newVersion, _, _ := migration.Version()
	if err == migrate.ErrNoChange {
		m.logger.Info(fmt.Sprintf("No new migrations to apply (current version: %d)", newVersion))
	} else {
		m.logger.Info(fmt.Sprintf("Migrations applied successfully (version: %d)", newVersion))
	}

	return nil
}
