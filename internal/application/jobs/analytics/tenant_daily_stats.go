package analytics

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"time"
)

// dayBoundsUTC computes the UTC start/end of a tenant-local calendar day.
// Avoids applying DATE(... AT TIME ZONE ...) in WHERE predicates (which would
// disable the (tenantId, createdAt) index range scan).
func dayBoundsUTC(localDay time.Time, tz string) (time.Time, time.Time, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	start := time.Date(localDay.Year(), localDay.Month(), localDay.Day(), 0, 0, 0, 0, loc).UTC()
	end := start.Add(24 * time.Hour)
	return start, end, nil
}

// upsertTenantDailyStats writes one TenantDailyStats row per tenant-local day
// that has any activity in the window [from, to). Each per-day INSERT uses
// precomputed UTC bounds so the raw-table counters use index range scans.
func upsertTenantDailyStats(ctx context.Context, tx *sql.Tx, tenantId, tz string, from, to time.Time) error {
	days, err := tenantLocalDaysInRange(ctx, tx, tenantId, tz, from, to)
	if err != nil {
		return err
	}
	slog.Info("daily stats: active days resolved", "tenantId", tenantId, "days", len(days))
	for i, localDay := range days {
		if i > 0 && i%30 == 0 {
			slog.Info("daily stats progress", "tenantId", tenantId, "done", i, "total", len(days))
		}
		dayStart, dayEnd, err := dayBoundsUTC(localDay, tz)
		if err != nil {
			return err
		}
		details, err := buildDayDetailsJSON(ctx, tx, tenantId, dayStart, dayEnd)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, dailyStatsSQL,
			tenantId, // $1
			localDay, // $2 (Date)
			dayStart, // $3 (UTC range start)
			dayEnd,   // $4 (UTC range end)
			details,  // $5 (JSON)
		); err != nil {
			return err
		}
	}
	return nil
}

func tenantLocalDaysInRange(ctx context.Context, tx *sql.Tx, tenantId, tz string, from, to time.Time) ([]time.Time, error) {
	rows, err := tx.QueryContext(ctx, tenantLocalDaysInRangeSQL, tenantId, tz, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]time.Time, 0, 8)
	for rows.Next() {
		var d time.Time
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

type watchItem struct {
	LessonId string `json:"lessonId"`
	Value    int    `json:"value"`
}

// buildDayDetailsJSON returns { topWatch: [{lessonId, value}] } — top 10 lessons by
// watch seconds in the day's UTC bounds. Shape consumed by the TS getTopLessons
// dashboard query.
func buildDayDetailsJSON(ctx context.Context, tx *sql.Tx, tenantId string, dayStart, dayEnd time.Time) ([]byte, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT "lessonId", SUM("seconds")::int AS v FROM "LessonWatchEvent"
WHERE "tenantId" = $1 AND "createdAt" >= $2 AND "createdAt" < $3
GROUP BY "lessonId" ORDER BY v DESC LIMIT 10
`, tenantId, dayStart, dayEnd)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	top := make([]watchItem, 0, 10)
	for rows.Next() {
		var item watchItem
		if err := rows.Scan(&item.LessonId, &item.Value); err != nil {
			return nil, err
		}
		top = append(top, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"topWatch": top})
}
