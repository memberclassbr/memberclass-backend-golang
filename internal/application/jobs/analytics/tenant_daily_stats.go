package analytics

import (
	"context"
	"database/sql"
	"time"
)

// upsertTenantDailyStats is implemented in Task 12. Stubbed here to keep the
// package compiling alongside the daily_rollup skeleton.
func upsertTenantDailyStats(ctx context.Context, tx *sql.Tx, tenantId, tz string, from, to time.Time) error {
	_ = ctx
	_ = tx
	_ = tenantId
	_ = tz
	_ = from
	_ = to
	return nil
}
