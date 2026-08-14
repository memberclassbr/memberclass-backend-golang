package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// newTestRouter installs a meter provider with a manual reader and the same
// view Init uses, then returns a chi router wired exactly as app.newRouter
// wires it. Asserting against the collected metrics is the only way to check
// what actually reaches the collector: the attributes a middleware passes and
// the attributes the SDK keeps are two different sets, and the view between
// them is the whole point.
func newTestRouter(t *testing.T) (*chi.Mux, *sdkmetric.ManualReader) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithView(httpServerAttributeView()),
	)
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() { otel.SetMeterProvider(previous) })

	router := chi.NewRouter()
	router.Use(HTTPServerMetrics)
	router.Get("/api/v1/vitrine/{vitrineId}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	router.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return router, reader
}

// collectPoints returns one attribute map per data point of the named metric.
func collectPoints(t *testing.T, reader *sdkmetric.ManualReader, name string) []map[string]string {
	t.Helper()

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}

	var out []map[string]string
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != name {
				continue
			}
			switch data := m.Data.(type) {
			case metricdata.Histogram[float64]:
				for _, dp := range data.DataPoints {
					out = append(out, attrMap(dp.Attributes))
				}
			case metricdata.Histogram[int64]:
				for _, dp := range data.DataPoints {
					out = append(out, attrMap(dp.Attributes))
				}
			case metricdata.Sum[int64]:
				for _, dp := range data.DataPoints {
					out = append(out, attrMap(dp.Attributes))
				}
			default:
				t.Fatalf("metric %q has unexpected data type %T", name, m.Data)
			}
		}
	}
	return out
}

func attrMap(set attribute.Set) map[string]string {
	out := map[string]string{}
	for _, kv := range set.ToSlice() {
		out[string(kv.Key)] = kv.Value.Emit()
	}
	return out
}

func TestHTTPServerMetrics_RecordsRouteAndStatus(t *testing.T) {
	router, reader := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/vitrine/f47ac10b", nil)
	router.ServeHTTP(httptest.NewRecorder(), req)

	points := collectPoints(t, reader, "http.server.request.duration")
	if len(points) != 1 {
		t.Fatalf("got %d duration points, want 1: %v", len(points), points)
	}
	attrs := points[0]

	// The id is in the path and must not be in the label, or every vitrine
	// becomes its own time series.
	if got := attrs["http.route"]; got != "/api/v1/vitrine/{vitrineId}" {
		t.Errorf("http.route = %q, want the chi pattern", got)
	}
	// otelchi's metric middlewares recorded no status code at all, which left
	// the service with latency and no error rate. This is that regression.
	if got := attrs["http.response.status_code"]; got != "418" {
		t.Errorf("http.response.status_code = %q, want 418", got)
	}
	if got := attrs["http.request.method"]; got != "GET" {
		t.Errorf("http.request.method = %q, want GET", got)
	}
}

// server.address comes from the Host header, which the caller writes. Without
// the view a loop sending a fresh Host mints a fresh series per request, and
// the cost lands on the collector's Prometheus rather than here.
func TestHTTPServerMetrics_HostHeaderDoesNotMintSeries(t *testing.T) {
	router, reader := newTestRouter(t)

	for _, host := range []string{"a.example.com", "b.example.com", "c.example.com"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/vitrine/x", nil)
		req.Host = host
		router.ServeHTTP(httptest.NewRecorder(), req)
	}

	points := collectPoints(t, reader, "http.server.request.duration")
	if len(points) != 1 {
		t.Fatalf("three Host headers produced %d series, want 1: %v", len(points), points)
	}
	for _, banned := range []string{"server.address", "server.port", "network.protocol.name"} {
		if _, ok := points[0][banned]; ok {
			t.Errorf("%s survived the attribute view", banned)
		}
	}
}

// An unmatched request keeps its status code and loses its route: labelling it
// with the raw path is exactly the cardinality the pattern exists to avoid.
func TestHTTPServerMetrics_UnmatchedRequestHasNoRoute(t *testing.T) {
	router, reader := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/nope/12345", nil)
	router.ServeHTTP(httptest.NewRecorder(), req)

	points := collectPoints(t, reader, "http.server.request.duration")
	if len(points) != 1 {
		t.Fatalf("got %d duration points, want 1", len(points))
	}
	if route, ok := points[0]["http.route"]; ok {
		t.Errorf("unmatched request labelled with http.route = %q", route)
	}
	if got := points[0]["http.response.status_code"]; got != "404" {
		t.Errorf("http.response.status_code = %q, want 404", got)
	}
}

// The platform healthcheck runs every few seconds. Left instrumented it is the
// busiest route in the service and says nothing about any of the code.
func TestHTTPServerMetrics_SkipsHealth(t *testing.T) {
	router, reader := newTestRouter(t)

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))

	if points := collectPoints(t, reader, "http.server.request.duration"); len(points) != 0 {
		t.Fatalf("/health was recorded: %v", points)
	}
	if points := collectPoints(t, reader, "http.server.active_requests"); len(points) != 0 {
		t.Fatalf("/health was counted as an active request: %v", points)
	}
}

func TestHTTPServerMetrics_RegistersActiveRequests(t *testing.T) {
	router, reader := newTestRouter(t)

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/vitrine/x", nil))

	// No released otelhttp records this one for a server, so its absence would
	// be silent — the dashboard panel would simply have no data.
	points := collectPoints(t, reader, "http.server.active_requests")
	if len(points) != 1 {
		t.Fatalf("got %d active_requests points, want 1: %v", len(points), points)
	}
	// semconv gives this instrument the method and the scheme, and not the
	// route — which is just as well, since chi has not resolved one yet.
	if got := points[0]["http.request.method"]; got != "GET" {
		t.Errorf("http.request.method = %q, want GET", got)
	}
	if got := points[0]["url.scheme"]; got != "http" {
		t.Errorf("url.scheme = %q, want http", got)
	}
}

// net/http accepts any token as a method and this middleware runs before chi
// has rejected anything, so an unnormalised method is one series per request.
func TestHTTPServerMetrics_NormalisesUnknownMethod(t *testing.T) {
	router, reader := newTestRouter(t)

	for _, method := range []string{"BREW", "STEEP", "POUR"} {
		req := httptest.NewRequest(method, "/api/v1/vitrine/x", nil)
		router.ServeHTTP(httptest.NewRecorder(), req)
	}

	points := collectPoints(t, reader, "http.server.active_requests")
	if len(points) != 1 {
		t.Fatalf("three unknown methods produced %d series, want 1: %v", len(points), points)
	}
	if got := points[0]["http.request.method"]; got != "_OTHER" {
		t.Errorf("http.request.method = %q, want _OTHER", got)
	}
}

func TestNormalizeMethod(t *testing.T) {
	cases := map[string]string{
		http.MethodGet:    http.MethodGet,
		http.MethodDelete: http.MethodDelete,
		"get":             "_OTHER", // lowercase is not the standard token
		"BREW":            "_OTHER",
		"":                "_OTHER",
	}
	for in, want := range cases {
		if got := normalizeMethod(in); got != want {
			t.Errorf("normalizeMethod(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInstrumented(t *testing.T) {
	// The match is exact. /healthz and /health/something are somebody else's
	// routes, and a prefix match would quietly stop measuring them.
	cases := map[string]bool{
		"/health":             false,
		"/healthz":            true,
		"/health/subresource": true,
		"/api/v1/health":      true,
		"/api/v1/vitrine/abc": true,
	}
	for path, want := range cases {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if got := Instrumented(req); got != want {
			t.Errorf("Instrumented(%q) = %v, want %v", path, got, want)
		}
	}
	if Instrumented(nil) {
		t.Error("Instrumented(nil) must not panic or claim a nil request is instrumentable")
	}
}
