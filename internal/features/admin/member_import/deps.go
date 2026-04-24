// Package member_import is a vertical slice for the
// `POST /imports/members` endpoint: an admin-only bulk user import.
//
// Security model
//   - Session cookie (next-auth) is decrypted by the AuthMiddleware that the
//     router composes in front of every route in this slice.
//   - The slice itself re-validates: the session's userId must belong to the
//     target tenantId AND their UsersOnTenants.role must be != "member"
//     (i.e. owner/admin/etc).
//
// Behavior
//   - Validates input + auth, then INSERTs a UserImport header and returns 202
//     immediately with `{ importId, status: "processing" }`.
//   - Spins a goroutine (panic-recovered) that processes rows in batches of
//     100: find-or-create User, upsert UsersOnTenants, create MemberOnDelivery
//     rows, mint MagicToken rows, dispatch emails via Resend, update counters
//     and per-row status on UserImport / UserImportRow.
//
// See CLAUDE.md ("Architecture migration in progress") for the VSA rules.
package member_import

import (
	"context"
	"database/sql"
	"net/http"
	"sync"

	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/external_services/resend"
)

// Feature holds the shared dependencies for every action in this slice.
type Feature struct {
	db     *sql.DB
	log    ports.Logger
	resend resend.Service

	// inflight tracks background import goroutines so shutdown can drain
	// them before the DB is closed. Without this, an in-flight import
	// racing with `dbMap.CloseAll()` would hit "use of closed network
	// connection" errors and leave its UserImport row stuck in "processing"
	// (StartupReset only picks it up after a 5-minute grace on next boot).
	inflight sync.WaitGroup
}

// New builds the slice. Wire it in cmd/api/main.go via fx.Provide.
func New(db *sql.DB, log ports.Logger, resendSvc resend.Service) *Feature {
	return &Feature{db: db, log: log, resend: resendSvc}
}

// Wait blocks until every in-flight import goroutine has returned, or until
// `ctx` expires — whichever comes first. Call during graceful shutdown,
// BEFORE closing the DB. If ctx expires, the still-running goroutines will
// be interrupted by the DB close and their UserImport row is recovered by
// StartupReset on the next boot (5-min grace).
func (f *Feature) Wait(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		f.inflight.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		f.log.Warn("member_import: shutdown drained with in-flight imports still running",
			"timeout", ctx.Err().Error())
	}
}

// MiddlewareSet carries the chi-compatible middlewares the slice's routes
// need. Only session auth — CORS is applied at the router level.
type MiddlewareSet struct {
	SessionAuth func(http.Handler) http.Handler
}
