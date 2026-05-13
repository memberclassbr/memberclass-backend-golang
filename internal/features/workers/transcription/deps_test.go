package transcription

import (
	"testing"

	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/logger"
)

// TestNew_DoesNotPanicWithMissingDeps guards New against panicking when
// OPENAI_API_KEY and ffmpeg are absent — the slice has to boot even in
// those environments so the rest of the app keeps working.
func TestNew_DoesNotPanicWithMissingDeps(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	log := logger.NewLogger()
	f := New(nil, nil, log, nil)
	if f == nil {
		t.Fatal("New returned nil")
	}
	if f.openaiBaseURL != defaultOpenAIBase {
		t.Fatalf("openaiBaseURL = %q, want %q", f.openaiBaseURL, defaultOpenAIBase)
	}
	if f.workers != defaultWorkerWorkers {
		t.Fatalf("workers = %d, want %d", f.workers, defaultWorkerWorkers)
	}
	if f.pollInterval != defaultPollInterval {
		t.Fatalf("pollInterval = %s, want %s", f.pollInterval, defaultPollInterval)
	}
}

func TestNew_HonorsEnvOverrides(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("TRANSCRIPTION_WORKER_CONCURRENCY", "4")
	t.Setenv("TRANSCRIPTION_POLL_INTERVAL_SECONDS", "5")
	log := logger.NewLogger()
	f := New(nil, nil, log, nil)
	if f.openaiAPIKey != "test-key" {
		t.Fatalf("openaiAPIKey = %q, want test-key", f.openaiAPIKey)
	}
	if f.workers != 4 {
		t.Fatalf("workers = %d, want 4", f.workers)
	}
	if f.pollInterval.Seconds() != 5 {
		t.Fatalf("pollInterval = %s, want 5s", f.pollInterval)
	}
}

func TestPreflight_FailsWithoutDB(t *testing.T) {
	f := &Feature{openaiAPIKey: "k"}
	if err := f.preflight(); err == nil {
		t.Fatal("preflight returned nil, want error for missing DB")
	}
}

func TestPreflight_FailsWithoutKey(t *testing.T) {
	f := &Feature{}
	if err := f.preflight(); err == nil {
		t.Fatal("preflight returned nil, want error for missing key")
	}
}
