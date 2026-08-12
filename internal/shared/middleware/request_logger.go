package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/trace"

	"github.com/memberclass-backend-golang/internal/platform/logger"
)

// RequestLogger emits one structured line per request.
//
// It replaces chi's middleware.Logger for two reasons. The first is
// correlation: this one stamps trace_id and span_id, so a slow span found in
// the collector can be taken back to the log line that produced it, and the
// other way round. Without that the traces and the logs are two tools that
// cannot talk about the same request.
//
// The second is the route label. It logs the chi route pattern
// (/api/v1/vitrine/{vitrineId}), not the raw path, which keeps a log search by
// endpoint from having to guess at ids. Unmatched requests — 404s — have no
// pattern, so they fall back to the raw path.
//
// Mount it after RequestID, RealIP and the OTel middleware: it reads what all
// three put in the context.
func RequestLogger(log logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = r.URL.Path
			}

			attrs := []any{
				"method", r.Method,
				"route", route,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", time.Since(start).Milliseconds(),
				"remote_ip", r.RemoteAddr,
			}
			if id := chimw.GetReqID(r.Context()); id != "" {
				attrs = append(attrs, "request_id", id)
			}
			if sc := trace.SpanContextFromContext(r.Context()); sc.IsValid() {
				attrs = append(attrs,
					"trace_id", sc.TraceID().String(),
					"span_id", sc.SpanID().String(),
				)
			}

			// 4xx is the caller's problem and 5xx is ours; logging both at INFO
			// would bury the ones worth an alert.
			switch {
			case ww.Status() >= http.StatusInternalServerError:
				log.Error("http.request", attrs...)
			case ww.Status() >= http.StatusBadRequest:
				log.Warn("http.request", attrs...)
			default:
				log.Info("http.request", attrs...)
			}
		})
	}
}
