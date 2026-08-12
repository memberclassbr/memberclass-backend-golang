package telemetry

import (
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Transport wraps base so every outgoing request produces a client span and
// carries the trace context to whoever answers it. Pass nil for base to wrap
// http.DefaultTransport.
//
// It exists so the packages that talk to Bunny, iLovePDF, Resend, Spaces and
// OpenAI depend on this one function instead of each importing otelhttp and
// deciding for itself what to name things. Without it, a slow endpoint and a
// slow vendor look identical in a trace.
//
// Calling it when telemetry is off costs nothing measurable: the global
// provider is a no-op and the spans are never recorded.
func Transport(base http.RoundTripper) http.RoundTripper {
	return otelhttp.NewTransport(base)
}

// Client returns an http.Client with the given timeout and an instrumented
// transport. Most callers want this rather than Transport.
func Client(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: Transport(nil)}
}
