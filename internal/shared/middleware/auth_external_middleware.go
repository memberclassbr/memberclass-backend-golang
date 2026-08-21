// Package middleware holds the HTTP middlewares shared across slices: the
// three credential checks and the three rate limiters. They live here rather
// than inside a slice because every slice composes them, and none owns them.
package middleware

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/memberclass-backend-golang/internal/platform/apikeyusage"
	"github.com/memberclass-backend-golang/internal/platform/logger"
	"github.com/memberclass-backend-golang/internal/shared/tenant"
)

// sqlTenantByToken resolves a hashed API key to the key's id and its tenant.
//
// Two things about this query are load-bearing. The expiry is compared here,
// in the same statement that authenticates, rather than by a job that
// materialises a status: any gap between the two is a window where an expired
// key still works. And now() is the database's clock, not the container's —
// the panel derives its "Expired" label from the same clock, and a drifting
// container would make the screen and the gate disagree.
const sqlTenantByToken = `
	SELECT k.id, t.id, t.name
	FROM "TenantApiKey" k
	JOIN "Tenant" t ON t.id = k."tenantId"
	WHERE k."tokenHash" = $1
	  AND (k."expiresAt" IS NULL OR k."expiresAt" > now())
	LIMIT 1
`

// sqlTenantByLegacyToken is the pre-"TenantApiKey" lookup, kept as a fallback
// so this service and the panel can deploy in either order.
//
// Without it, the backfill that copies these hashes into "TenantApiKey" would
// have to run on every customer's database *before* this service deploys
// there; get the order wrong for one of them and every integration that
// customer has answers 401, indistinguishably from a wrong key.
//
// It is temporary, and its removal is tracked in issue #38 — from 2026-09-20,
// once the apikey.auth.legacy_fallback counter has stayed at zero everywhere.
// A key that only exists here has no id and no expiry, so it is counted in no
// usage panel and never expires: two more reasons this is a bridge, not a
// second supported way to hold a key.
const sqlTenantByLegacyToken = `
	SELECT id, name
	FROM "Tenant"
	WHERE token_api_auth = $1
	LIMIT 1
`

const (
	// APIKeyHeader is the header a tenant authenticates with.
	APIKeyHeader = "x-api-key"

	// LegacyAPIKeyHeader is the name the same key used to be sent under. It is
	// still accepted, and unadvertised: swagger documents only APIKeyHeader.
	// Dropping it would log out every integration in one deploy, so it goes
	// when the callers do, not before.
	LegacyAPIKeyHeader = "mc-api-key"
)

// usageRecorder is the part of apikeyusage this middleware uses. It is an
// interface so a test can assert what was recorded without a Redis.
type usageRecorder interface {
	Record(ctx context.Context, apiKeyID, endpoint string, status int)
}

// AuthExternalMiddleware authenticates a tenant by its API key, read from
// x-api-key or, for callers that have not migrated, mc-api-key. The header
// carries the plaintext key; the database stores its SHA-256, so the
// middleware hashes before looking up.
//
// It is also where per-key usage is counted, because it is the only place that
// knows which key a request arrived with. Counting on the way back out — above
// the rate limiter, which is mounted below this — is what makes a 429 show up
// in the errors column instead of vanishing.
type AuthExternalMiddleware struct {
	db    *sql.DB
	log   logger.Logger
	usage usageRecorder

	// legacyFallbacks counts requests that only authenticated because of
	// sqlTenantByLegacyToken. It is the evidence issue #38 closes on: a
	// deployment nobody has backfilled looks exactly like a healthy one from
	// the outside, so "is anyone still on the old column?" has to be a number
	// rather than a guess.
	legacyFallbacks metric.Int64Counter

	// warnedLegacy keeps the log line to one per tenant per process. The
	// counter answers how much; the log answers who, and an un-backfilled
	// deployment serves every request through this path — logging each one
	// would bury everything else.
	warnedLegacy sync.Map
}

// NewAuthExternalMiddleware builds the middleware. usage may be nil, which
// disables counting and leaves authentication untouched.
func NewAuthExternalMiddleware(db *sql.DB, log logger.Logger, usage usageRecorder) *AuthExternalMiddleware {
	m := &AuthExternalMiddleware{db: db, log: log, usage: usage}

	meter := otel.GetMeterProvider().Meter("github.com/memberclass-backend-golang/internal/shared/middleware")
	counter, err := meter.Int64Counter(
		"apikey.auth.legacy_fallback",
		metric.WithDescription("Requests authenticated by the legacy Tenant.token_api_auth column"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		// An instrument that will not build is not a reason to refuse to
		// authenticate anyone.
		log.Warn("Legacy API key fallback metrics unavailable: " + err.Error())
	} else {
		m.legacyFallbacks = counter
	}

	return m
}

// Authenticate rejects the request unless the key resolves to a tenant, and
// puts that tenant in the request context for downstream handlers.
func (m *AuthExternalMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := apiKeyFrom(r)
		if key == "" {
			// A missing header and a wrong key are told apart, which gives
			// away nothing: a caller that sent no credential already knows it
			// sent none. A caller that sent one still cannot learn whether the
			// value exists — expired and unknown answer identically below.
			sendAPIKeyError(w, "Header "+APIKeyHeader+" é obrigatório", "MISSING_API_KEY")
			return
		}

		apiKeyID, found, err := m.tenantByKey(r.Context(), key)
		if err != nil {
			// A lookup failure, an unknown key and an expired key are reported
			// identically: the response must not tell a caller whether a key
			// exists, or whether it ever did.
			sendAPIKeyError(w, "API key inválida", "INVALID_API_KEY")
			return
		}

		ctx := context.WithValue(r.Context(), tenant.ContextKey, found)

		// The id stays in this closure. It is deliberately not put in the
		// context: no handler scopes anything by key — every key is valid for
		// every endpoint of its tenant — and a value in the context is an
		// invitation to start.
		wrapped := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(wrapped, r.WithContext(ctx))

		m.recordUsage(r, apiKeyID, wrapped.Status())
	})
}

// recordUsage counts the finished request. The route pattern is only resolved
// once chi has descended into the matching route, which is why this runs after
// the handler rather than before it.
//
// A request authenticated through the legacy column arrives here with an empty
// apiKeyID and is counted nowhere: there is no key row for the panel to hang
// the number on.
func (m *AuthExternalMiddleware) recordUsage(r *http.Request, apiKeyID string, status int) {
	if m.usage == nil || apiKeyID == "" {
		return
	}

	endpoint := apikeyusage.Endpoint(chi.RouteContext(r.Context()).RoutePattern())
	if endpoint == "" {
		return
	}

	// A handler that wrote nothing left the status at zero; net/http sends 200
	// in that case, so that is what is counted.
	if status == 0 {
		status = http.StatusOK
	}

	m.usage.Record(r.Context(), apiKeyID, endpoint, status)
}

// apiKeyFrom reads the tenant key, preferring the current header name.
//
// A caller sending both wins with x-api-key rather than being rejected for the
// ambiguity: during a migration the two are the same key, and failing a request
// that carries a valid credential would be a worse answer than picking the
// header we are asking everyone to move to.
func apiKeyFrom(r *http.Request) string {
	if key := r.Header.Get(APIKeyHeader); key != "" {
		return key
	}
	return r.Header.Get(LegacyAPIKeyHeader)
}

// tenantByKey resolves the key, trying "TenantApiKey" first and the legacy
// column only if that finds nothing.
//
// The fallback is taken on any failure of the first lookup, not just on
// ErrNoRows, because the failure a fallback exists for is precisely the one
// that is not a missing row: a deployment whose panel migration has not run
// yet has no "TenantApiKey" table at all, and the error it raises must not be
// the thing that logs out every integration that customer has.
func (m *AuthExternalMiddleware) tenantByKey(ctx context.Context, key string) (string, *tenant.Tenant, error) {
	sum := sha256.Sum256([]byte(key))
	hash := hex.EncodeToString(sum[:])

	var (
		apiKeyID string
		found    tenant.Tenant
	)
	err := m.db.QueryRowContext(ctx, sqlTenantByToken, hash).Scan(&apiKeyID, &found.ID, &found.Name)
	switch {
	case err == nil:
		return apiKeyID, &found, nil
	case !errors.Is(err, sql.ErrNoRows):
		m.log.Error("Error finding tenant API key: " + err.Error())
	}

	legacy, legacyErr := m.tenantByLegacyToken(ctx, hash)
	if legacyErr != nil {
		// The first error is the interesting one unless it was simply "no such
		// key", so it is what the caller sees; either way the response is the
		// same.
		return "", nil, err
	}

	m.noteLegacyFallback(ctx, legacy)
	return "", legacy, nil
}

func (m *AuthExternalMiddleware) tenantByLegacyToken(ctx context.Context, hash string) (*tenant.Tenant, error) {
	var found tenant.Tenant
	err := m.db.QueryRowContext(ctx, sqlTenantByLegacyToken, hash).Scan(&found.ID, &found.Name)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, err
	case err != nil:
		m.log.Error("Error finding tenant with legacy token: " + err.Error())
		return nil, err
	}
	return &found, nil
}

func (m *AuthExternalMiddleware) noteLegacyFallback(ctx context.Context, found *tenant.Tenant) {
	if m.legacyFallbacks != nil {
		m.legacyFallbacks.Add(ctx, 1)
	}

	if _, warned := m.warnedLegacy.LoadOrStore(found.ID, struct{}{}); warned {
		return
	}
	m.log.Warn(
		"Tenant authenticated by legacy token_api_auth: tenant " + found.ID +
			" has no row in \"TenantApiKey\". Run the panel's backfill for this deployment; " +
			"usage for this key is counted nowhere until then.",
	)
}

func sendAPIKeyError(w http.ResponseWriter, message, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":        false,
		"error":     message,
		"errorCode": code,
	})
}
