package config

import (
	"strings"
	"testing"
	"time"
)

// requiredEnv is the minimum set that lets Load succeed. Tests start from this
// and remove or add one variable at a time.
var requiredEnv = map[string]string{
	"DB_DSN":              "postgres://localhost/app",
	"UPSTASH_REDIS_URL":   "redis://localhost:6379",
	"DO_SPACES_ID":        "spaces-id",
	"DO_SPACES_SECRET":    "spaces-secret",
	"DO_SPACES_BUCKET":    "tenant-bucket",
	"DO_SPACES_URL":       "https://sfo3.digitaloceanspaces.com",
	"INTERNAL_AI_API_KEY": "internal-key",
	"NEXTAUTH_SECRET":     "nextauth-secret",
	"PUBLIC_DOMAIN_URL":   "customer.com.br",
}

// setEnv applies the required set plus any overrides. An empty override value
// clears the variable, which is how a test simulates "not configured".
func setEnv(t *testing.T, overrides map[string]string) {
	t.Helper()
	for k, v := range requiredEnv {
		t.Setenv(k, v)
	}
	// Optional variables may leak in from the developer's own environment;
	// clear every one this package reads so tests are deterministic.
	for _, k := range []string{
		"APP_ENV", "PORT", "LOG_LEVEL", "DB_DRIVER",
		"PUBLIC_FILES_URL", "NEXT_PUBLIC_FILES_URL", "NEXT_PUBLIC_DOMAIN_URL",
		"PUBLIC_API_NAME", "PUBLIC_API_URL",
		"BUNNY_API_KEY", "BUNNY_BASE_URL", "BUNNY_TIMEOUT_SECONDS",
		"ILOVEPDF_API_KEYS", "ILOVEPDF_BASE_URL",
		"RESEND_API_KEY", "RESEND_BASE_URL",
		"DB_TRANSCRIPTION_DSN", "OPENAI_API_KEY",
		"TRANSCRIPTION_POLL_INTERVAL_SECONDS", "TRANSCRIPTION_WORKER_CONCURRENCY",
		"PANDA_API_KEY", "PANDA_ALLOWED_TENANT_IDS",
		"ANALYTICS_DELETE_ENABLED",
		"GO_API_JWT_SECRET", "GO_API_JWT_LEGACY_FALLBACK",
		"OTEL_ENDPOINT_HOST", "OTEL_TOKEN", "OTEL_PROJECT", "OTEL_INSECURE",
		"OTEL_EXPORT_INTERVAL_SECONDS", "OTEL_SERVICE_INSTANCE_ID",
		"RAILWAY_REPLICA_ID", "RAILWAY_GIT_COMMIT_SHA",
	} {
		t.Setenv(k, "")
	}
	for _, k := range firebaseKeyEnvVars {
		t.Setenv(k, "")
	}
	for k, v := range overrides {
		t.Setenv(k, v)
	}
}

func TestLoad_MissingRequiredAreReportedTogether(t *testing.T) {
	setEnv(t, map[string]string{
		"DB_DSN":          "",
		"NEXTAUTH_SECRET": "",
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected Load to fail with two required variables unset")
	}
	for _, want := range []string{"DB_DSN", "NEXTAUTH_SECRET"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

// An empty INTERNAL_AI_API_KEY used to make the admin endpoints compare a
// missing header against an empty string and pass. Boot must fail instead.
func TestLoad_RejectsEmptyAuthSecrets(t *testing.T) {
	for _, key := range []string{"INTERNAL_AI_API_KEY", "NEXTAUTH_SECRET"} {
		t.Run(key, func(t *testing.T) {
			setEnv(t, map[string]string{key: ""})

			if _, err := Load(); err == nil {
				t.Fatalf("expected Load to fail when %s is empty", key)
			}
		})
	}
}

func TestLoad_Defaults(t *testing.T) {
	setEnv(t, nil)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.App.Port != "8181" {
		t.Errorf("Port = %q, want 8181", cfg.App.Port)
	}
	if cfg.DB.Driver != "postgres" {
		t.Errorf("DB.Driver = %q, want postgres", cfg.DB.Driver)
	}
	if cfg.App.IsDevelopment() {
		t.Error("IsDevelopment = true with APP_ENV unset; production must be the default")
	}
	if cfg.Bunny.Timeout != 30*time.Second {
		t.Errorf("Bunny.Timeout = %v, want 30s", cfg.Bunny.Timeout)
	}
	if cfg.Transcription.WorkerConcurrency != 2 {
		t.Errorf("WorkerConcurrency = %d, want 2", cfg.Transcription.WorkerConcurrency)
	}
	if cfg.Analytics.DeleteEnabled {
		t.Error("Analytics.DeleteEnabled = true by default; deletion must be opt-in")
	}
}

func TestLoad_OptionalFeaturesGateOffWithWarnings(t *testing.T) {
	setEnv(t, nil)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Transcription.Enabled {
		t.Error("Transcription enabled without DB_TRANSCRIPTION_DSN")
	}
	if cfg.Resend.Enabled {
		t.Error("Resend enabled without RESEND_API_KEY")
	}
	if cfg.IlovePDF.Enabled {
		t.Error("IlovePDF enabled without ILOVEPDF_API_KEYS")
	}
	if cfg.Notifications.Enabled {
		t.Error("Notifications enabled without any Firebase key")
	}

	warnings := strings.Join(cfg.Warnings(), "\n")
	for _, want := range []string{
		"DB_TRANSCRIPTION_DSN", "RESEND_API_KEY", "ILOVEPDF_API_KEYS", "FIREBASE_SERVICE_ACCOUNT_KEY",
	} {
		if !strings.Contains(warnings, want) {
			t.Errorf("warnings do not explain %s:\n%s", want, warnings)
		}
	}
}

func TestLoad_TranscriptionNeedsBothDSNAndOpenAIKey(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		wantEnabled bool
	}{
		{
			name:        "dsn only",
			env:         map[string]string{"DB_TRANSCRIPTION_DSN": "postgres://vector"},
			wantEnabled: false,
		},
		{
			name:        "key only",
			env:         map[string]string{"OPENAI_API_KEY": "sk-test"},
			wantEnabled: false,
		},
		{
			name: "both",
			env: map[string]string{
				"DB_TRANSCRIPTION_DSN": "postgres://vector",
				"OPENAI_API_KEY":       "sk-test",
			},
			wantEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setEnv(t, tt.env)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.Transcription.Enabled != tt.wantEnabled {
				t.Errorf("Transcription.Enabled = %v, want %v", cfg.Transcription.Enabled, tt.wantEnabled)
			}
		})
	}
}

func TestLoad_PandaGatesOnKeyAndAllowlist(t *testing.T) {
	setEnv(t, map[string]string{
		"PANDA_API_KEY":            "panda-key",
		"PANDA_ALLOWED_TENANT_IDS": " tenant-a , ,tenant-b ",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !cfg.Transcription.PandaEnabled {
		t.Fatal("PandaEnabled = false with key and allowlist set")
	}
	got := cfg.Transcription.PandaAllowedTenantIDs
	if len(got) != 2 || got[0] != "tenant-a" || got[1] != "tenant-b" {
		t.Errorf("PandaAllowedTenantIDs = %q, want [tenant-a tenant-b] with blanks dropped", got)
	}
}

func TestLoad_PandaDisabledWhenAllowlistIsEmpty(t *testing.T) {
	setEnv(t, map[string]string{
		"PANDA_API_KEY":            "panda-key",
		"PANDA_ALLOWED_TENANT_IDS": " , ",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Transcription.PandaEnabled {
		t.Error("PandaEnabled = true with an allowlist of only blanks")
	}
}

// The service shares a .env with the Next.js frontend, which prefixes its own
// variables. The fallback keeps a deployment that only sets the NEXT_PUBLIC_
// form from failing to boot.
func TestLoad_NextPublicFallbacks(t *testing.T) {
	setEnv(t, map[string]string{
		"PUBLIC_DOMAIN_URL":      "",
		"NEXT_PUBLIC_DOMAIN_URL": "customer.com.br",
		"NEXT_PUBLIC_FILES_URL":  "https://files.customer.com.br",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Public.DomainURL != "customer.com.br" {
		t.Errorf("Public.DomainURL = %q, want the NEXT_PUBLIC_ fallback", cfg.Public.DomainURL)
	}
	if cfg.Public.FilesURL != "https://files.customer.com.br" {
		t.Errorf("Public.FilesURL = %q, want the NEXT_PUBLIC_ fallback", cfg.Public.FilesURL)
	}
}

func TestLoad_InvalidNumericValuesFallBackToDefaults(t *testing.T) {
	setEnv(t, map[string]string{
		"BUNNY_TIMEOUT_SECONDS":            "not-a-number",
		"TRANSCRIPTION_WORKER_CONCURRENCY": "0",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Bunny.Timeout != 30*time.Second {
		t.Errorf("Bunny.Timeout = %v, want the 30s default", cfg.Bunny.Timeout)
	}
	if cfg.Transcription.WorkerConcurrency != 2 {
		t.Errorf("WorkerConcurrency = %d, want the default 2", cfg.Transcription.WorkerConcurrency)
	}
}

func TestApp_IsDevelopment(t *testing.T) {
	tests := map[string]bool{
		"development": true,
		"dev":         true,
		"local":       true,
		"LOCAL":       true,
		"production":  false,
		"staging":     false,
		"":            false,
	}

	for env, want := range tests {
		if got := (App{Env: env}).IsDevelopment(); got != want {
			t.Errorf("IsDevelopment(%q) = %v, want %v", env, got, want)
		}
	}
}

// A short HMAC key is brute-forceable offline from a single captured token, and
// that token carries tenant-scoped admin access. Load refuses to start rather
// than run with one.
func TestLoad_RejectsAShortGoAPISecret(t *testing.T) {
	setEnv(t, map[string]string{"GO_API_JWT_SECRET": "too-short"})

	_, err := Load()

	if err == nil {
		t.Fatal("Load accepted a GO_API_JWT_SECRET below the length floor")
	}
	if !strings.Contains(err.Error(), "GO_API_JWT_SECRET") {
		t.Errorf("error does not name the offending variable: %v", err)
	}
}

// GO_API_JWT_SECRET is optional because the frontend's own copy is: it falls
// back to signing with NEXTAUTH_SECRET when unset, so a backend that required
// this one would reject every token the moment it deployed first.
func TestLoad_GoAPISecretIsOptional(t *testing.T) {
	setEnv(t, map[string]string{"GO_API_JWT_SECRET": ""})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Auth.GoAPIJWTSecret != "" {
		t.Errorf("Auth.GoAPIJWTSecret = %q, want empty", cfg.Auth.GoAPIJWTSecret)
	}
	if !cfg.Auth.AllowLegacyJWTSecret {
		t.Error("the legacy fallback must default on, or an unset GO_API_JWT_SECRET locks everyone out")
	}
}

// The go-token secret is its own variable so that leaking it does not also leak
// the key the session cookie is derived from — but only once the fallback is
// closed, which is why both earlier states warn.
func TestLoad_WarnsWhileTheSessionSecretIsStillAccepted(t *testing.T) {
	t.Run("no dedicated secret", func(t *testing.T) {
		setEnv(t, map[string]string{"GO_API_JWT_SECRET": ""})

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if !strings.Contains(strings.Join(cfg.Warnings(), "\n"), "GO_API_JWT_SECRET") {
			t.Errorf("boot log does not name the variable to set:\n%s", strings.Join(cfg.Warnings(), "\n"))
		}
	})

	t.Run("fallback still open", func(t *testing.T) {
		setEnv(t, map[string]string{"GO_API_JWT_SECRET": "go-api-jwt-secret-at-least-32-bytes"})

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if !strings.Contains(strings.Join(cfg.Warnings(), "\n"), "GO_API_JWT_LEGACY_FALLBACK") {
			t.Errorf("boot log does not name the flag that closes the window:\n%s", strings.Join(cfg.Warnings(), "\n"))
		}
	})

	t.Run("fallback closed is quiet", func(t *testing.T) {
		setEnv(t, map[string]string{
			"GO_API_JWT_SECRET":          "go-api-jwt-secret-at-least-32-bytes",
			"GO_API_JWT_LEGACY_FALLBACK": "false",
		})

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Auth.AllowLegacyJWTSecret {
			t.Error("AllowLegacyJWTSecret = true with GO_API_JWT_LEGACY_FALLBACK=false")
		}
		if strings.Contains(strings.Join(cfg.Warnings(), "\n"), "GO_API_JWT") {
			t.Errorf("still warning about the secret in the end state:\n%s", strings.Join(cfg.Warnings(), "\n"))
		}
	})
}

// The docs are the one surface a customer reads verbatim, so a deployment that
// never set its brand must say so at boot rather than serve generic docs
// silently.
func TestLoad_WarnsWhenTheDocsBrandingIsUnset(t *testing.T) {
	setEnv(t, nil)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	warnings := strings.Join(cfg.Warnings(), "\n")
	for _, want := range []string{"PUBLIC_API_NAME", "PUBLIC_API_URL"} {
		if !strings.Contains(warnings, want) {
			t.Errorf("boot log does not name %s:\n%s", want, warnings)
		}
	}
}

func TestLoad_DocsBrandingIsSilentWhenConfigured(t *testing.T) {
	setEnv(t, map[string]string{
		"PUBLIC_API_NAME": "Ephra",
		"PUBLIC_API_URL":  "https://api.ephra.com.br",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Public.APIName != "Ephra" || cfg.Public.APIURL != "https://api.ephra.com.br" {
		t.Errorf("Public = %+v, want the configured brand and URL", cfg.Public)
	}
	if warnings := strings.Join(cfg.Warnings(), "\n"); strings.Contains(warnings, "PUBLIC_API_") {
		t.Errorf("still warning about configured variables:\n%s", warnings)
	}
}

func TestNormalizeOTLPEndpoint(t *testing.T) {
	cases := []struct {
		raw          string
		wantEndpoint string
		wantInsecure bool
		wantDropped  string
	}{
		{"otel.example.com", "otel.example.com", false, ""},
		{"  otel.example.com:4318  ", "otel.example.com:4318", false, ""},
		// The exporters build https://<endpoint>/v1/metrics themselves. A
		// scheme left in the value becomes part of the Host header and every
		// export fails against a host nobody typed.
		{"https://otel.example.com", "otel.example.com", false, ""},
		{"https://otel.example.com/", "otel.example.com", false, ""},
		// http:// is an explicit request for plaintext: stripping the scheme
		// and then negotiating TLS anyway would just fail the handshake.
		{"http://10.0.0.4:4318", "10.0.0.4:4318", true, ""},
		{"http://10.0.0.4:4318/", "10.0.0.4:4318", true, ""},
		// A path is ignored rather than prefixed, so it comes back for the
		// caller to warn about instead of vanishing.
		{"https://otel.example.com/otlp", "otel.example.com", false, "/otlp"},
		{"", "", false, ""},
	}

	for _, tc := range cases {
		endpoint, insecure, dropped := normalizeOTLPEndpoint(tc.raw)
		if endpoint != tc.wantEndpoint || insecure != tc.wantInsecure || dropped != tc.wantDropped {
			t.Errorf("normalizeOTLPEndpoint(%q) = (%q, %v, %q), want (%q, %v, %q)",
				tc.raw, endpoint, insecure, dropped,
				tc.wantEndpoint, tc.wantInsecure, tc.wantDropped)
		}
	}
}

func TestLoad_TelemetryNeedsAllThreeVariables(t *testing.T) {
	base := map[string]string{
		"OTEL_ENDPOINT_HOST": "otel.example.com",
		"OTEL_TOKEN":         "collector-token",
		"OTEL_PROJECT":       "acme",
	}

	for _, missing := range []string{"OTEL_ENDPOINT_HOST", "OTEL_TOKEN", "OTEL_PROJECT"} {
		env := map[string]string{}
		for k, v := range base {
			env[k] = v
		}
		env[missing] = ""

		setEnv(t, env)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.Telemetry.Enabled {
			t.Errorf("telemetry enabled with %s unset", missing)
		}
		if warnings := strings.Join(cfg.Warnings(), "\n"); !strings.Contains(warnings, missing) {
			t.Errorf("boot log does not name %s:\n%s", missing, warnings)
		}
	}
}

func TestLoad_TelemetryDefaultsAndOverrides(t *testing.T) {
	setEnv(t, map[string]string{
		"OTEL_ENDPOINT_HOST":     "https://otel.example.com",
		"OTEL_TOKEN":             "collector-token",
		"OTEL_PROJECT":           "acme",
		"RAILWAY_REPLICA_ID":     "replica-7",
		"RAILWAY_GIT_COMMIT_SHA": "abc123",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.Telemetry.Enabled {
		t.Fatal("telemetry should be enabled with all three variables set")
	}
	if cfg.Telemetry.Endpoint != "otel.example.com" {
		t.Errorf("Endpoint = %q, want the scheme stripped", cfg.Telemetry.Endpoint)
	}
	if cfg.Telemetry.Insecure {
		t.Error("an https:// endpoint must not enable plaintext export")
	}
	if cfg.Telemetry.ExportInterval != 60*time.Second {
		t.Errorf("ExportInterval = %v, want 60s", cfg.Telemetry.ExportInterval)
	}
	if cfg.Telemetry.InstanceID != "replica-7" {
		t.Errorf("InstanceID = %q, want the platform's replica id", cfg.Telemetry.InstanceID)
	}
	if cfg.Telemetry.Version != "abc123" {
		t.Errorf("Version = %q, want the commit sha", cfg.Telemetry.Version)
	}

	// An explicit id wins, so a platform that names replicas badly can be
	// overridden without a code change.
	setEnv(t, map[string]string{
		"OTEL_ENDPOINT_HOST":           "otel.example.com",
		"OTEL_TOKEN":                   "collector-token",
		"OTEL_PROJECT":                 "acme",
		"RAILWAY_REPLICA_ID":           "replica-7",
		"OTEL_SERVICE_INSTANCE_ID":     "chosen-by-hand",
		"OTEL_EXPORT_INTERVAL_SECONDS": "15",
	})
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Telemetry.InstanceID != "chosen-by-hand" {
		t.Errorf("InstanceID = %q, want the explicit override", cfg.Telemetry.InstanceID)
	}
	if cfg.Telemetry.ExportInterval != 15*time.Second {
		t.Errorf("ExportInterval = %v, want 15s", cfg.Telemetry.ExportInterval)
	}
}

// Plaintext export puts OTEL_TOKEN on the wire in the clear, so a deployment
// that has asked for it must be told at boot rather than discovering it later.
func TestLoad_TelemetryWarnsAboutPlaintextExport(t *testing.T) {
	for _, env := range []map[string]string{
		{"OTEL_ENDPOINT_HOST": "http://10.0.0.4:4318"},
		{"OTEL_ENDPOINT_HOST": "otel.example.com", "OTEL_INSECURE": "true"},
	} {
		env["OTEL_TOKEN"] = "collector-token"
		env["OTEL_PROJECT"] = "acme"

		setEnv(t, env)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if !cfg.Telemetry.Insecure {
			t.Fatalf("Insecure = false for %v", env)
		}
		if warnings := strings.Join(cfg.Warnings(), "\n"); !strings.Contains(warnings, "plaintext") {
			t.Errorf("no plaintext warning for %v:\n%s", env, warnings)
		}
	}
}

func TestLoad_TelemetryWarnsAboutAnIgnoredEndpointPath(t *testing.T) {
	setEnv(t, map[string]string{
		"OTEL_ENDPOINT_HOST": "https://otel.example.com/otlp",
		"OTEL_TOKEN":         "collector-token",
		"OTEL_PROJECT":       "acme",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Telemetry.Endpoint != "otel.example.com" {
		t.Errorf("Endpoint = %q, want the path removed", cfg.Telemetry.Endpoint)
	}
	if warnings := strings.Join(cfg.Warnings(), "\n"); !strings.Contains(warnings, "/otlp") {
		t.Errorf("the ignored path is not named in the boot log:\n%s", warnings)
	}
}
