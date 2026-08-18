package telemetry

import (
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
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

func resourceAttributes(t *testing.T, cfg *config.Config) map[string]string {
	t.Helper()

	res, err := newResource(context.Background(), cfg)
	if err != nil {
		// Detection is allowed to be partial; the resource still has to work.
		t.Logf("newResource reported partial detection: %v", err)
	}
	if res == nil {
		t.Fatal("newResource returned no resource")
	}

	attrs := map[string]string{}
	for _, kv := range res.Attributes() {
		attrs[string(kv.Key)] = kv.Value.Emit()
	}
	return attrs
}

func TestNewResource_Identity(t *testing.T) {
	attrs := resourceAttributes(t, enabledConfig())

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

	// Unlike the version, this one is never omitted. Everything is pushed, so
	// no scraper stamps `instance` for us: replicas sharing a series identity
	// interleave into one cumulative counter and rate() over it is noise.
	if attrs["service.instance.id"] == "" {
		t.Error("service.instance.id must be resolved even when the platform injects nothing")
	}
}

func TestNewResource_HonoursExplicitPlatformAttributes(t *testing.T) {
	cfg := enabledConfig()
	cfg.Telemetry.Version = "abc123"
	cfg.Telemetry.InstanceID = "replica-7"

	attrs := resourceAttributes(t, cfg)

	if got := attrs["service.version"]; got != "abc123" {
		t.Errorf("service.version = %q, want %q", got, "abc123")
	}
	if got := attrs["service.instance.id"]; got != "replica-7" {
		t.Errorf("service.instance.id = %q, want %q", got, "replica-7")
	}
}

// The tracer and the meter share one resource. If they disagreed about who
// this process is, a metric and the span explaining it would land under two
// identities and nothing would join them.
func TestNewResource_InstanceIDIsStableAcrossCalls(t *testing.T) {
	cfg := enabledConfig()

	first := resourceAttributes(t, cfg)["service.instance.id"]
	second := resourceAttributes(t, cfg)["service.instance.id"]

	if first != second {
		t.Errorf("service.instance.id changed between calls: %q then %q", first, second)
	}
}

// Delta histograms are dropped silently by the collector's remote write, which
// looks like an application bug rather than a configuration one. The selector
// is pinned so the environment variable that would cause it cannot.
func TestCumulativeTemporality_IgnoresInstrumentKind(t *testing.T) {
	kinds := []sdkmetric.InstrumentKind{
		sdkmetric.InstrumentKindCounter,
		sdkmetric.InstrumentKindUpDownCounter,
		sdkmetric.InstrumentKindHistogram,
		sdkmetric.InstrumentKindObservableCounter,
		sdkmetric.InstrumentKindObservableUpDownCounter,
		sdkmetric.InstrumentKindObservableGauge,
	}
	for _, kind := range kinds {
		if got := cumulativeTemporality(kind); got != metricdata.CumulativeTemporality {
			t.Errorf("temporality for %v = %v, want cumulative", kind, got)
		}
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
