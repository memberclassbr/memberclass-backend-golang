// Package telemetry installs the OpenTelemetry providers this service exports
// through: traces and metrics over OTLP/HTTP to a collector that runs outside
// the deployment — the service is on Railway, the collector on a dedicated
// observability VPS.
//
// That topology is the reason for most of what follows. Everything here is a
// push: nothing scrapes this process, so anything a scrape would have supplied
// has to be supplied by the exporter instead.
//
// The sampler is AlwaysSample. Sampling policy lives in the collector, not in
// the binaries — changing the rate there is a server-side edit, while changing
// it here would mean redeploying every customer's service. The cost is that
// background pollers, whose queries have no parent request, each become their
// own root trace; the collector drops them by name.
//
// service.name is the same string for every deployment and the customer goes
// in service.namespace. There is one deployment per customer, so naming the
// service after the customer would turn a single service into N unrelated ones
// and make "how is the API doing" unanswerable.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	// Pinned to the version resource.WithTelemetrySDK() emits. Merging two
	// resources built against different schema URLs is a hard error, so this
	// import has to track the SDK rather than be picked freely.
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"

	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/memberclass-backend-golang/internal/platform/logger"
)

// ServiceName is reported by every deployment. See the package comment for why
// it does not carry the customer's name.
const ServiceName = "memberclass-backend"

const (
	// shutdownTimeout bounds the final flush. The collector is off-host, so a
	// shutdown that waits on it indefinitely would hold the container open past
	// the platform's patience and get it killed mid-flush anyway.
	shutdownTimeout = 5 * time.Second

	// defaultExportInterval covers a Config built by something other than
	// config.Load, which already defaults this. A zero interval would make the
	// periodic reader fall back to the SDK's own default silently.
	defaultExportInterval = 60 * time.Second
)

// ShutdownFunc flushes whatever is buffered and releases the providers. It is
// safe to call on a disabled setup, so callers can defer it unconditionally.
type ShutdownFunc func(context.Context) error

func noopShutdown(context.Context) error { return nil }

// Init installs the global tracer and meter providers and returns the function
// that tears them down.
//
// A deployment without the OTEL variables is not an error: Init logs which
// variable is missing, returns a no-op shutdown, and the service runs
// uninstrumented. Telemetry going missing must never be the reason an API stops
// serving.
func Init(ctx context.Context, cfg *config.Config, log logger.Logger) (ShutdownFunc, error) {
	if !cfg.Telemetry.Enabled {
		log.Warn("telemetry disabled, service will run uninstrumented")
		return noopShutdown, nil
	}

	res, err := newResource(ctx, cfg)
	if err != nil {
		// Detection is best-effort and newResource still returns a usable
		// resource. A host that will not report its OS is not a reason to boot
		// a deployment blind.
		log.Warn("telemetry resource detection incomplete: " + err.Error())
	}

	traceExporter, err := otlptracehttp.New(ctx, traceExporterOptions(cfg)...)
	if err != nil {
		return nil, fmt.Errorf("trace exporter: %w", err)
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	metricExporter, err := otlpmetrichttp.New(ctx, metricExporterOptions(cfg)...)
	if err != nil {
		// The tracer provider already owns a background batcher goroutine, so
		// it has to come down before we bail out.
		_ = tracerProvider.Shutdown(context.WithoutCancel(ctx))
		return nil, fmt.Errorf("metric exporter: %w", err)
	}

	interval := cfg.Telemetry.ExportInterval
	if interval <= 0 {
		interval = defaultExportInterval
	}

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithView(httpServerAttributeView()),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter,
			sdkmetric.WithInterval(interval))),
	)

	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	// Go runtime metrics (heap, GC, goroutines). A container killed for memory
	// leaves no other trace than the restart itself.
	if err := runtime.Start(runtime.WithMeterProvider(meterProvider)); err != nil {
		log.Warn("runtime metrics unavailable: " + err.Error())
	}

	log.Info("Telemetry initialised",
		"endpoint", cfg.Telemetry.Endpoint,
		"insecure", cfg.Telemetry.Insecure,
		"service", ServiceName,
		"namespace", cfg.Telemetry.Project,
		"instance", ServiceInstanceID(cfg),
		"environment", cfg.App.Env,
		"metricInterval", interval.String(),
	)

	return func(ctx context.Context) error {
		// The caller's context is usually the one the shutdown signal just
		// cancelled. Inheriting its cancellation would make every Shutdown
		// return immediately and drop the buffered spans — exactly the ones
		// describing the shutdown.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer cancel()

		return errors.Join(
			tracerProvider.Shutdown(ctx),
			meterProvider.Shutdown(ctx),
		)
	}, nil
}

func traceExporterOptions(cfg *config.Config) []otlptracehttp.Option {
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(cfg.Telemetry.Endpoint),
		otlptracehttp.WithHeaders(map[string]string{
			"Authorization": "Bearer " + cfg.Telemetry.Token,
		}),
		// The collector is a network away rather than a sidecar, and Railway
		// meters egress. Span batches compress well enough that this is the
		// difference between a rounding error and a line item.
		otlptracehttp.WithCompression(otlptracehttp.GzipCompression),
	}
	if cfg.Telemetry.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	return opts
}

func metricExporterOptions(cfg *config.Config) []otlpmetrichttp.Option {
	opts := []otlpmetrichttp.Option{
		otlpmetrichttp.WithEndpoint(cfg.Telemetry.Endpoint),
		otlpmetrichttp.WithHeaders(map[string]string{
			"Authorization": "Bearer " + cfg.Telemetry.Token,
		}),
		otlpmetrichttp.WithCompression(otlpmetrichttp.GzipCompression),
		otlpmetrichttp.WithTemporalitySelector(cumulativeTemporality),
	}
	if cfg.Telemetry.Insecure {
		opts = append(opts, otlpmetrichttp.WithInsecure())
	}
	return opts
}

// cumulativeTemporality forces cumulative aggregation on every instrument,
// ignoring OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE.
//
// This is a guard, not a preference. The collector on the VPS forwards to
// Prometheus through prometheusremotewrite, and that exporter drops delta
// histograms silently — simple sums keep flowing, so the symptom is "counters
// healthy, latency panels empty", which reads like an application bug and is
// not one. Nothing here sets that variable today; pinning the selector makes
// the whole failure class unreachable if somebody ever does, on the collector
// side or in a Railway variable copied between services.
func cumulativeTemporality(sdkmetric.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}

// httpServerAttributeView allow-lists the labels on http.server.* metrics.
//
// otelhttp derives server.address from the Host header, which the caller
// controls. On a public API that is unbounded cardinality: a loop sending a
// fresh Host mints a fresh series per request, and the bill lands on the VPS's
// Prometheus rather than here. server.port and network.protocol.name go with
// it — neither varies in a way anyone queries.
//
// The list is an allow-list, so a new attribute worth keeping has to be added
// here as well as emitted. Everything otelhttp and this package record on
// http.server.* is listed below.
func httpServerAttributeView() sdkmetric.View {
	return sdkmetric.NewView(
		sdkmetric.Instrument{Name: "http.server.*"},
		sdkmetric.Stream{AttributeFilter: attribute.NewAllowKeysFilter(
			semconv.HTTPRequestMethodKey,
			semconv.HTTPResponseStatusCodeKey,
			semconv.HTTPRouteKey,
			semconv.URLSchemeKey,
			semconv.NetworkProtocolVersionKey,
			semconv.ErrorTypeKey,
		)},
	)
}

// newResource describes this process to the collector. It always returns a
// usable resource: detection failures come back as an error the caller logs,
// never as a reason to abandon the boot.
//
// service.version is omitted rather than sent empty when the platform does not
// provide it — an empty attribute looks like a deployment reporting a blank
// version. service.instance.id gets the opposite treatment and is always set;
// see ServiceInstanceID.
func newResource(ctx context.Context, cfg *config.Config) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		semconv.ServiceName(ServiceName),
		semconv.ServiceNamespace(cfg.Telemetry.Project),
		semconv.ServiceInstanceID(ServiceInstanceID(cfg)),
		semconv.DeploymentEnvironmentNameKey.String(cfg.App.Env),
	}
	if v := cfg.Telemetry.Version; v != "" {
		attrs = append(attrs, semconv.ServiceVersion(v))
	}

	res, err := resource.New(ctx,
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithOS(),
		resource.WithProcessPID(),
		resource.WithProcessRuntimeDescription(),
		// WithAttributes goes last: it wins over anything a detector guessed.
		resource.WithAttributes(attrs...),
	)
	if res == nil {
		return resource.NewWithAttributes(semconv.SchemaURL, attrs...), err
	}
	return res, err
}
