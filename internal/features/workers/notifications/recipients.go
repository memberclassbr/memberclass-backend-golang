package notifications

import (
	"context"
)

// recipient is a single (userId, fcm token) pair the worker will push to.
// One user may appear multiple times (one per device) — that's fine, each
// token is an independent FCM target.
type recipient struct {
	userID string
	token  string
}

// resolveRecipients enumerates the destinations for a multicast send.
// NOT used for fanout=READ + audience=tenant — those use FCM topics and
// don't need a token list.
//
// All variants filter UsersOnTenants.pushDisabledTypes against n.Type so a
// user who muted a category does not receive the push.
//
// Ordering MATTERS: dispatch resumes from `lastBatchIndex` after a crash by
// re-running this query and skipping the first N batches. If the order
// shifts between calls (e.g. the planner picks a different join), resumed
// runs target a different slice of users — duplicate sends to some and
// missed sends to others. The ORDER BY in each query keeps the slice stable.
func (f *Feature) resolveRecipients(ctx context.Context, n Notification) ([]recipient, error) {
	switch {
	case n.Fanout == FanoutWrite:
		// Personal notification — one or more devices for a single user, joined
		// through the UserNotification row that the writer (Next.js) created.
		const q = `
			SELECT un."userId", nd.token
			FROM "UserNotification" un
			JOIN "UsersOnTenants" uot
			  ON uot."userId" = un."userId" AND uot."tenantId" = un."tenantId"
			JOIN "NotificationDevice" nd
			  ON nd."userId" = un."userId" AND nd."tenantId" = un."tenantId"
			WHERE un."notificationId" = $1
			  AND NOT COALESCE($2::text = ANY(uot."pushDisabledTypes"), FALSE)
			ORDER BY un.id
		`
		return f.queryRecipients(ctx, q, n.ID, string(n.Type))

	case n.Fanout == FanoutRead && deref(n.AudienceType) == string(AudienceTenant):
		// Tenant-wide broadcast. Enumerate NotificationDevice rows for the
		// tenant directly — devices may exist without a UsersOnTenants row
		// because the user has not logged in yet to bind the device. Those
		// anonymous devices ALWAYS receive (no preference to honor); logged-in
		// devices apply the pushDisabledTypes filter.
		const q = `
			SELECT COALESCE(nd."userId", ''), nd.token
			FROM "NotificationDevice" nd
			LEFT JOIN "UsersOnTenants" uot
			  ON uot."userId" = nd."userId" AND uot."tenantId" = nd."tenantId"
			WHERE nd."tenantId" = $1
			  AND (
			    uot."userId" IS NULL
			    OR NOT COALESCE($2::text = ANY(uot."pushDisabledTypes"), FALSE)
			  )
			ORDER BY nd.id
		`
		return f.queryRecipients(ctx, q, n.TenantID, string(n.Type))

	case n.Fanout == FanoutRead && deref(n.AudienceType) == string(AudienceDelivery):
		const q = `
			SELECT mod."memberId", nd.token
			FROM "MemberOnDelivery" mod
			JOIN "UsersOnTenants" uot
			  ON uot."userId" = mod."memberId" AND uot."tenantId" = mod."tenantId"
			JOIN "NotificationDevice" nd
			  ON nd."userId" = mod."memberId" AND nd."tenantId" = mod."tenantId"
			WHERE mod."deliveryId" = $1 AND mod."tenantId" = $2
			  AND NOT COALESCE($3::text = ANY(uot."pushDisabledTypes"), FALSE)
			ORDER BY mod."memberId", nd.id
		`
		return f.queryRecipients(ctx, q, deref(n.AudienceID), n.TenantID, string(n.Type))

	default:
		return nil, nil
	}
}

func (f *Feature) queryRecipients(ctx context.Context, q string, args ...any) ([]recipient, error) {
	rows, err := f.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []recipient
	for rows.Next() {
		var r recipient
		if err := rows.Scan(&r.userID, &r.token); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
