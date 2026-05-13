package transcription

import (
	"context"
	"fmt"
)

// claimedJob carries the columns returned by sqlClaimJobs into the worker
// pool. Payload stays raw — the pipeline owns its JSON shape.
type claimedJob struct {
	ID          string
	TenantID    string
	Payload     []byte
	Attempts    int
	MaxAttempts int
}

// claimPending atomically claims up to `limit` PENDING jobs of type
// VIDEO_PROCESSING and flips them to RUNNING in one round-trip. FOR
// UPDATE SKIP LOCKED makes concurrent claims safe across worker
// goroutines (and across multiple deployed replicas).
func (f *Feature) claimPending(ctx context.Context, limit int) ([]claimedJob, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := f.transcriptionDB.QueryContext(ctx, sqlClaimJobs, limit)
	if err != nil {
		return nil, fmt.Errorf("claim jobs: %w", err)
	}
	defer rows.Close()

	var out []claimedJob
	for rows.Next() {
		var j claimedJob
		if err := rows.Scan(&j.ID, &j.TenantID, &j.Payload, &j.Attempts, &j.MaxAttempts); err != nil {
			return nil, fmt.Errorf("scan claimed job: %w", err)
		}
		out = append(out, j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed jobs: %w", err)
	}
	return out, nil
}

// markJobFailed flips a RUNNING job back to PENDING if attempts remain,
// otherwise terminates it as FAILED. The state machine — including the
// attempts >= max_attempts comparison — lives in sqlMarkJobFailed so the
// retry decision is atomic with the status flip.
func (f *Feature) markJobFailed(ctx context.Context, jobID, errMsg string) error {
	var status string
	if err := f.transcriptionDB.QueryRowContext(ctx, sqlMarkJobFailed, jobID, errMsg).Scan(&status); err != nil {
		return fmt.Errorf("mark job failed: %w", err)
	}
	f.log.Info("transcription.worker.job_state_after_failure", "jobId", jobID, "status", status)
	return nil
}

// resetOrphans pushes RUNNING rows older than orphanStaleThreshold back to
// PENDING so they get re-tried. Covers the crash-mid-run case where a
// worker died before flipping status.
func (f *Feature) resetOrphans(ctx context.Context) (int, error) {
	rows, err := f.transcriptionDB.QueryContext(ctx, sqlResetOrphans, int(orphanStaleThreshold.Seconds()))
	if err != nil {
		return 0, fmt.Errorf("reset orphans: %w", err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return n, fmt.Errorf("scan orphan id: %w", err)
		}
		n++
	}
	return n, rows.Err()
}
