package telemetry

import (
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/memberclass-backend-golang/internal/platform/config"
)

// fakeLogger records what Init said, so the tests can assert that a disabled
// deployment is told which variable is responsible rather than left guessing.
type fakeLogger struct {
	warns []string
	infos []string
}

func (f *fakeLogger) Debug(msg string, _ ...any) {}
func (f *fakeLogger) Info(msg string, _ ...any)  { f.infos = append(f.infos, msg) }
func (f *fakeLogger) Warn(msg string, _ ...any)  { f.warns = append(f.warns, msg) }
func (f *fakeLogger) Error(msg string, _ ...any) {}

func enabledConfig() *config.Config {
	cfg := &config.Config{}
	cfg.App.Env = "development"
	cfg.Telemetry = config.Telemetry{
		Enabled: true,
		// Port 1 is never listening, so the exporter fails fast instead of
		// hanging. Nothing here needs a reachable collector: the exporters are
		// lazy, and Init only has to install the providers.
		Endpoint: "127.0.0.1:1",
		Token:    "test-token",
		Project:  "acme",
	}
	return cfg
}

func TestInit_DisabledReturnsUsableNoop(t *testing.T) {
	cfg := &config.Config{}
	cfg.App.Env = "development"
	log := &fakeLogger{}

	shutdown, err := Init(context.Background(), cfg, log)
	if err != nil {
		t.Fatalf("Init with telemetry off should not error, got %v", err)
	}
	if shutdown == nil {
		t.Fatal("shutdown must be callable even when telemetry is off")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("no-op shutdown returned %v", err)
	}
	if len(log.warns) == 0 {
		t.Fatal("a deployment running uninstrumented must say so at boot")
	}
}

func TestInit_InstallsGlobalProviders(t *testing.T) {
	shutdown, err := Init(context.Background(), enabledConfig(), &fakeLogger{})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	if _, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider); !ok {
		t.Fatalf("global tracer provider is %T, want the SDK one", otel.GetTracerProvider())
	}

	// Without both propagators an incoming traceparent is dropped and every
	// request starts a fresh trace, which silently breaks correlation with the
	// Next.js frontend.
	fields := otel.GetTextMapPropagator().Fields()
	for _, want := range []string{"traceparent", "baggage"} {
		if !contains(fields, want) {
			t.Errorf("propagator fields %v missing %q", fields, want)
		}
	}
}

func TestShutdown_SurvivesCancelledContext(t *testing.T) {
	shutdown, err := Init(context.Background(), enabledConfig(), &fakeLogger{})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// The real caller shuts down because its context was cancelled by SIGTERM.
	// Shutdown must not inherit that cancellation, or the final flush is a
	// no-op and the last spans never leave the process.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = shutdown(ctx)
	if err != nil && strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("shutdown inherited the cancelled context: %v", err)
	}
}

func TestNewResource_OmitsUnsetPlatformAttributes(t *testing.T) {
	cfg := enabledConfig()

	res, err := newResource(context.Background(), cfg)
	if err != nil {
		t.Fatalf("newResource: %v", err)
	}

	attrs := map[string]string{}
	for _, kv := range res.Attributes() {
		attrs[string(kv.Key)] = kv.Value.Emit()
	}

	if got := attrs["service.name"]; got != ServiceName {
		t.Errorf("service.name = %q, want %q", got, ServiceName)
	}
	// The customer belongs in the namespace: one deployment per customer means
	// a per-customer service.name would fragment the fleet into N services.
	if got := attrs["service.namespace"]; got != "acme" {
		t.Errorf("service.namespace = %q, want %q", got, "acme")
	}
	if got := attrs["deployment.environment.name"]; got != "development" {
		t.Errorf("deployment.environment = %q, want %q", got, "development")
	}
	if _, ok := attrs["service.version"]; ok {
		t.Error("service.version should be absent when the platform did not inject it")
	}
	if _, ok := attrs["service.instance.id"]; ok {
		t.Error("service.instance.id should be absent when the platform did not inject it")
	}

	cfg.Telemetry.Version = "abc123"
	cfg.Telemetry.InstanceID = "replica-7"
	res, err = newResource(context.Background(), cfg)
	if err != nil {
		t.Fatalf("newResource: %v", err)
	}
	attrs = map[string]string{}
	for _, kv := range res.Attributes() {
		attrs[string(kv.Key)] = kv.Value.Emit()
	}
	if got := attrs["service.version"]; got != "abc123" {
		t.Errorf("service.version = %q, want %q", got, "abc123")
	}
	if got := attrs["service.instance.id"]; got != "replica-7" {
		t.Errorf("service.instance.id = %q, want %q", got, "replica-7")
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
