package notifications

import (
	"context"
	"time"
)

// StartCleanupJob runs daily retention sweeps in the background. The job
// goroutine ticks every 24h and exits when ctx is cancelled. Pass the
// shutdown-aware context from main.go.
//
// Two sweeps run on each tick:
//   - 30-day retention on Notification (CASCADE drops UserNotification rows).
//   - top-100 trim on UserNotification per (userId, tenantId).
//
// Both sweeps run once at startup so a fresh deploy doesn't wait 24h to
// catch up on a backlog.
func (f *Feature) StartCleanupJob(ctx context.Context) {
	go func() {
		f.runCleanup(ctx)

		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				f.runCleanup(ctx)
			}
		}
	}()
}

func (f *Feature) runCleanup(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Minute)
	defer cancel()

	if n, err := f.pruneOldNotifications(ctx); err != nil {
		f.log.Error("notifications.cleanup.prune_failed", "error", err.Error())
	} else if n > 0 {
		f.log.Info("notifications.cleanup.pruned", "rows", n)
	}

	if n, err := f.trimUserInbox(ctx); err != nil {
		f.log.Error("notifications.cleanup.trim_failed", "error", err.Error())
	} else if n > 0 {
		f.log.Info("notifications.cleanup.trimmed", "rows", n)
	}
}

// pruneOldNotifications deletes Notification rows older than 30 days. The
// schema's ON DELETE CASCADE on UserNotification.notificationId removes
// the inbox rows for free.
func (f *Feature) pruneOldNotifications(ctx context.Context) (int64, error) {
	res, err := f.db.ExecContext(ctx, `
		DELETE FROM "Notification"
		WHERE "createdAt" < NOW() - INTERVAL '30 days'
	`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// trimUserInbox keeps only the 100 newest UserNotification rows per
// (userId, tenantId). Anything older is dropped — we don't surface deep
// history in the inbox UI.
func (f *Feature) trimUserInbox(ctx context.Context) (int64, error) {
	res, err := f.db.ExecContext(ctx, `
		DELETE FROM "UserNotification"
		WHERE id IN (
			SELECT id FROM (
				SELECT id, ROW_NUMBER() OVER (
					PARTITION BY "userId", "tenantId"
					ORDER BY "createdAt" DESC
				) AS rn
				FROM "UserNotification"
			) sub
			WHERE rn > 100
		)
	`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
