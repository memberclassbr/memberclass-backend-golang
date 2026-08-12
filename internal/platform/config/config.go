// Package config loads every environment variable the service reads, once, at
// startup.
//
// Two rules drive the design:
//
//   - Required variables are validated up front and *all* missing ones are
//     reported together. A deployment with three unset variables should be
//     fixable in one pass, not three restarts.
//   - Optional variables gate a feature. A missing one never blocks boot; it
//     flips the owning section's Enabled to false and adds a line to Warnings()
//     so the log says plainly which feature is off and why.
//
// The service runs as one deployment per customer, each with its own database,
// its own Spaces bucket and its own env. Silent degradation — a warning buried
// in the log, a worker that never claims a job, an auth check that passes
// because the expected key is empty — is the expensive failure mode here, so
// anything security-relevant is required rather than optional.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the fully resolved environment. Every field is populated by Load;
// nothing reads os.Getenv after that.
type Config struct {
	App           App
	DB            DB
	Redis         Redis
	Storage       Storage
	Auth          Auth
	Public        Public
	Bunny         Bunny
	IlovePDF      IlovePDF
	Resend        Resend
	Transcription Transcription
	Notifications Notifications
	Analytics     Analytics
	Telemetry     Telemetry

	warnings []string
}

// App holds process-level settings.
type App struct {
	Port     string
	LogLevel string
	Env      string
}

// IsDevelopment reports whether response caching and other production-only
// behaviour should be disabled. Mirrors the APP_ENV check the slices used
// before this package existed.
func (a App) IsDevelopment() bool {
	switch strings.ToLower(a.Env) {
	case "development", "dev", "local":
		return true
	default:
		return false
	}
}

// DB holds the two database connections the service owns. There is exactly one
// tenant database per deployment; Transcription points at a separate Postgres
// with the pgvector extension and is optional.
type DB struct {
	Driver string
	DSN    string
}

// Redis backs the cache and the three rate limiters.
type Redis struct {
	URL string
}

// Storage is the DigitalOcean Spaces bucket for this deployment. One bucket
// per deployment: uploads always target Bucket, never a bucket derived from a
// media URL.
type Storage struct {
	AccessKey string
	SecretKey string
	Bucket    string
	URL       string
}

// Auth holds the shared secrets that gate privileged endpoints.
type Auth struct {
	// InternalAPIKey is compared against the x-internal-api-key header by the
	// admin-facing endpoints. Required: an empty expected key makes the
	// comparison trivially satisfiable.
	InternalAPIKey string
	// NextAuthSecret decrypts the NextAuth session cookie on `/api/comments`.
	// Must match NEXTAUTH_SECRET on the frontend byte-for-byte.
	NextAuthSecret string
	// GoAPIJWTSecret verifies the go-token Bearer JWTs the frontend mints for
	// the routes at the root. Must match GO_API_JWT_SECRET on that side.
	//
	// Optional, and that is the whole point: the frontend treats its own
	// GO_API_JWT_SECRET as optional too and falls back to NEXTAUTH_SECRET when
	// unset, so a backend that accepted only this one would reject every token
	// the moment it deployed ahead of the frontend's env change.
	//
	// Load enforces a length floor when it is set — see minJWTSecretBytes.
	GoAPIJWTSecret string

	// AllowLegacyJWTSecret keeps NEXTAUTH_SECRET in the set of keys a go-token
	// may be signed with. It is what makes the migration staged rather than a
	// flag day, and it runs in three states:
	//
	//  1. GO_API_JWT_SECRET unset — verified with NEXTAUTH_SECRET, as before.
	//  2. GO_API_JWT_SECRET set — both accepted. Neither side has to move
	//     first; set the same value on the frontend whenever it suits.
	//  3. GO_API_JWT_LEGACY_FALLBACK=false — only the dedicated secret.
	//
	// State 3 is the one that actually buys something, and until a deployment
	// reaches it nothing has been gained: NEXTAUTH_SECRET also derives the
	// session cookie's key, so a leaked go-token key is still a forged
	// session. States 1 and 2 both warn at boot for that reason.
	AllowLegacyJWTSecret bool
}

// minJWTSecretBytes is the floor for GO_API_JWT_SECRET.
//
// The Bearer verification is hand-rolled HMAC rather than go-jose, so nothing
// in the crypto path enforces a key length. A short secret is brute-forceable
// offline from a single captured token, and that token carries tenant-scoped
// admin access — 32 bytes is what go-jose would have required for HS256.
//
// It is not enforced on NEXTAUTH_SECRET: that value is already deployed at
// whatever length it has, and refusing to boot over it would be an outage
// rather than a fix.
const minJWTSecretBytes = 32

// Public holds the customer-facing hostnames used to build links and email
// addresses.
type Public struct {
	// DomainURL is the frontend root domain, e.g. memberclass.com.br. Bare
	// host: no scheme, no port, no path.
	//
	// It is the single source for both halves of a magic link — the `From`
	// host of the email that carries it and the root under which a tenant's
	// subdomain is built. There used to be a second variable,
	// PUBLIC_ROOT_DOMAIN, holding this service's own host:port; every
	// deployment set the two to the same value, and the one path that read it
	// produced links pointing at the backend rather than at the frontend.
	DomainURL string
	// FilesURL is the CDN prefix that resolves relative asset paths (tenant
	// logos in email templates). Optional.
	FilesURL string
}

// Bunny holds the account-level CDN credentials. Per-tenant library keys live
// in the database, not here.
type Bunny struct {
	APIKey  string
	BaseURL string
	Timeout time.Duration
}

// IlovePDF converts lesson PDFs to page images.
type IlovePDF struct {
	Enabled bool
	APIKeys []string
	BaseURL string
}

// Resend sends the transactional email for the member-import slice.
type Resend struct {
	Enabled bool
	APIKey  string
	BaseURL string
}

// Transcription is the video → Whisper → pgvector pipeline. It needs both its
// own database and an OpenAI key; with either missing the worker stays inert
// and every other feature keeps running.
type Transcription struct {
	Enabled           bool
	DSN               string
	OpenAIKey         string
	PollInterval      time.Duration
	WorkerConcurrency int

	// Panda is a second video source, rolled out to an explicit tenant
	// allowlist. Disabled independently of the rest of the pipeline.
	PandaEnabled          bool
	PandaAPIKey           string
	PandaAllowedTenantIDs []string
}

// Notifications dispatches FCM pushes. Each Firebase project has its own
// service-account JSON; the worker picks one per tenant at send time, so the
// keys are kept as a map rather than fixed fields.
type Notifications struct {
	Enabled bool
	// FirebaseServiceAccounts maps env var name to the raw JSON credential.
	// Only non-empty variables are present.
	FirebaseServiceAccounts map[string]string
}

// Analytics owns the daily and monthly rollup jobs.
type Analytics struct {
	// DeleteEnabled allows the monthly rollup to delete the raw rows it has
	// aggregated. Off by default: turning it on is destructive.
	DeleteEnabled bool
}

// Telemetry configures the OpenTelemetry exporters. It is optional in the
// strict sense: with any of the three variables unset the providers are never
// installed and the service runs uninstrumented, which is what a laptop
// without a collector needs.
//
// Endpoint is a host[:port] with no scheme. The collector runs outside this
// deployment, so the connection is TLS and Token travels inside it — a
// plaintext exporter would put the credential on the wire in the clear.
//
// Project names the customer this deployment serves. It becomes the resource's
// service.namespace rather than its service.name: every deployment reports the
// same service name so the fleet can be read as one service, and the namespace
// is what tells two customers apart.
type Telemetry struct {
	Enabled  bool
	Endpoint string
	Token    string
	Project  string

	// Version and InstanceID are best-effort. Railway injects them into every
	// container; anywhere else they are empty and simply left off the resource.
	Version    string
	InstanceID string
}

// firebaseKeyEnvVars are the service-account variables the notifications
// worker may look up, one per Firebase project.
var firebaseKeyEnvVars = []string{
	"FIREBASE_SERVICE_ACCOUNT_KEY",
	"FIREBASE_SERVICE_ACCOUNT_KEY_2",
	"FIREBASE_SERVICE_ACCOUNT_KEY_3",
	"FIREBASE_SERVICE_ACCOUNT_KEY_4",
	"FIREBASE_SERVICE_ACCOUNT_KEY_5",
}

// Load reads the environment and validates it. It returns an error naming every
// missing required variable at once; callers should treat that as fatal.
func Load() (*Config, error) {
	var missing []string
	required := func(key string, fallbacks ...string) string {
		v := lookup(key, fallbacks...)
		if v == "" {
			missing = append(missing, key)
		}
		return v
	}

	cfg := &Config{
		App: App{
			Port:     optional("PORT", "8181"),
			LogLevel: optional("LOG_LEVEL", "INFO"),
			Env:      optional("APP_ENV", "production"),
		},
		DB: DB{
			Driver: optional("DB_DRIVER", "postgres"),
			DSN:    required("DB_DSN"),
		},
		Redis: Redis{
			URL: required("UPSTASH_REDIS_URL"),
		},
		Storage: Storage{
			AccessKey: required("DO_SPACES_ID"),
			SecretKey: required("DO_SPACES_SECRET"),
			Bucket:    required("DO_SPACES_BUCKET"),
			URL:       required("DO_SPACES_URL"),
		},
		Auth: Auth{
			InternalAPIKey: required("INTERNAL_AI_API_KEY"),
			NextAuthSecret: required("NEXTAUTH_SECRET"),
			GoAPIJWTSecret: lookup("GO_API_JWT_SECRET"),
			// Defaults to on: a deployment that has not been told the frontend
			// has migrated must keep accepting what the frontend signs today.
			AllowLegacyJWTSecret: optional("GO_API_JWT_LEGACY_FALLBACK", "true") != "false",
		},
		Public: Public{
			DomainURL: required("PUBLIC_DOMAIN_URL", "NEXT_PUBLIC_DOMAIN_URL"),
			FilesURL:  lookup("PUBLIC_FILES_URL", "NEXT_PUBLIC_FILES_URL"),
		},
		Bunny: Bunny{
			APIKey:  os.Getenv("BUNNY_API_KEY"),
			BaseURL: optional("BUNNY_BASE_URL", "https://video.bunnycdn.com/library/"),
			Timeout: durationSeconds("BUNNY_TIMEOUT_SECONDS", 30*time.Second),
		},
		Analytics: Analytics{
			DeleteEnabled: os.Getenv("ANALYTICS_DELETE_ENABLED") == "true",
		},
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	if n := len(cfg.Auth.GoAPIJWTSecret); n > 0 && n < minJWTSecretBytes {
		return nil, fmt.Errorf(
			"GO_API_JWT_SECRET must be at least %d bytes, got %d — a shorter HMAC key is brute-forceable offline from one captured token",
			minJWTSecretBytes, n,
		)
	}
	// Both of these say the same thing — a leaked go-token key is still the
	// session key — and both stop once a deployment reaches state 3.
	switch {
	case cfg.Auth.GoAPIJWTSecret == "":
		cfg.warnings = append(cfg.warnings,
			"go-token JWTs are verified with NEXTAUTH_SECRET: set GO_API_JWT_SECRET "+
				"(>= 32 bytes, the same value on the frontend) so a leaked API key is not also the session key")
	case cfg.Auth.AllowLegacyJWTSecret:
		cfg.warnings = append(cfg.warnings,
			"go-token JWTs still accept NEXTAUTH_SECRET as a fallback: once the frontend signs with "+
				"GO_API_JWT_SECRET, set GO_API_JWT_LEGACY_FALLBACK=false to close the window")
	}

	cfg.loadIlovePDF()
	cfg.loadResend()
	cfg.loadTranscription()
	cfg.loadNotifications()
	cfg.loadTelemetry()

	return cfg, nil
}

// Warnings returns one line per disabled feature, explaining which variable is
// responsible. Callers should log these at startup — they are the only signal
// that a deployment is running with a feature switched off.
func (c *Config) Warnings() []string {
	return c.warnings
}

func (c *Config) disable(feature, reason string) {
	c.warnings = append(c.warnings, fmt.Sprintf("%s disabled: %s", feature, reason))
}

func (c *Config) loadIlovePDF() {
	c.IlovePDF.BaseURL = optional("ILOVEPDF_BASE_URL", "https://api.ilovepdf.com/v1")

	raw := os.Getenv("ILOVEPDF_API_KEYS")
	if raw == "" {
		c.disable("PDF processing", "ILOVEPDF_API_KEYS not set")
		return
	}
	for _, key := range strings.Split(raw, ",") {
		if key = strings.TrimSpace(key); key != "" {
			c.IlovePDF.APIKeys = append(c.IlovePDF.APIKeys, key)
		}
	}
	if len(c.IlovePDF.APIKeys) == 0 {
		c.disable("PDF processing", "ILOVEPDF_API_KEYS contains no usable keys")
		return
	}
	c.IlovePDF.Enabled = true
}

func (c *Config) loadResend() {
	c.Resend.BaseURL = optional("RESEND_BASE_URL", "https://api.resend.com")

	c.Resend.APIKey = os.Getenv("RESEND_API_KEY")
	if c.Resend.APIKey == "" {
		c.disable("transactional email", "RESEND_API_KEY not set")
		return
	}
	c.Resend.Enabled = true
}

func (c *Config) loadTranscription() {
	c.Transcription.PollInterval = durationSeconds("TRANSCRIPTION_POLL_INTERVAL_SECONDS", 30*time.Second)
	c.Transcription.WorkerConcurrency = intOr("TRANSCRIPTION_WORKER_CONCURRENCY", 2)
	c.Transcription.DSN = os.Getenv("DB_TRANSCRIPTION_DSN")
	c.Transcription.OpenAIKey = os.Getenv("OPENAI_API_KEY")

	switch {
	case c.Transcription.DSN == "":
		c.disable("transcription", "DB_TRANSCRIPTION_DSN not set")
	case c.Transcription.OpenAIKey == "":
		c.disable("transcription", "OPENAI_API_KEY not set")
	default:
		c.Transcription.Enabled = true
	}

	c.Transcription.PandaAPIKey = os.Getenv("PANDA_API_KEY")
	for _, id := range strings.Split(os.Getenv("PANDA_ALLOWED_TENANT_IDS"), ",") {
		if id = strings.TrimSpace(id); id != "" {
			c.Transcription.PandaAllowedTenantIDs = append(c.Transcription.PandaAllowedTenantIDs, id)
		}
	}

	switch {
	case c.Transcription.PandaAPIKey == "":
		c.disable("Panda video source", "PANDA_API_KEY not set")
	case len(c.Transcription.PandaAllowedTenantIDs) == 0:
		c.disable("Panda video source", "PANDA_ALLOWED_TENANT_IDS is empty")
	default:
		c.Transcription.PandaEnabled = true
	}
}

func (c *Config) loadNotifications() {
	c.Notifications.FirebaseServiceAccounts = make(map[string]string)
	for _, envVar := range firebaseKeyEnvVars {
		if raw := os.Getenv(envVar); raw != "" {
			c.Notifications.FirebaseServiceAccounts[envVar] = raw
		}
	}
	if len(c.Notifications.FirebaseServiceAccounts) == 0 {
		c.disable("push notifications", "no FIREBASE_SERVICE_ACCOUNT_KEY* variable is set")
		return
	}
	c.Notifications.Enabled = true
}

func (c *Config) loadTelemetry() {
	c.Telemetry.Endpoint = strings.TrimSpace(os.Getenv("OTEL_ENDPOINT_HOST"))
	c.Telemetry.Token = strings.TrimSpace(os.Getenv("OTEL_TOKEN"))
	c.Telemetry.Project = strings.TrimSpace(os.Getenv("OTEL_PROJECT"))
	c.Telemetry.Version = strings.TrimSpace(os.Getenv("RAILWAY_GIT_COMMIT_SHA"))
	c.Telemetry.InstanceID = strings.TrimSpace(os.Getenv("RAILWAY_REPLICA_ID"))

	switch {
	case c.Telemetry.Endpoint == "":
		c.disable("telemetry", "OTEL_ENDPOINT_HOST not set")
	case c.Telemetry.Token == "":
		c.disable("telemetry", "OTEL_TOKEN not set")
	case c.Telemetry.Project == "":
		c.disable("telemetry", "OTEL_PROJECT not set")
	default:
		c.Telemetry.Enabled = true
	}
}

// lookup returns the first non-empty value among key and its fallbacks. The
// fallbacks exist because this service shares a .env file with the Next.js
// frontend, which prefixes its own variables with NEXT_PUBLIC_.
func lookup(key string, fallbacks ...string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	for _, fallback := range fallbacks {
		if v := strings.TrimSpace(os.Getenv(fallback)); v != "" {
			return v
		}
	}
	return ""
}

func optional(key, def string) string {
	if v := lookup(key); v != "" {
		return v
	}
	return def
}

func intOr(key string, def int) int {
	v, err := strconv.Atoi(lookup(key))
	if err != nil || v <= 0 {
		return def
	}
	return v
}

func durationSeconds(key string, def time.Duration) time.Duration {
	seconds := intOr(key, -1)
	if seconds <= 0 {
		return def
	}
	return time.Duration(seconds) * time.Second
}
