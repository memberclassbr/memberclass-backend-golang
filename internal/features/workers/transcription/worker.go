package transcription

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Start spins up the worker pool. Idempotent — calling Start twice is a
// no-op. Wire it from cmd/api/main.go's startApplication AFTER the DBs
// are open and BEFORE the HTTP server starts accepting work so newly
// enqueued jobs begin processing immediately.
//
// Refuses to start when transcriptionDB is missing or the chunks.embedding
// column probe fails — running with the wrong dim would re-fire embed
// requests into a Postgres error loop (see 2026-05-13 burn).
func (f *Feature) Start(parent context.Context) {
	if f.transcriptionDB == nil {
		f.log.Warn("transcription.worker.disabled", "reason", "transcriptionDB is nil (DB_TRANSCRIPTION_DSN unset or unreachable)")
		return
	}
	if err := f.probeEmbedDims(parent); err != nil {
		f.log.Error("transcription.worker.disabled",
			"reason", "embed dimension probe failed — worker will not start to avoid retry loop",
			"error", err.Error())
		return
	}
	f.log.Info("transcription.worker.embed_dims_resolved", "dims", f.embedDims)

	f.mu.Lock()
	if f.running {
		f.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	f.cancel = cancel
	f.done = make(chan struct{})
	f.running = true
	f.mu.Unlock()

	go func() {
		defer close(f.done)
		f.run(ctx)
	}()
}

// Stop signals the worker to drain and waits for the run loop to exit.
// `timeout` only bounds how long we log about a slow shutdown — we always
// block until the goroutine has actually returned, otherwise a subsequent
// Start could spin up a second loop racing the still-alive one against
// the same DB.
func (f *Feature) Stop(timeout time.Duration) {
	f.mu.Lock()
	if !f.running {
		f.mu.Unlock()
		return
	}
	cancel, done := f.cancel, f.done
	f.mu.Unlock()

	cancel()

	select {
	case <-done:
	case <-time.After(timeout):
		f.log.Warn("transcription.worker.shutdown_slow", "timeout", timeout.String())
		<-done
	}
	f.mu.Lock()
	f.running = false
	f.mu.Unlock()
}

// run is the worker pool's main loop: it polls jobs at f.pollInterval and
// scans for orphans at orphanResetInterval, fanning each claimed job out
// to a pool of f.workers goroutines through a buffered channel.
func (f *Feature) run(ctx context.Context) {
	pollT := time.NewTicker(f.pollInterval)
	orphanT := time.NewTicker(orphanResetInterval)
	defer pollT.Stop()
	defer orphanT.Stop()

	// jobChan capacity = workers so a slow tick doesn't queue work past
	// what the pool can drain.
	jobChan := make(chan claimedJob, f.workers)
	for i := 0; i < f.workers; i++ {
		go f.processLoop(ctx, jobChan)
	}

	f.log.Info("transcription.worker.started", "workers", f.workers, "poll", f.pollInterval.String())

	for {
		select {
		case <-ctx.Done():
			close(jobChan)
			f.log.Info("transcription.worker.stopped")
			return
		case <-orphanT.C:
			n, err := f.resetOrphans(ctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				f.log.Error("transcription.worker.orphan_reset_failed", "error", err.Error())
			} else if n > 0 {
				f.log.Info("transcription.worker.orphans_reset", "count", n)
			}
		case <-pollT.C:
			if err := f.tick(ctx, jobChan); err != nil && !errors.Is(err, context.Canceled) {
				f.log.Error("transcription.worker.tick_failed", "error", err.Error())
			}
		}
	}
}

// tick claims up to f.workers jobs and pushes them onto jobChan. Returning
// quickly is more important than draining the full backlog — the next
// tick will pick up what's left. The claim limit is intentionally bounded
// by worker count so a slow tenant can't monopolize the pool.
func (f *Feature) tick(ctx context.Context, jobChan chan<- claimedJob) error {
	jobs, err := f.claimPending(ctx, f.workers)
	if err != nil {
		return err
	}
	if len(jobs) == 0 {
		return nil
	}
	f.log.Info("transcription.worker.claimed", "count", len(jobs))
	for _, j := range jobs {
		select {
		case jobChan <- j:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// processLoop drains jobs off the channel until ctx cancels or the channel
// closes. Each job is executed via executeJob; failures land in
// markJobFailed which handles the retry/terminate decision.
func (f *Feature) processLoop(ctx context.Context, jobChan <-chan claimedJob) {
	for {
		select {
		case <-ctx.Done():
			return
		case j, ok := <-jobChan:
			if !ok {
				return
			}
			f.processOne(ctx, j)
		}
	}
}

// probeEmbedDims reads the declared width of public.chunks.embedding from
// the Railway DB and caches it on f.embedDims. For pgvector, atttypmod
// stores the dimension directly. Refusing to start on probe failure is
// deliberate: a mismatch between code-requested dims and column dims
// produces a tight Postgres error loop (see the 2026-05-13 incident).
func (f *Feature) probeEmbedDims(ctx context.Context) error {
	const q = `
        SELECT atttypmod
          FROM pg_attribute
         WHERE attrelid = 'public.chunks'::regclass
           AND attname  = 'embedding'
           AND NOT attisdropped
    `
	var dims int
	if err := f.transcriptionDB.QueryRowContext(ctx, q).Scan(&dims); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("chunks.embedding column not found")
		}
		return err
	}
	if dims <= 0 {
		return errors.New("chunks.embedding has no declared dimension (vector with no typmod)")
	}
	f.embedDims = dims
	return nil
}

func (f *Feature) processOne(ctx context.Context, j claimedJob) {
	f.log.Info("transcription.worker.job_started", "jobId", j.ID, "tenant", j.TenantID, "attempt", j.Attempts)
	if err := f.executeJob(ctx, j.ID, j.TenantID, j.Payload); err != nil {
		f.log.Error("transcription.worker.job_failed", "jobId", j.ID, "tenant", j.TenantID, "error", err.Error())
		if mErr := f.markJobFailed(ctx, j.ID, err.Error()); mErr != nil {
			f.log.Error("transcription.worker.mark_failed_error", "jobId", j.ID, "error", mErr.Error())
		}
		return
	}
	f.log.Info("transcription.worker.job_completed", "jobId", j.ID, "tenant", j.TenantID)
}
