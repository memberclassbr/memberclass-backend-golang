package transcription

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// jobPayload is the JSON shape we INSERT into jobs.payload when enqueueing
// a TRANSCRIPTION-style job (handler + cron). One job = one lesson.
type jobPayload struct {
	LessonID string `json:"lessonId"`
	TenantID string `json:"tenantId"`
	VideoURL string `json:"videoUrl"`         // lesson.mediaUrl (Bunny embed URL)
	CourseID string `json:"courseId,omitempty"`
	Title    string `json:"title,omitempty"`
}

// jobResult is written to jobs.result on COMPLETED. Useful for the GET
// /jobs/{id} status response so the caller knows where the chunks landed.
type jobResult struct {
	VideoID      string  `json:"videoId"`
	TranscriptID string  `json:"transcriptId"`
	ChunksCount  int     `json:"chunksCount"`
	DurationSecs float64 `json:"durationSeconds"`
	CostCents    int     `json:"costCents"`
}

// resolveAudioFunc lets tests bypass Bunny + ffmpeg by injecting an audio
// resolver that returns local file paths. Production leaves it nil and the
// real Bunny meta + HLS download + ffmpeg split runs.
type resolveAudioFunc func(ctx context.Context, libID, guid, accessKey, tmpDir string) (parts []string, duration float64, err error)

// executeJob runs the full pipeline for a single TRANSCRIPTION job that
// the worker pool has already claimed (status=RUNNING). On success it
// commits chunks/transcripts/token_usage on the transcription DB, flips
// the memberclass Lesson row, and marks the job COMPLETED. On any error
// it returns and the caller is expected to mark the job FAILED (with the
// retry decision left to sqlMarkJobFailed).
func (f *Feature) executeJob(ctx context.Context, jobID, tenantID string, rawPayload []byte) error {
	if err := f.preflight(); err != nil {
		return err
	}

	var p jobPayload
	if err := json.Unmarshal(rawPayload, &p); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	if p.LessonID == "" || p.VideoURL == "" {
		return fmt.Errorf("payload missing lessonId/videoUrl")
	}

	// 1. Tenant lookup + AI guard.
	var (
		tID, tName             string
		aiEnabled              sql.NullBool
		bunnyLibID, bunnyAPIKey sql.NullString
	)
	row := f.memberclassDB.QueryRowContext(ctx, sqlSelectTenantBunnyCreds, tenantID)
	if err := row.Scan(&tID, &tName, &aiEnabled, &bunnyLibID, &bunnyAPIKey); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("tenant %s not found", tenantID)
		}
		return fmt.Errorf("select tenant: %w", err)
	}
	if !aiEnabled.Valid || !aiEnabled.Bool {
		return fmt.Errorf("tenant %s has aiEnabled=false", tenantID)
	}
	if !bunnyLibID.Valid || bunnyLibID.String == "" || !bunnyAPIKey.Valid || bunnyAPIKey.String == "" {
		return fmt.Errorf("tenant %s missing Bunny credentials", tenantID)
	}

	// 2. Extract libraryId + guid from the iframe URL. We trust the URL
	// over the tenant's library id; if they disagree it's a config error
	// the operator needs to fix manually.
	libID, guid, err := guidFromEmbedURL(p.VideoURL)
	if err != nil {
		return fmt.Errorf("parse media URL: %w", err)
	}
	if libID != bunnyLibID.String {
		f.log.Warn("transcription.pipeline.bunny_library_mismatch",
			"tenant", tenantID, "tenantLibraryId", bunnyLibID.String, "urlLibraryId", libID)
	}

	// 3. Resolve playable audio. Production: validate via Bunny meta then
	// pull HLS through ffmpeg. Tests inject testHookResolveAudio to skip
	// the network round-trip.
	tmpDir, err := os.MkdirTemp("", "tx_"+jobID+"_")
	if err != nil {
		return fmt.Errorf("mktemp: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	parts, duration, err := f.resolveAudio(ctx, libID, guid, bunnyAPIKey.String, tmpDir)
	if err != nil {
		return fmt.Errorf("resolve audio: %w", err)
	}
	if len(parts) == 0 {
		return fmt.Errorf("no audio parts produced")
	}

	// 4. Transcribe each part; concatenate text + segments with timestamp
	// offsets so a chunked-audio lesson still produces a single coherent
	// transcript.
	var allSegments []whisperSegment
	var transcriptText strings.Builder
	var elapsed float64
	costCents := 0
	for i, part := range parts {
		fh, err := os.Open(part)
		if err != nil {
			return fmt.Errorf("open part %d (%s): %w", i, part, err)
		}
		resp, err := f.transcribeAudio(ctx, fh, filepath.Base(part))
		_ = fh.Close()
		if err != nil {
			return fmt.Errorf("whisper part %d: %w", i, err)
		}
		for _, s := range resp.Segments {
			allSegments = append(allSegments, whisperSegment{
				Start: s.Start + elapsed,
				End:   s.End + elapsed,
				Text:  s.Text,
			})
		}
		transcriptText.WriteString(strings.TrimSpace(resp.Text))
		transcriptText.WriteString(" ")
		elapsed += resp.Duration
		costCents += whisperCostCents(resp.Duration)
	}
	if duration == 0 {
		duration = elapsed
	}
	if transcriptText.Len() == 0 {
		return fmt.Errorf("whisper returned empty transcript")
	}

	// 5. Chunk + embed.
	chunks := splitIntoChunks(allSegments, 500, 50)
	if len(chunks) == 0 {
		return fmt.Errorf("chunker produced 0 chunks (segments=%d)", len(allSegments))
	}
	embeddings := make([][]float32, len(chunks))
	for start := 0; start < len(chunks); start += embedBatchSize {
		end := start + embedBatchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		texts := make([]string, end-start)
		for i := range texts {
			texts[i] = chunks[start+i].Text
		}
		vecs, tokens, err := f.embedBatch(ctx, texts)
		if err != nil {
			return fmt.Errorf("embed batch [%d:%d]: %w", start, end, err)
		}
		if len(vecs) != end-start {
			return fmt.Errorf("embed batch [%d:%d]: returned %d vectors, want %d", start, end, len(vecs), end-start)
		}
		copy(embeddings[start:end], vecs)
		costCents += embedCostCents(tokens)
	}

	// 6. Persist on the transcription DB in a single transaction so the
	// chunks/transcript/video rows always land together or not at all.
	tx, err := f.transcriptionDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	videoID := uuid.NewString()
	videoMetadata, _ := json.Marshal(map[string]any{
		"jobId":         jobID,
		"embeddingModel": embedModel,
		"transcriber":   whisperModel,
	})
	if err := tx.QueryRowContext(ctx, sqlUpsertVideo,
		videoID, tenantID, p.CourseID, p.LessonID, p.Title,
		SourceTypeBunnyCDN, p.VideoURL, VideoStatusGeneratingEmbeddings, duration, videoMetadata,
	).Scan(&videoID); err != nil {
		return fmt.Errorf("upsert video: %w", err)
	}

	// Reprocess housekeeping: drop any prior chunks/transcripts that
	// belonged to a previous run of this video so the new ones don't sit
	// next to stale data.
	if _, err := tx.ExecContext(ctx, sqlDeleteChunksByVideo, videoID); err != nil {
		return fmt.Errorf("delete prior chunks: %w", err)
	}
	if _, err := tx.ExecContext(ctx, sqlDeleteTranscriptsByVideo, videoID); err != nil {
		return fmt.Errorf("delete prior transcripts: %w", err)
	}

	transcriptID := uuid.NewString()
	segmentsJSON, _ := json.Marshal(allSegments)
	transcriptMeta, _ := json.Marshal(map[string]any{"jobId": jobID})
	if _, err := tx.ExecContext(ctx, sqlInsertTranscript,
		transcriptID, videoID, tenantID, p.LessonID,
		strings.TrimSpace(transcriptText.String()),
		"pt", whisperModel, nil, segmentsJSON, elapsed, transcriptMeta,
	); err != nil {
		return fmt.Errorf("insert transcript: %w", err)
	}

	// Bulk-insert chunks. lib/pq's CopyIn is the fastest path for
	// thousands of rows with vectors; the column order MUST match
	// chunksColumns exactly.
	stmt, err := tx.PrepareContext(ctx, pq.CopyInSchema("public", chunksTable, chunksColumns...))
	if err != nil {
		return fmt.Errorf("prepare copyin: %w", err)
	}
	now := time.Now()
	for i, c := range chunks {
		if _, err := stmt.ExecContext(ctx,
			uuid.NewString(),
			videoID,
			transcriptID,
			tenantID,
			nullableString(p.CourseID),
			p.LessonID,
			c.Text,
			c.Order,
			c.StartTime,
			c.EndTime,
			pgvectorString(embeddings[i]),
			embedModel,
			"openai",
			"{}",
			now,
			now,
		); err != nil {
			_ = stmt.Close()
			return fmt.Errorf("copy chunk %d: %w", i, err)
		}
	}
	if _, err := stmt.ExecContext(ctx); err != nil {
		_ = stmt.Close()
		return fmt.Errorf("flush copyin: %w", err)
	}
	if err := stmt.Close(); err != nil {
		return fmt.Errorf("close copyin: %w", err)
	}

	if _, err := tx.ExecContext(ctx, sqlUpdateVideoStatus, videoID, VideoStatusCompleted, ""); err != nil {
		return fmt.Errorf("mark video completed: %w", err)
	}

	tokenMeta, _ := json.Marshal(map[string]any{
		"chunks":   len(chunks),
		"duration": elapsed,
	})
	if _, err := tx.ExecContext(ctx, sqlInsertTokenUsage,
		uuid.NewString(), tenantID, nullableString(p.CourseID), videoID, transcriptID,
		0, 0, 0, costCents, 0, costCents,
		whisperModel+"+"+embedModel, "transcribe+embed", tokenMeta,
	); err != nil {
		return fmt.Errorf("insert token_usage: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transcription tx: %w", err)
	}

	// 7. Flip the memberclass flag AFTER the pgvector commit. If this
	// fails the chunks are already safe; a later run will be idempotent
	// via the UNIQUE (tenant_id, source_url) index, so we log + continue.
	if _, err := f.memberclassDB.ExecContext(ctx, sqlMarkLessonTranscribed, p.LessonID); err != nil {
		f.log.Error("transcription.pipeline.mark_lesson_failed",
			"error", err.Error(), "lessonId", p.LessonID, "videoId", videoID)
	}

	// 8. Final: mark the job COMPLETED with a result blob the GET
	// /jobs/{id} handler can serialize back to the caller.
	result, _ := json.Marshal(jobResult{
		VideoID:      videoID,
		TranscriptID: transcriptID,
		ChunksCount:  len(chunks),
		DurationSecs: elapsed,
		CostCents:    costCents,
	})
	if _, err := f.transcriptionDB.ExecContext(ctx, sqlMarkJobCompleted, jobID, result); err != nil {
		f.log.Error("transcription.pipeline.mark_job_failed",
			"error", err.Error(), "jobId", jobID, "videoId", videoID)
		return fmt.Errorf("mark job completed: %w", err)
	}
	return nil
}

// resolveAudio either delegates to the test hook or runs the real Bunny
// validation + HLS download + ffmpeg pipeline.
func (f *Feature) resolveAudio(ctx context.Context, libID, guid, accessKey, tmpDir string) ([]string, float64, error) {
	if f.testHookResolveAudio != nil {
		return f.testHookResolveAudio(ctx, libID, guid, accessKey, tmpDir)
	}

	meta, err := f.fetchBunnyVideoMeta(ctx, libID, guid, accessKey)
	if err != nil {
		return nil, 0, err
	}
	hlsURL := buildHLSURL(libID, guid)

	full := filepath.Join(tmpDir, "audio.mp3")
	if _, err := extractAudioMP3(ctx, hlsURL, full); err != nil {
		return nil, 0, err
	}

	info, err := os.Stat(full)
	if err != nil {
		return nil, 0, err
	}
	if info.Size() <= whisperMaxAudioBytes {
		return []string{full}, meta.Length, nil
	}

	parts, err := splitAudioByDuration(ctx, full, tmpDir, 600)
	if err != nil {
		return nil, 0, err
	}
	return parts, meta.Length, nil
}

// pgvectorString encodes a float32 slice in the literal `[v1,v2,...]`
// format pgvector accepts on INSERT. Using `%g` keeps the textual length
// reasonable while still round-tripping precision for our 32-bit inputs.
func pgvectorString(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.Grow(len(v) * 12)
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(x), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

// nullableString returns nil for an empty string so empty course_id /
// lesson_id values land in the DB as NULL rather than '' (the legacy
// schema treats both as `nullable text` and downstream RAG filters
// rely on IS NULL semantics).
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
