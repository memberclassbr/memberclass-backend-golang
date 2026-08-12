// Package telemetry installs the OpenTelemetry providers this service exports
// through: traces and metrics over OTLP/HTTP to a collector that runs outside
// the deployment.
//
// Two decisions are worth knowing before reading the code.
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
	// metricExportInterval is how often the periodic reader ships metrics.
	metricExportInterval = 60 * time.Second

	// shutdownTimeout bounds the final flush. The collector is off-host, so a
	// shutdown that waits on it indefinitely would hold the container open past
	// the platform's patience and get it killed mid-flush anyway.
	shutdownTimeout = 5 * time.Second
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
		return nil, fmt.Errorf("resource: %w", err)
	}

	traceExporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(cfg.Telemetry.Endpoint),
		otlptracehttp.WithHeaders(map[string]string{
			"Authorization": "Bearer " + cfg.Telemetry.Token,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("trace exporter: %w", err)
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	metricExporter, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpoint(cfg.Telemetry.Endpoint),
		otlpmetrichttp.WithHeaders(map[string]string{
			"Authorization": "Bearer " + cfg.Telemetry.Token,
		}),
	)
	if err != nil {
		// The tracer provider already owns a background batcher goroutine, so
		// it has to come down before we bail out.
		_ = tracerProvider.Shutdown(context.WithoutCancel(ctx))
		return nil, fmt.Errorf("metric exporter: %w", err)
	}

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter,
			sdkmetric.WithInterval(metricExportInterval))),
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
		"service", ServiceName,
		"namespace", cfg.Telemetry.Project,
		"environment", cfg.App.Env,
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

// newResource describes this process to the collector. Version and instance id
// are omitted rather than sent empty when the platform does not provide them —
// an empty attribute is worse than an absent one, because it looks like a
// deployment that reported a blank version.
func newResource(ctx context.Context, cfg *config.Config) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		semconv.ServiceName(ServiceName),
		semconv.ServiceNamespace(cfg.Telemetry.Project),
		semconv.DeploymentEnvironmentNameKey.String(cfg.App.Env),
	}
	if v := cfg.Telemetry.Version; v != "" {
		attrs = append(attrs, semconv.ServiceVersion(v))
	}
	if id := cfg.Telemetry.InstanceID; id != "" {
		attrs = append(attrs, semconv.ServiceInstanceID(id))
	}

	return resource.New(ctx,
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(attrs...),
	)
}
