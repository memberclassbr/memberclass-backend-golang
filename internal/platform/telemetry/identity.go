package telemetry

import (
	"os"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/memberclass-backend-golang/internal/platform/config"
)

var (
	instanceIDOnce sync.Once
	instanceIDVal  string
)

// ServiceInstanceID identifies this replica, and it is a correctness
// requirement rather than decoration.
//
// Everything this service exports is pushed. Under a scrape the Prometheus
// server stamps `instance` itself from the target address, so an absent
// service.instance.id costs nothing; under a push nothing does that. Every
// replica then emits the same series identity, the collector's remote write
// receives N interleaved streams as one series, and because each replica has
// its own zero for a cumulative counter the series walks backwards whenever a
// sample from a younger replica lands. rate() over that returns noise. Two
// replicas are enough — and Railway restarts a container often enough that a
// single-replica deployment hits the same thing across a redeploy.
//
// So this never returns empty. An id that is wrong-but-unique is harmless
// (it changes on restart and inflates the series count); an id that is
// *shared* corrupts the data.
func ServiceInstanceID(cfg *config.Config) string {
	if cfg != nil {
		if explicit := strings.TrimSpace(cfg.Telemetry.InstanceID); explicit != "" {
			return explicit
		}
	}

	// Memoised: the resource is built once per provider, but Init is also
	// called from cmd/analytics and the tests, and a fresh UUID per call would
	// let the tracer and the meter disagree about which process they are.
	instanceIDOnce.Do(func() { instanceIDVal = detectInstanceID() })
	return instanceIDVal
}

// detectInstanceID is the fallback chain for a deployment whose platform did
// not name the replica. cfg's RAILWAY_REPLICA_ID is already consulted by the
// caller, so this starts at the hostname.
func detectInstanceID() string {
	// In a container the hostname is the container id, which is exactly the
	// per-replica identity wanted. On a laptop it is the machine name, which
	// is stable and equally fine because there is only one of it.
	if host, err := os.Hostname(); err == nil {
		host = strings.TrimSpace(host)
		if host != "" && host != "localhost" {
			return host
		}
	}
	return uuid.NewString()
}
