package telemetry

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	otelmetric "go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

// instrumentationName is the meter scope for the instruments this package
// registers itself, as opposed to the ones otelhttp owns.
const instrumentationName = "github.com/memberclass-backend-golang/internal/platform/telemetry"

// knownMethods is the HTTP method set. Anything else becomes _OTHER.
//
// The method reaching a middleware is whatever the caller wrote on the wire —
// net/http accepts any token, and chi's 405 handling happens further down, so
// this runs before anything has rejected a nonsense verb. On a public API an
// unnormalised method is one new series per request.
var knownMethods = map[string]struct{}{
	http.MethodGet: {}, http.MethodHead: {}, http.MethodPost: {},
	http.MethodPut: {}, http.MethodPatch: {}, http.MethodDelete: {},
	http.MethodConnect: {}, http.MethodOptions: {}, http.MethodTrace: {},
}

func normalizeMethod(method string) string {
	if _, ok := knownMethods[method]; ok {
		return method
	}
	return "_OTHER"
}

// Instrumented reports whether a request should be measured and traced.
//
// The platform's healthcheck hits /health every few seconds, which makes it
// the highest-volume route in the service and the loudest thing on every
// latency panel — while saying nothing, because it does not touch the code
// anyone is looking at. It is served either way; it is only not recorded.
//
// This is exported so the tracing middleware can be handed the same predicate.
// Two families of instrumentation measuring different sets of requests is
// worse than either measuring less: a trace count and a request count that
// disagree send people looking for dropped spans that were never emitted.
func Instrumented(r *http.Request) bool {
	return r != nil && r.URL != nil && r.URL.Path != "/health"
}

// HTTPServerMetrics is a chi middleware recording the semconv HTTP server
// metrics: request duration, request and response body size, and active
// requests. It replaced otelchi's metric middlewares, which recorded no status
// code at all — so the service had latency but no error rate — and labelled
// what they did record with the pre-1.21 attribute names.
//
// It must be mounted on a chi router, since the route pattern comes from chi's
// RouteContext.
func HTTPServerMetrics(next http.Handler) http.Handler {
	if next == nil {
		return next
	}

	meter := otel.GetMeterProvider().Meter(instrumentationName)

	// otelhttp covers duration and the two body-size histograms, but no
	// released version records active_requests for a *server* — only for a
	// client transport. Hence this one, by hand, under the semconv name so it
	// is the metric a dashboard already expects rather than a local invention.
	active, err := meter.Int64UpDownCounter(
		"http.server.active_requests",
		otelmetric.WithUnit("{request}"),
		otelmetric.WithDescription("Number of active HTTP server requests."),
	)
	if err != nil {
		// A broken instrument costs one metric; it does not cost a boot.
		active = nil
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if active != nil {
			// semconv gives active_requests the method and the scheme, and
			// deliberately not the route — which is just as well, because the
			// route is not knowable on the way in: chi resolves the pattern as
			// the request descends.
			attrs := otelmetric.WithAttributes(
				semconv.HTTPRequestMethodKey.String(normalizeMethod(r.Method)),
				semconv.URLScheme(requestScheme(r)),
			)
			active.Add(r.Context(), 1, attrs)
			defer active.Add(r.Context(), -1, attrs)
		}

		// otelhttp reads its labeler after the handler returns, which is the
		// only reason http.route can be attached at all — and why it reaches
		// the duration histogram and both body-size histograms rather than
		// only whatever this middleware records itself.
		defer func() {
			route := chiRoutePattern(r)
			if route == "" {
				// Unmatched: no pattern exists, and labelling it with the raw
				// path is exactly the cardinality the pattern is there to
				// avoid. A 404 keeps its status code and loses its route.
				return
			}
			if labeler, ok := otelhttp.LabelerFromContext(r.Context()); ok {
				labeler.Add(semconv.HTTPRoute(route))
			}
		}()

		next.ServeHTTP(w, r)
	})

	instrumented := otelhttp.NewHandler(inner, ServiceName,
		// No-op tracer on purpose. The server span belongs to otelchi, which
		// labels it with the chi route pattern; letting otelhttp open one too
		// nests two server spans per request and splits every trace in half.
		otelhttp.WithTracerProvider(tracenoop.NewTracerProvider()),
	)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The skip lives out here rather than in otelhttp.WithFilter, because
		// a rejected filter still calls the wrapped handler — active_requests
		// would go on counting /health.
		if !Instrumented(r) {
			next.ServeHTTP(w, r)
			return
		}
		instrumented.ServeHTTP(w, r)
	})
}

// requestScheme reports the scheme of the hop this process actually saw.
//
// X-Forwarded-Proto is not consulted: it is a caller-controlled header, so
// reading it would put an attacker's string into a metric label. Behind
// Railway's proxy TLS terminates upstream and this reports http, which is the
// truth about this connection.
func requestScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

// chiRoutePattern returns the matched route pattern
// ("/api/v1/vitrine/{vitrineId}"), or "" when nothing matched. It is only
// populated once routing has happened, so callers must read it on the way back
// out of the handler chain.
func chiRoutePattern(r *http.Request) string {
	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		return ""
	}
	return rctx.RoutePattern()
}
