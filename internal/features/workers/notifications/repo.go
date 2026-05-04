package notifications

import (
	"context"
	"database/sql"
)

// claimPending pulls up to `limit` Notification rows that are due (status=pending
// and scheduledAt is null or past) and atomically marks them 'sending' so no
// other worker picks them up.
//
// We don't use FOR UPDATE SKIP LOCKED because CockroachDB's SERIALIZABLE
// semantics make that subtly different from Postgres. The UPDATE…RETURNING
// pattern is race-free here: the inner SELECT inside an UPDATE re-reads
// under the same transaction, so two concurrent calls cannot both flip the
// same row from pending → sending.
func (f *Feature) claimPending(ctx context.Context, limit int) ([]Notification, error) {
	const q = `
		UPDATE "Notification"
		SET status = 'sending', "updatedAt" = NOW()
		WHERE id IN (
			SELECT id FROM "Notification"
			WHERE status = 'pending'
			  AND ("scheduledAt" IS NULL OR "scheduledAt" <= NOW())
			ORDER BY "createdAt"
			LIMIT $1
		)
		RETURNING id, "tenantId", type, fanout, status,
		          title, body, "messageKey", "messageData",
		          "audienceType", "audienceId",
		          "recipientCount", "sentCount", "failedCount", "lastBatchIndex",
		          "scheduledAt", "updatedAt"
	`
	rows, err := f.db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(
			&n.ID, &n.TenantID, &n.Type, &n.Fanout, &n.Status,
			&n.Title, &n.Body, &n.MessageKey, &n.MessageData,
			&n.AudienceType, &n.AudienceID,
			&n.RecipientCount, &n.SentCount, &n.FailedCount, &n.LastBatchIndex,
			&n.ScheduledAt, &n.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// resetOrphans returns to 'pending' any row stuck in 'sending' for more than
// 5 minutes — a previous worker process crashed mid-dispatch. The lastBatchIndex
// column lets dispatch resume without re-sending the chunks already done.
func (f *Feature) resetOrphans(ctx context.Context) (int64, error) {
	const q = `
		UPDATE "Notification"
		SET status = 'pending', "updatedAt" = NOW()
		WHERE status = 'sending' AND "updatedAt" < NOW() - INTERVAL '5 minutes'
	`
	res, err := f.db.ExecContext(ctx, q)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (f *Feature) markSent(ctx context.Context, id string, sentCount, failedCount int) error {
	_, err := f.db.ExecContext(ctx, `
		UPDATE "Notification"
		SET status = 'sent', "sentAt" = NOW(),
		    "sentCount" = $2, "failedCount" = $3,
		    "updatedAt" = NOW()
		WHERE id = $1
	`, id, sentCount, failedCount)
	return err
}

func (f *Feature) markFailed(ctx context.Context, id, reason string) error {
	_, err := f.db.ExecContext(ctx, `
		UPDATE "Notification"
		SET status = 'failed', "failureReason" = $2, "updatedAt" = NOW()
		WHERE id = $1
	`, id, reason)
	return err
}

func (f *Feature) updateProgress(ctx context.Context, id string, sentCount, failedCount, lastBatchIndex int) error {
	_, err := f.db.ExecContext(ctx, `
		UPDATE "Notification"
		SET "sentCount" = $2, "failedCount" = $3, "lastBatchIndex" = $4,
		    "updatedAt" = NOW()
		WHERE id = $1
	`, id, sentCount, failedCount, lastBatchIndex)
	return err
}

func (f *Feature) setRecipientCount(ctx context.Context, id string, n int) error {
	_, err := f.db.ExecContext(ctx, `
		UPDATE "Notification"
		SET "recipientCount" = $2, "updatedAt" = NOW()
		WHERE id = $1
	`, id, n)
	return err
}

// getTenantInstance reads Tenant.notificationsInstance — the value drives
// which Firebase project the FCM client uses. Returns "" when the column
// is null (default project).
func (f *Feature) getTenantInstance(ctx context.Context, tenantID string) (string, error) {
	var instance sql.NullString
	err := f.db.QueryRowContext(ctx,
		`SELECT "notificationsInstance" FROM "Tenant" WHERE id = $1`, tenantID,
	).Scan(&instance)
	if err != nil {
		return "", err
	}
	if !instance.Valid {
		return "", nil
	}
	return instance.String, nil
}

// deleteDevice removes a stale FCM token (FCM returned
// "registration-token-not-registered"). Keyed by (tenantId, token) — token
// is effectively unique and the userId column may be NULL for anonymous
// devices that haven't bound to a logged-in user yet, so a userId-based
// WHERE would skip those rows.
func (f *Feature) deleteDevice(ctx context.Context, tenantID, token string) error {
	_, err := f.db.ExecContext(ctx, `
		DELETE FROM "NotificationDevice"
		WHERE "tenantId" = $1 AND token = $2
	`, tenantID, token)
	return err
}
