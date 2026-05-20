// Package transcription is a vertical slice that owns the full
// video transcription + embedding pipeline. It exposes HTTP routes to
// enqueue jobs and runs an in-process worker pool that polls a `jobs`
// table on the Railway pgvector database and processes lessons
// end-to-end.
//
// Pipeline per lesson:
//   1. Resolve Bunny playback URL (HLS) from lesson.mediaUrl + tenant creds
//   2. Extract audio to MP3 16 kHz mono (ffmpeg)
//   3. Split audio into 10-min windows if total > Whisper 25 MB limit
//   4. Transcribe each window via OpenAI Whisper API (whisper-1)
//   5. Chunk transcript (~500 tokens, 50 overlap, aligned to Whisper segments)
//   6. Embed chunks via OpenAI text-embedding-3-small (batched)
//   7. UPSERT video + INSERT transcript + INSERT chunks (single tx, Railway pgvector)
//   8. UPDATE lesson.transcriptionCompleted = true (memberclass DB)
//
// Storage: a dedicated Railway Postgres service created from the
// "PostgreSQL pgvector" template. The vanilla Railway Postgres image
// does NOT ship the vector binary; recreating the service from the
// pgvector template is mandatory. Schema (videos/transcripts/chunks/
// jobs/token_usage) was migrated one-shot from the legacy Supabase
// instance via scripts/migrate-supabase-to-railway.sh.
//
// See CLAUDE.md ("Architecture migration in progress") for VSA rules
// this slice must follow.
package transcription

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/ports"
	bunnyport "github.com/memberclass-backend-golang/internal/domain/ports/bunny"
)

// Tunables. Package-level so tests can read them; move into Feature if any
// of these ever need to vary per tenant.
const (
	defaultPollInterval  = 30 * time.Second
	defaultWorkerWorkers = 2

	// orphanResetInterval bounds how often the worker scans for rows that
	// got stuck in RUNNING because a previous instance crashed mid-job.
	orphanResetInterval = 5 * time.Minute
	// orphanStaleThreshold: a RUNNING row older than this is considered
	// crashed and pushed back to PENDING (within max_attempts).
	orphanStaleThreshold = 30 * time.Minute

	// whisperMaxAudioBytes is Whisper's 25 MB upload limit, minus a 1 MB
	// safety margin for multipart overhead.
	whisperMaxAudioBytes = 24 * 1024 * 1024

	// embedBatchSize keeps a single embeddings request under ~1 MB and far
	// below OpenAI's per-call cap (2048 inputs).
	embedBatchSize = 96
)

// Feature holds the shared dependencies for every action in this slice
// (HTTP handlers, worker goroutines, cron callback). Wire it in
// cmd/api/main.go via fx.Provide; start/stop it from startApplication.
type Feature struct {
	transcriptionDB *sql.DB                 // Railway pgvector (videos/transcripts/chunks/jobs/token_usage)
	memberclassDB   *sql.DB                 // memberclass CockroachDB (Lesson/Tenant + transcriptionCompleted flag)
	log             ports.Logger
	bunny           bunnyport.BunnyService

	openaiAPIKey       string
	openaiBaseURL      string
	bunnyBaseURL       string
	bunnyAccountAPIKey string // account-level key (BUNNY_API_KEY); resolves CDN hostname per library
	httpClient         *http.Client

	pollInterval time.Duration
	workers      int

	// embedDims is the dimension width of chunks.embedding on the
	// Railway pgvector DB. Populated from the column's typmod at Start()
	// so embedBatch can request matching widths via OpenAI's dimensions
	// param. Zero means "probe failed / not yet probed" — callers fall
	// back to defaultEmbedDims.
	embedDims int

	// Worker lifecycle. Lock guards every transition; the goroutine itself
	// is owned by run() and signals exit via done.
	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	done    chan struct{}

	// testHookResolveAudio, when non-nil, replaces the Bunny meta + HLS
	// download + ffmpeg split chain with a caller-supplied resolver that
	// produces local audio files. Production keeps this nil.
	testHookResolveAudio resolveAudioFunc
}

// New builds the slice. Resolves env-driven tunables and warns (does not
// fail) on missing OPENAI_API_KEY / ffmpeg so the rest of the app can still
// boot when the transcription pipeline is intentionally disabled.
func New(
	transcriptionDB *sql.DB,
	memberclassDB *sql.DB,
	log ports.Logger,
	bunny bunnyport.BunnyService,
) *Feature {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		log.Warn("transcription: ffmpeg not found in PATH — pipeline will refuse to run jobs", "error", err.Error())
	}

	poll := defaultPollInterval
	if v := os.Getenv("TRANSCRIPTION_POLL_INTERVAL_SECONDS"); v != "" {
		if s, err := strconv.Atoi(v); err == nil && s > 0 {
			poll = time.Duration(s) * time.Second
		}
	}
	workers := defaultWorkerWorkers
	if v := os.Getenv("TRANSCRIPTION_WORKER_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			workers = n
		}
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Warn("transcription: OPENAI_API_KEY not set — pipeline will refuse to run jobs")
	}

	return &Feature{
		transcriptionDB: transcriptionDB,
		memberclassDB:   memberclassDB,
		log:             log,
		bunny:           bunny,
		openaiAPIKey:       apiKey,
		openaiBaseURL:      defaultOpenAIBase,
		bunnyBaseURL:       defaultBunnyBaseURL,
		bunnyAccountAPIKey: os.Getenv("BUNNY_API_KEY"),
		httpClient:         &http.Client{Timeout: 5 * time.Minute},
		pollInterval:    poll,
		workers:         workers,
	}
}

// MiddlewareSet carries the chi-compatible middlewares the slice's routes
// need. The router owns middleware construction; slices just compose them.
type MiddlewareSet struct {
	// AuthInternal validates x-internal-api-key against INTERNAL_AI_API_KEY,
	// mirroring the legacy /api/v1/ai/* gate.
	AuthInternal func(http.Handler) http.Handler
	// RateLimitTenant gates by tenant API key extracted earlier in the chain.
	RateLimitTenant func(http.Handler) http.Handler
}

// preflight runs at the top of every operation that actually needs the
// pipeline (enqueue, job execution). If the slice was wired without the
// Railway DB or without OPENAI_API_KEY we surface a 5xx instead of
// silently producing garbage.
func (f *Feature) preflight() error {
	if f.transcriptionDB == nil {
		return fmt.Errorf("transcription DB not configured (DB_TRANSCRIPTION_DSN)")
	}
	if f.openaiAPIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY not configured")
	}
	return nil
}
