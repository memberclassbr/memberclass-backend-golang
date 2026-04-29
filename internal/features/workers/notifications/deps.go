// Package notifications is a vertical slice that owns push notification
// dispatch. It runs as an in-process worker (no HTTP routes), polling the
// `Notification` table populated by the Next.js admin/web app and fanning
// out via Firebase Cloud Messaging.
//
// Architecture:
//   - Long-poll the Notification table; claim rows atomically with
//     UPDATE…RETURNING (CockroachDB-safe; no FOR UPDATE SKIP LOCKED).
//   - Resolve recipients per fanout/audience, filter by
//     UsersOnTenants.pushDisabledTypes.
//   - Dispatch via FCM topic (audience=tenant) or multicast (chunks of 500).
//   - Persist progress (sentCount/failedCount/lastBatchIndex) so a crashed
//     run can resume without resending.
//   - Daily cleanup: 30d retention on Notification + top-100 trim on
//     UserNotification per (userId, tenantId).
//
// See CLAUDE.md ("Architecture migration in progress") for the VSA rules.
package notifications

import (
	"context"
	"database/sql"
	"sync"

	"github.com/memberclass-backend-golang/internal/domain/ports"
)

// Feature holds the shared dependencies for the notifications worker slice.
type Feature struct {
	db  *sql.DB
	log ports.Logger
	fcm *fcmClient

	// running is set when Start() is called and cleared on Stop().
	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	done    chan struct{}
}

// New builds the slice. Wire it in cmd/api/main.go via fx.Provide.
func New(db *sql.DB, log ports.Logger) *Feature {
	return &Feature{
		db:  db,
		log: log,
		fcm: newFCMClient(),
	}
}
