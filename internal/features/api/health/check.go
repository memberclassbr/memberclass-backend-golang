package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// checkTimeout bounds the whole probe. It is short on purpose: a health check
// that waits ten seconds on a wedged database is itself a way to take the
// deployment down, because the platform gives up first and reads the timeout
// as a failure anyway.
const checkTimeout = 2 * time.Second

type checkResponse struct {
	Status string `json:"status"`
}

// Check handles `GET /health`.
//
// 200 means every dependency answered; 503 means at least one did not, and the
// platform should stop routing traffic here. Which one failed goes to the log,
// not to the response — see the package comment.
func (f *Feature) Check(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, checkResponse{Status: "error"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), checkTimeout)
	defer cancel()

	if failed := f.failedDependencies(ctx); len(failed) > 0 {
		for _, d := range failed {
			f.log.Error("health.dependency_unavailable", "dependency", d.name, "error", d.err.Error())
		}
		writeJSON(w, http.StatusServiceUnavailable, checkResponse{Status: "unhealthy"})
		return
	}

	writeJSON(w, http.StatusOK, checkResponse{Status: "ok"})
}

type dependencyFailure struct {
	name string
	err  error
}

// failedDependencies probes everything and returns all failures rather than
// stopping at the first. One restart should tell the operator that both the
// database and Redis are down, not send them chasing the database alone.
func (f *Feature) failedDependencies(ctx context.Context) []dependencyFailure {
	var failed []dependencyFailure

	if err := f.db.PingContext(ctx); err != nil {
		failed = append(failed, dependencyFailure{name: "database", err: err})
	}

	// A key that is never written: the answer does not matter, completing the
	// round trip does.
	if _, err := f.cache.Exists(ctx, "health:probe"); err != nil {
		failed = append(failed, dependencyFailure{name: "cache", err: err})
	}

	return failed
}

func writeJSON(w http.ResponseWriter, code int, body checkResponse) {
	w.Header().Set("Content-Type", "application/json")
	// A cached health check reports the state of a past request, which is the
	// one thing it must never do.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
