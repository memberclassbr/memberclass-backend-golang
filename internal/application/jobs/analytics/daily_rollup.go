package analytics

import (
	"context"
	"database/sql"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/ports"
)

// DailyRollupJob aggregates the previous tenant-local day(s) of raw events into
// TenantDailyUserActivity + TenantDailyStats. Cron 0 0 8 * * * UTC covers
// previous-day boundaries across America/{Sao_Paulo, New_York, Los_Angeles}.
type DailyRollupJob struct {
	db     *sql.DB
	logger ports.Logger
}

func NewDailyRollupJob(db *sql.DB, logger ports.Logger) *DailyRollupJob {
	return &DailyRollupJob{db: db, logger: logger}
}

func (j *DailyRollupJob) Name() string { return "analytics.daily_rollup" }

func (j *DailyRollupJob) Execute(ctx context.Context) error {
	now := time.Now().UTC()
	return j.RunForUTCInstant(ctx, now.Add(-24*time.Hour))
}

// RunForUTCInstant rolls up the tenant-local day(s) intersecting a 48h window
// centered slightly behind the given UTC instant. The window safely covers any
// tenant timezone (UTC-12 .. UTC+14).
func (j *DailyRollupJob) RunForUTCInstant(ctx context.Context, anyUTCInDay time.Time) error {
	tenants, err := j.listAllTenantsWithTimezone(ctx)
	if err != nil {
		return err
	}
	from := anyUTCInDay.Add(-36 * time.Hour)
	to := anyUTCInDay.Add(12 * time.Hour)
	for _, t := range tenants {
		if err := j.rollupTenantDays(ctx, t.id, t.timezone, from, to); err != nil {
			j.logger.Error("daily rollup failed", "tenantId", t.id, "err", err.Error())
		}
	}
	return nil
}

// RunForUTCInstantForTenant is the single-tenant variant used by backfill.
func (j *DailyRollupJob) RunForUTCInstantForTenant(ctx context.Context, anyUTCInDay time.Time, tenantId string) error {
	tz, err := j.lookupTenantTimezone(ctx, tenantId)
	if err != nil {
		return err
	}
	from := anyUTCInDay.Add(-36 * time.Hour)
	to := anyUTCInDay.Add(12 * time.Hour)
	return j.rollupTenantDays(ctx, tenantId, tz, from, to)
}

func (j *DailyRollupJob) lookupTenantTimezone(ctx context.Context, tenantId string) (string, error) {
	var tz string
	err := j.db.QueryRowContext(ctx,
		`SELECT COALESCE("timezone", 'America/Sao_Paulo') FROM "Tenant" WHERE id = $1`,
		tenantId,
	).Scan(&tz)
	return tz, err
}

type tenantTz struct {
	id       string
	timezone string
}

func (j *DailyRollupJob) listAllTenantsWithTimezone(ctx context.Context) ([]tenantTz, error) {
	rows, err := j.db.QueryContext(ctx, listAllTenantsSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]tenantTz, 0, 64)
	for rows.Next() {
		var t tenantTz
		if err := rows.Scan(&t.id, &t.timezone); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (j *DailyRollupJob) rollupTenantDays(ctx context.Context, tenantId, tz string, from, to time.Time) error {
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, q := range dailyActivitySQLs {
		if _, err := tx.ExecContext(ctx, q, tenantId, tz, from, to); err != nil {
			return err
		}
	}
	if err := upsertTenantDailyStats(ctx, tx, tenantId, tz, from, to); err != nil {
		return err
	}
	return tx.Commit()
}
