package telemetry

import (
	"testing"

	"github.com/memberclass-backend-golang/internal/platform/config"
)

func TestServiceInstanceID_PrefersTheConfiguredValue(t *testing.T) {
	cfg := &config.Config{}
	cfg.Telemetry.InstanceID = "  replica-7  "

	if got := ServiceInstanceID(cfg); got != "replica-7" {
		t.Errorf("ServiceInstanceID = %q, want the trimmed configured value", got)
	}
}

// Everything this service exports is pushed, and nothing stamps `instance` on
// a push. An empty id means every replica shares one series identity, their
// cumulative counters interleave, and rate() over the result is noise.
func TestServiceInstanceID_NeverEmpty(t *testing.T) {
	for name, cfg := range map[string]*config.Config{
		"nil config": nil,
		"unset":      {},
		"blank":      {Telemetry: config.Telemetry{InstanceID: "   "}},
	} {
		if got := ServiceInstanceID(cfg); got == "" {
			t.Errorf("%s: ServiceInstanceID is empty", name)
		}
	}
}

// The tracer and the meter build their resource separately. A fresh id per
// call would put a metric and the span that explains it under two identities.
func TestServiceInstanceID_IsStable(t *testing.T) {
	cfg := &config.Config{}

	first := ServiceInstanceID(cfg)
	if second := ServiceInstanceID(cfg); first != second {
		t.Errorf("ServiceInstanceID changed between calls: %q then %q", first, second)
	}
	if third := ServiceInstanceID(nil); third != first {
		t.Errorf("ServiceInstanceID(nil) = %q, want the same detected id %q", third, first)
	}
}

func TestDetectInstanceID_NeverEmpty(t *testing.T) {
	if detectInstanceID() == "" {
		t.Error("detectInstanceID returned empty; the uuid fallback did not fire")
	}
}
