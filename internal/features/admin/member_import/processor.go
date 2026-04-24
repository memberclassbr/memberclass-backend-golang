package member_import

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/memberclass-backend-golang/internal/domain/utils"
	"golang.org/x/crypto/bcrypt"
)

const (
	batchSize        = 100
	magicTokenTTL    = 24 * time.Hour
	bcryptCost       = 10
	defaultPassLen   = 10
	magicTokenRawLen = 32
	// 12 chars × 32-char alphabet ≈ 60 bits of entropy. Keeps collision
	// probability on the unique-indexed `shortCode` column negligible even
	// at millions of active tokens, which matters because a collision would
	// cascade to an email with a broken link (see I1 in review).
	shortCodeLen = 12
)

// rowState tracks per-row progress as we walk a batch. It's used to drive
// both the UserImportRow inserts and the email send categorization.
type rowState struct {
	rowIndex     int
	input        importUserInput
	userID       string
	name         string
	status       string // "created" | "updated" | "already_had" | "skipped" | "error"
	isNewUser    bool
	hasAll       bool
	magicToken   string // RAW token used in the email link
	shortCode    string // shortCode stored on MagicToken row
	errorMessage string
	emailSent    sql.NullString // "login" | "delivery" | "none"
	emailStatus  sql.NullString // "sent" | "failed" | "none"
}

// ---------- Orchestration ----------

// runImport is the long-running goroutine entry point. It runs with a fresh
// background context because the HTTP request that triggered it has already
// returned.
func (f *Feature) runImport(importID string, req *importRequest, tenant *tenantRow) {
	started := time.Now()

	// Panic recovery: mark the import as failed so the UI doesn't hang.
	defer func() {
		if rec := recover(); rec != nil {
			msg := fmt.Sprintf("panic: %v", rec)
			f.log.Error("import.panic", "import_id", importID, "message", msg)
			_, _ = f.db.Exec(
				`UPDATE "UserImport" SET status='failed', "errorMessage"=$1, "finishedAt"=NOW() WHERE id=$2`,
				msg, importID,
			)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Hour)
	defer cancel()

	// Shared credentials valid for this whole import.
	passwordAccount := req.PassDefault
	if passwordAccount == "" {
		passwordAccount = randomString(defaultPassLen)
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(passwordAccount), bcryptCost)
	if err != nil {
		f.finalizeAsFailed(importID, fmt.Sprintf("bcrypt hash password: %v", err))
		return
	}

	tokenRaw := randomBase64(magicTokenRawLen)
	tokenHash, err := bcrypt.GenerateFromPassword([]byte(tokenRaw), bcryptCost)
	if err != nil {
		f.finalizeAsFailed(importID, fmt.Sprintf("bcrypt hash token: %v", err))
		return
	}
	magicTokenValidUntil := time.Now().Add(magicTokenTTL)

	counters := &importCounters{}

	// Process in batches of 100 rows.
	for batchStart := 0; batchStart < len(req.Users); batchStart += batchSize {
		batchEnd := batchStart + batchSize
		if batchEnd > len(req.Users) {
			batchEnd = len(req.Users)
		}
		batch := req.Users[batchStart:batchEnd]

		batchStartedAt := time.Now()

		states, err := f.processBatch(
			ctx, importID, req,
			batch, batchStart,
			string(passwordHash), string(tokenHash),
			magicTokenValidUntil,
			counters,
		)
		if err != nil {
			f.log.Error("import.batch_failed",
				"import_id", importID,
				"batch_start", batchStart,
				"error", err.Error(),
			)
			// Continue to next batch; partial success is better than aborting.
		}

		// Send emails for this batch (fire synchronously within the job but
		// not blocking the HTTP handler — we're already in a goroutine).
		f.sendBatchEmails(ctx, importID, tenant, states, passwordAccount, counters)

		// Persist per-row outcomes (including email status now that we know).
		if err := f.insertRowRecords(ctx, importID, states); err != nil {
			f.log.Error("import.row_insert_failed",
				"import_id", importID, "error", err.Error())
		}

		// Update processed counter + batch-specific counters in one UPDATE.
		if err := f.updateImportCounters(ctx, importID, len(batch), counters); err != nil {
			f.log.Error("import.counters_update_failed",
				"import_id", importID, "error", err.Error())
		}

		f.log.Info("import.batch_processed",
			"import_id", importID,
			"batch_start", batchStart,
			"batch_size", len(batch),
			"duration_ms", time.Since(batchStartedAt).Milliseconds(),
		)
	}

	// Final status: completed vs partial.
	finalStatus := "completed"
	if counters.errorRows > 0 || counters.emailsFailed > 0 {
		finalStatus = "partial"
	}
	if err := f.finalizeStatus(importID, finalStatus, ""); err != nil {
		f.log.Error("import.finalize_failed",
			"import_id", importID, "error", err.Error())
	}

	f.log.Info("import.finished",
		"import_id", importID,
		"status", finalStatus,
		"duration_ms", time.Since(started).Milliseconds(),
		"created", counters.createdUsers,
		"updated", counters.updatedUsers,
		"already_had", counters.alreadyHadAll,
		"skipped", counters.skippedRows,
		"errors", counters.errorRows,
		"login_emails", counters.loginEmailsSent,
		"delivery_emails", counters.deliveryEmailsSent,
		"emails_failed", counters.emailsFailed,
	)
}

// ---------- Counters ----------

type importCounters struct {
	createdUsers       int
	updatedUsers       int
	alreadyHadAll      int
	skippedRows        int
	errorRows          int
	loginEmailsSent    int
	deliveryEmailsSent int
	emailsFailed       int
}

// ---------- Batch processor ----------

func (f *Feature) processBatch(
	ctx context.Context,
	importID string,
	req *importRequest,
	batch []importUserInput,
	batchStart int,
	passwordHash, tokenHash string,
	magicTokenValidUntil time.Time,
	counters *importCounters,
) ([]rowState, error) {
	states := make([]rowState, 0, len(batch))

	for i, u := range batch {
		rowIdx := batchStart + i
		state := rowState{rowIndex: rowIdx, input: u, name: u.Name}

		if u.Name == "" || u.Email == "" {
			state.status = "skipped"
			counters.skippedRows++
			states = append(states, state)
			continue
		}

		// --- Find-or-create user ---
		userID, existingName, existingDoc, uotExists, findErr := f.findUser(ctx, u, req.TenantID)
		if findErr != nil {
			state.status = "error"
			state.errorMessage = findErr.Error()
			counters.errorRows++
			states = append(states, state)
			continue
		}

		email := strings.ToLower(u.Email)

		if userID == "" {
			// Brand new User + UsersOnTenants.
			newID, err := f.createUser(ctx, email, u.Phone, passwordHash, tokenHash, magicTokenValidUntil)
			if err != nil {
				state.status = "error"
				state.errorMessage = err.Error()
				counters.errorRows++
				states = append(states, state)
				continue
			}
			userID = newID
			state.isNewUser = true
			counters.createdUsers++

			if err := f.upsertUsersOnTenants(ctx, userID, req.TenantID, passwordHash, u.Name, u.Document); err != nil {
				state.status = "error"
				state.errorMessage = err.Error()
				counters.errorRows++
				states = append(states, state)
				continue
			}
		} else if !uotExists {
			// User exists in the global "User" table but was never linked to
			// THIS tenant. Treat as a creation — memberclass is whitelabel,
			// so each tenant is a distinct product from the end-user's POV.
			// They're receiving tenant-scoped credentials for the first time
			// and need the full onboarding email (login template), not the
			// "you got new content" delivery template.
			if err := f.upsertUsersOnTenants(ctx, userID, req.TenantID, passwordHash, u.Name, u.Document); err != nil {
				state.status = "error"
				state.errorMessage = err.Error()
				counters.errorRows++
				states = append(states, state)
				continue
			}
			state.isNewUser = true
			counters.createdUsers++
		}

		// --- Check whether they already have all requested deliveries ---
		hasAll := false
		if len(req.Deliveries) > 0 && !state.isNewUser {
			ha, err := f.userHasAllDeliveries(ctx, userID, req.TenantID, req.Deliveries)
			if err != nil {
				state.status = "error"
				state.errorMessage = err.Error()
				counters.errorRows++
				states = append(states, state)
				continue
			}
			hasAll = ha
		}
		state.hasAll = hasAll

		// --- Refresh magic token only when there's real work to deliver ---
		if !hasAll {
			if !state.isNewUser {
				if err := f.refreshUserMagicToken(ctx, userID, tokenHash, magicTokenValidUntil); err != nil {
					state.status = "error"
					state.errorMessage = err.Error()
					counters.errorRows++
					states = append(states, state)
					continue
				}
			}
			// Create a per-user MagicToken row (matches the Next.js shape).
			// The email's magic link uses the shortCode path — if this row
			// fails to persist (unique collision, DB hiccup, …) the user
			// would receive an email with a non-existent `code` param and
			// be locked out. Mark the row as error instead so NO email is
			// sent; the tenant admin retries the import for that user.
			perUserRaw := randomBase64(magicTokenRawLen)
			shortCode, mtErr := f.createMagicToken(ctx, userID, req.TenantID, perUserRaw, email, magicTokenValidUntil)
			if mtErr != nil {
				f.log.Error("import.magic_token_create_failed",
					"import_id", importID,
					"row_index", rowIdx,
					"error", mtErr.Error(),
				)
				state.status = "error"
				state.errorMessage = "magic token creation failed: " + mtErr.Error()
				counters.errorRows++
				states = append(states, state)
				continue
			}
			state.magicToken = perUserRaw
			state.shortCode = shortCode
		}

		// --- If user already existed in tenant, optionally refresh doc/name ---
		if uotExists && !state.isNewUser {
			needsUpdate := (u.Document != "" && existingDoc == "") || (u.Name != "" && existingName != u.Name)
			if needsUpdate {
				if err := f.updateUsersOnTenantsNameDoc(ctx, userID, req.TenantID, u.Name, u.Document); err != nil {
					state.status = "error"
					state.errorMessage = err.Error()
					counters.errorRows++
					states = append(states, state)
					continue
				}
			}
		}

		// --- Create delivery memberships (if requested and not already full) ---
		if len(req.Deliveries) > 0 && !hasAll {
			assignedAt := f.parseAccession(u.Accession)
			if err := f.insertMemberOnDeliveries(ctx, userID, req.TenantID, req.Deliveries, assignedAt); err != nil {
				state.status = "error"
				state.errorMessage = err.Error()
				counters.errorRows++
				states = append(states, state)
				continue
			}
		}

		state.userID = userID
		switch {
		case state.isNewUser:
			state.status = "created"
		case hasAll:
			state.status = "already_had"
			counters.alreadyHadAll++
		default:
			state.status = "updated"
			// counters.updatedUsers was incremented above when the user was
			// newly linked to the tenant; if they already belonged to the
			// tenant and we just added deliveries we still call this
			// "updated" from the row's perspective.
			if uotExists {
				counters.updatedUsers++
			}
		}

		states = append(states, state)
	}

	return states, nil
}

// ---------- Lookup ----------

// findUser tries document-based lookup first (with CPF variants), then falls
// back to email. Returns (userId, name-on-tenant, doc-on-tenant, uotExists, err).
// userId is "" when nothing matched.
func (f *Feature) findUser(ctx context.Context, u importUserInput, tenantID string) (string, string, string, bool, error) {
	if u.Document != "" {
		variants := documentVariants(u.Document)
		if len(variants) > 0 {
			const q = `
				SELECT uot."userId", COALESCE(uot.name, ''), COALESCE(uot.document, '')
				FROM "UsersOnTenants" uot
				WHERE uot."tenantId" = $1
				  AND uot.document = ANY($2)
				LIMIT 1
			`
			var userID, name, doc string
			err := f.db.QueryRowContext(ctx, q, tenantID, pq.Array(variants)).Scan(&userID, &name, &doc)
			if err == nil {
				return userID, name, doc, true, nil
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return "", "", "", false, fmt.Errorf("find by document: %w", err)
			}
		}
	}

	// Email fallback.
	email := strings.ToLower(u.Email)
	var userID string
	err := f.db.QueryRowContext(ctx,
		`SELECT id FROM "User" WHERE email = $1 LIMIT 1`, email,
	).Scan(&userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", "", false, nil
		}
		return "", "", "", false, fmt.Errorf("find by email: %w", err)
	}

	// Does the user already belong to this tenant?
	var name, doc string
	err = f.db.QueryRowContext(ctx,
		`SELECT COALESCE(name, ''), COALESCE(document, '')
		   FROM "UsersOnTenants"
		  WHERE "userId" = $1 AND "tenantId" = $2
		  LIMIT 1`,
		userID, tenantID,
	).Scan(&name, &doc)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return userID, "", "", false, nil
		}
		return "", "", "", false, fmt.Errorf("find uot by userId: %w", err)
	}
	return userID, name, doc, true, nil
}

// ---------- Writes ----------

func (f *Feature) createUser(ctx context.Context, email, phone, passwordHash, magicTokenHash string, validUntil time.Time) (string, error) {
	id := utils.GenerateCUID()
	var phoneArg any
	if phone != "" {
		phoneArg = phone
	} else {
		phoneArg = nil
	}
	const q = `
		INSERT INTO "User" (id, email, phone, password, "magicToken", "magicTokenValidUntil", "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
	`
	if _, err := f.db.ExecContext(ctx, q, id, email, phoneArg, passwordHash, magicTokenHash, validUntil); err != nil {
		return "", fmt.Errorf("insert User: %w", err)
	}
	return id, nil
}

func (f *Feature) upsertUsersOnTenants(ctx context.Context, userID, tenantID, passwordHash, name, document string) error {
	var docArg any
	if document != "" {
		docArg = document
	}
	const q = `
		INSERT INTO "UsersOnTenants" ("userId", "tenantId", role, password, name, document, "assignedAt")
		VALUES ($1, $2, 'member', $3, $4, $5, NOW())
		ON CONFLICT ("userId", "tenantId") DO NOTHING
	`
	if _, err := f.db.ExecContext(ctx, q, userID, tenantID, passwordHash, name, docArg); err != nil {
		return fmt.Errorf("insert UsersOnTenants: %w", err)
	}
	return nil
}

func (f *Feature) updateUsersOnTenantsNameDoc(ctx context.Context, userID, tenantID, name, document string) error {
	var docArg any
	if document != "" {
		docArg = document
	}
	const q = `
		UPDATE "UsersOnTenants"
		SET name = COALESCE(NULLIF($3, ''), name),
		    document = COALESCE($4, document)
		WHERE "userId" = $1 AND "tenantId" = $2
	`
	if _, err := f.db.ExecContext(ctx, q, userID, tenantID, name, docArg); err != nil {
		return fmt.Errorf("update UsersOnTenants: %w", err)
	}
	return nil
}

func (f *Feature) refreshUserMagicToken(ctx context.Context, userID, tokenHash string, validUntil time.Time) error {
	const q = `UPDATE "User" SET "magicToken"=$1, "magicTokenValidUntil"=$2, "updatedAt"=NOW() WHERE id=$3`
	if _, err := f.db.ExecContext(ctx, q, tokenHash, validUntil, userID); err != nil {
		return fmt.Errorf("update User magicToken: %w", err)
	}
	return nil
}

func (f *Feature) userHasAllDeliveries(ctx context.Context, userID, tenantID string, deliveries []deliveryRef) (bool, error) {
	ids := make([]string, 0, len(deliveries))
	for _, d := range deliveries {
		ids = append(ids, d.Value)
	}
	var count int
	err := f.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM "MemberOnDelivery"
		  WHERE "memberId" = $1 AND "tenantId" = $2 AND "deliveryId" = ANY($3)`,
		userID, tenantID, pq.Array(ids),
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("count deliveries: %w", err)
	}
	return count >= len(deliveries), nil
}

func (f *Feature) insertMemberOnDeliveries(ctx context.Context, userID, tenantID string, deliveries []deliveryRef, assignedAt time.Time) error {
	// Build a single multi-row INSERT ... VALUES statement.
	if len(deliveries) == 0 {
		return nil
	}
	var (
		placeholders []string
		args         []any
	)
	for i, d := range deliveries {
		base := i * 4
		placeholders = append(placeholders,
			fmt.Sprintf("($%d, $%d, $%d, $%d)", base+1, base+2, base+3, base+4),
		)
		args = append(args, userID, d.Value, tenantID, assignedAt)
	}
	q := `INSERT INTO "MemberOnDelivery" ("memberId", "deliveryId", "tenantId", "assignedAt") VALUES ` +
		strings.Join(placeholders, ", ") +
		` ON CONFLICT ("memberId", "deliveryId") DO NOTHING`

	if _, err := f.db.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("insert MemberOnDelivery: %w", err)
	}
	return nil
}

func (f *Feature) createMagicToken(ctx context.Context, userID, tenantID, tokenRaw, email string, expires time.Time) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(tokenRaw), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash magic token: %w", err)
	}
	shortCode := randomShortCode(shortCodeLen)
	id := utils.GenerateCUID()

	const q = `
		INSERT INTO "MagicToken" (id, token, "shortCode", "userId", "tenantId", method, expires, "createdAt")
		VALUES ($1, $2, $3, $4, $5, 'admin_import', $6, NOW())
	`
	if _, err := f.db.ExecContext(ctx, q, id, string(hashed), shortCode, userID, tenantID, expires); err != nil {
		return "", fmt.Errorf("insert MagicToken: %w", err)
	}
	_ = email // reserved for future telemetry; schema currently has no column for it
	return shortCode, nil
}

// ---------- Per-row persistence + counter updates ----------

func (f *Feature) insertRowRecords(ctx context.Context, importID string, states []rowState) error {
	if len(states) == 0 {
		return nil
	}
	var (
		placeholders []string
		args         []any
	)
	for i, s := range states {
		base := i * 9
		placeholders = append(placeholders,
			fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d, NOW())",
				base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9,
			),
		)
		var (
			doc    any
			userID any
			errMsg any
		)
		if s.input.Document != "" {
			doc = s.input.Document
		}
		if s.userID != "" {
			userID = s.userID
		}
		if s.errorMessage != "" {
			errMsg = s.errorMessage
		}
		args = append(args,
			utils.GenerateCUID(),
			importID,
			s.rowIndex,
			s.input.Email,
			s.input.Name,
			doc,
			s.status,
			nullStringArg(s.emailSent),
			nullStringArg(s.emailStatus),
		)
		_ = userID
		_ = errMsg
	}

	// UserImportRow has 10 user-supplied columns + auto createdAt via default.
	// We INSERT only the columns we control; the "userId" and "errorMessage"
	// get written via a separate UPDATE below.
	q := `INSERT INTO "UserImportRow" (
		id, "importId", "rowIndex", email, name, document, status, "emailSentType", "emailStatus", "createdAt"
	) VALUES ` + strings.Join(placeholders, ", ")

	if _, err := f.db.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("insert UserImportRow: %w", err)
	}

	// Second pass: set userId / errorMessage where they exist. Cheap because
	// batches are bounded at 100.
	for _, s := range states {
		if s.userID == "" && s.errorMessage == "" {
			continue
		}
		const upd = `
			UPDATE "UserImportRow"
			SET "userId" = COALESCE($3, "userId"),
			    "errorMessage" = COALESCE($4, "errorMessage")
			WHERE "importId" = $1 AND "rowIndex" = $2
		`
		var userID, errMsg any
		if s.userID != "" {
			userID = s.userID
		}
		if s.errorMessage != "" {
			errMsg = s.errorMessage
		}
		if _, err := f.db.ExecContext(ctx, upd, importID, s.rowIndex, userID, errMsg); err != nil {
			return fmt.Errorf("update UserImportRow userId/error: %w", err)
		}
	}
	return nil
}

func nullStringArg(v sql.NullString) any {
	if !v.Valid {
		return nil
	}
	return v.String
}

func (f *Feature) updateImportCounters(ctx context.Context, importID string, processed int, c *importCounters) error {
	const q = `
		UPDATE "UserImport"
		SET "processedRows" = "processedRows" + $1,
		    "createdUsers" = $2,
		    "updatedUsers" = $3,
		    "alreadyHadAllDeliveries" = $4,
		    "skippedRows" = $5,
		    "errorRows" = $6,
		    "loginEmailsSent" = $7,
		    "deliveryEmailsSent" = $8,
		    "emailsFailed" = $9
		WHERE id = $10
	`
	_, err := f.db.ExecContext(ctx, q,
		processed,
		c.createdUsers, c.updatedUsers, c.alreadyHadAll, c.skippedRows, c.errorRows,
		c.loginEmailsSent, c.deliveryEmailsSent, c.emailsFailed,
		importID,
	)
	return err
}

func (f *Feature) finalizeStatus(importID, status, errorMessage string) error {
	const q = `
		UPDATE "UserImport"
		SET status = $1,
		    "errorMessage" = $2,
		    "finishedAt" = NOW()
		WHERE id = $3
	`
	var errArg any
	if errorMessage != "" {
		errArg = errorMessage
	}
	_, err := f.db.ExecContext(context.Background(), q, status, errArg, importID)
	return err
}

func (f *Feature) finalizeAsFailed(importID, message string) {
	if err := f.finalizeStatus(importID, "failed", message); err != nil {
		f.log.Error("import.finalize_failed_status_update_failed",
			"import_id", importID, "message", message, "error", err.Error())
	}
}

// ---------- Random generators (slice-local so no shared utility expansion) ----------

func randomBase64(bytes int) string {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		// Extremely unlikely; fall back to time-based randomness.
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return base64.URLEncoding.EncodeToString(b)
}

// randomString generates a uniform random string over `alphabet` using
// crypto/rand.Int, which is unbiased regardless of alphabet length. The
// simpler `rand.Read(b) + b[i]%len(alphabet)` pattern would skew probability
// toward the first `256 % len` characters — negligible security impact at
// these lengths but worth doing right because it's free.
func randomString(n int) string {
	const alphabet = "ABCDEFGHIJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789"
	return uniformFromAlphabet(n, alphabet)
}

func randomShortCode(n int) string {
	// 32-char alphabet — no confusable chars (I, O, 0, 1).
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	return uniformFromAlphabet(n, alphabet)
}

func uniformFromAlphabet(n int, alphabet string) string {
	max := big.NewInt(int64(len(alphabet)))
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			// crypto/rand should never fail in practice; fall back to a
			// deterministic but unpredictable-enough timestamp to keep the
			// import from halting — the caller bcrypts this before storage.
			return fmt.Sprintf("%x", time.Now().UnixNano())
		}
		out[i] = alphabet[idx.Int64()]
	}
	return string(out)
}
