package transcription

// Enum values for the Railway pgvector database. These mirror the public.*
// USER-DEFINED types confirmed via Task 0 (2026-05-13). If a deploy drifts
// the schema, update this file in lockstep — the slice does NOT validate
// enum membership at runtime; bad strings produce 22P02 errors at
// INSERT/UPDATE time.
const (
	JobStatusPending   = "PENDING"
	JobStatusRunning   = "RUNNING"
	JobStatusCompleted = "COMPLETED"
	JobStatusFailed    = "FAILED"
	JobStatusCancelled = "CANCELLED"

	// JobTypeVideoProcessing groups the whole pipeline (download → audio →
	// Whisper → chunk → embed). The legacy schema also has
	// EMBEDDING_GENERATION for embedding-only reruns; we don't enqueue
	// those yet but keep the constant available.
	JobTypeVideoProcessing     = "VIDEO_PROCESSING"
	JobTypeEmbeddingGeneration = "EMBEDDING_GENERATION"

	VideoStatusPending              = "PENDING"
	VideoStatusDownloading          = "DOWNLOADING"
	VideoStatusExtractingAudio      = "EXTRACTING_AUDIO"
	VideoStatusTranscribing         = "TRANSCRIBING"
	VideoStatusChunking             = "CHUNKING"
	VideoStatusGeneratingEmbeddings = "GENERATING_EMBEDDINGS"
	VideoStatusCompleted            = "COMPLETED"
	VideoStatusFailed               = "FAILED"

	SourceTypeBunnyCDN = "BUNNY_CDN"
)

// ============================================================================
// Railway pgvector queries (DB_TRANSCRIPTION_DSN)
// ============================================================================

// sqlClaimJobs atomically grabs up to $1 PENDING rows of type
// VIDEO_PROCESSING and flips them to RUNNING. FOR UPDATE SKIP LOCKED makes
// the claim concurrent-safe so multiple worker goroutines do not collide
// (Postgres-only — CockroachDB would need a different pattern, but this DB
// is plain Postgres on Railway).
const sqlClaimJobs = `
    UPDATE jobs
       SET status     = 'RUNNING',
           started_at = now(),
           attempts   = attempts + 1,
           updated_at = now()
     WHERE id IN (
        SELECT id FROM jobs
         WHERE status = 'PENDING'
           AND type   = 'VIDEO_PROCESSING'
           AND attempts < max_attempts
         ORDER BY priority DESC, created_at ASC
         FOR UPDATE SKIP LOCKED
         LIMIT $1
     )
    RETURNING id, tenant_id, payload, attempts, max_attempts
`

const sqlMarkJobCompleted = `
    UPDATE jobs
       SET status       = 'COMPLETED',
           completed_at = now(),
           result       = $2::jsonb,
           updated_at   = now()
     WHERE id = $1
`

// sqlMarkJobFailed bumps a job back to PENDING if it has retries left;
// otherwise terminates it as FAILED. RETURNING lets the caller log the
// transition.
const sqlMarkJobFailed = `
    UPDATE jobs
       SET status     = CASE WHEN attempts >= max_attempts THEN 'FAILED' ELSE 'PENDING' END,
           failed_at  = CASE WHEN attempts >= max_attempts THEN now() ELSE failed_at END,
           error      = $2,
           updated_at = now()
     WHERE id = $1
    RETURNING status
`

// sqlResetOrphans pushes RUNNING rows older than $1 seconds back to PENDING
// — covers the crash-mid-run case where the worker died before it could
// flip status. attempts < max_attempts guards against infinite loops.
const sqlResetOrphans = `
    UPDATE jobs
       SET status = 'PENDING', updated_at = now()
     WHERE status = 'RUNNING'
       AND started_at < now() - ($1 * interval '1 second')
       AND attempts < max_attempts
    RETURNING id
`

const sqlGetJobStatus = `
    SELECT id, tenant_id, status, attempts, error, started_at, completed_at, payload, result
      FROM jobs
     WHERE id = $1
`

// sqlInsertJob is used by both the HTTP enqueue handler and the daily cron
// to push one TRANSCRIPTION job per unprocessed lesson.
const sqlInsertJob = `
    INSERT INTO jobs (id, tenant_id, type, status, priority, payload, max_attempts, created_at, updated_at)
    VALUES ($1, $2, 'VIDEO_PROCESSING', 'PENDING', $3, $4::jsonb, $5, now(), now())
`

// sqlUpsertVideo keyed on (tenant_id, source_url). On conflict we keep the
// pre-existing id and refresh status/updated_at so the pipeline can pick
// up a reprocess without orphaning chunks.
const sqlUpsertVideo = `
    INSERT INTO videos (
        id, tenant_id, course_id, lesson_id, title, source_type, source_url,
        status, duration, metadata, created_at, updated_at
    ) VALUES (
        $1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, now(), now()
    )
    ON CONFLICT (tenant_id, source_url) DO UPDATE SET
        status     = EXCLUDED.status,
        updated_at = now(),
        lesson_id  = EXCLUDED.lesson_id,
        duration   = COALESCE(EXCLUDED.duration, videos.duration)
    RETURNING id
`

const sqlUpdateVideoStatus = `
    UPDATE videos
       SET status       = $2,
           updated_at   = now(),
           processed_at = CASE WHEN $2 = 'COMPLETED' THEN now() ELSE processed_at END,
           error        = NULLIF($3, '')
     WHERE id = $1
`

// Reprocessing housekeeping: drop prior chunks/transcripts so we don't end
// up with stale text alongside fresh.
const sqlDeleteChunksByVideo      = `DELETE FROM chunks      WHERE video_id = $1`
const sqlDeleteTranscriptsByVideo = `DELETE FROM transcripts WHERE video_id = $1`

const sqlInsertTranscript = `
    INSERT INTO transcripts (
        id, video_id, tenant_id, lesson_id, text, language, model, confidence,
        segments, processing_time, metadata, created_at, updated_at
    ) VALUES (
        $1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11::jsonb, now(), now()
    )
`

// chunksTable + chunksColumns are consumed by lib/pq's CopyIn helper in
// pipeline.go — that's the fastest way to bulk-insert thousands of chunk
// rows with vector embeddings. Order MUST match the call site exactly.
const chunksTable = `chunks`

var chunksColumns = []string{
	"id", "video_id", "transcript_id", "tenant_id", "course_id", "lesson_id",
	"text", "order", "start_time", "end_time", "embedding", "embedding_model",
	"provider", "metadata", "created_at", "updated_at",
}

const sqlInsertTokenUsage = `
    INSERT INTO token_usage (
        id, tenant_id, course_id, video_id, transcript_id,
        prompt_tokens, completion_tokens, total_tokens,
        input_cost_cents, output_cost_cents, total_cost_cents,
        model, operation, metadata, created_at
    ) VALUES (
        $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14::jsonb, now()
    )
`

// ============================================================================
// memberclass DB queries (DB_DSN — CockroachDB-compatible)
// ============================================================================

// sqlSelectTenantBunnyCreds returns the Bunny credentials + AI flag for the
// given tenant. Columns are quoted because they're camelCase in the Prisma-
// authored schema.
const sqlSelectTenantBunnyCreds = `
    SELECT id, name, "aiEnabled", "bunnyLibraryId", "bunnyLibraryApiKey"
      FROM "Tenant"
     WHERE id = $1
`

// sqlSelectUnprocessedLessons walks the Vitrine → Course → Section → Module
// → Lesson hierarchy (Lesson has no direct tenantId — the relationship is
// owned by Vitrine). Filters mirror the legacy lesson_repository.go pattern
// so the cron picks the same population the old service did.
const sqlSelectUnprocessedLessons = `
    SELECT l.id,
           l.name,
           l.slug,
           l.type,
           l."mediaUrl",
           l.thumbnail,
           l.content,
           m.id   AS module_id,
           m.name AS module_name,
           s.id   AS section_id,
           s.name AS section_name,
           c.id   AS course_id,
           c.name AS course_name,
           v.id   AS vitrine_id,
           v.name AS vitrine_name
      FROM "Lesson" l
      JOIN "Module"  m ON l."moduleId"  = m.id
      JOIN "Section" s ON m."sectionId" = s.id
      JOIN "Course"  c ON s."courseId"  = c.id
      JOIN "Vitrine" v ON c."vitrineId" = v.id
     WHERE v."tenantId"               = $1
       AND l.published                 = true
       AND COALESCE(l."transcriptionCompleted", false) = false
       AND l."mediaUrl" LIKE '%https://iframe.mediadelivery.net%'
     ORDER BY COALESCE(v."order", 0) ASC,
              COALESCE(c."order", 0) ASC,
              COALESCE(s."order", 0) ASC,
              COALESCE(m."order", 0) ASC,
              COALESCE(l."order", 0) ASC
`

const sqlMarkLessonTranscribed = `
    UPDATE "Lesson"
       SET "transcriptionCompleted" = true,
           "updatedAt"              = NOW()
     WHERE id = $1
`

const sqlMarkLessonTranscriptionStatus = `
    UPDATE "Lesson"
       SET "transcriptionCompleted" = $2,
           "updatedAt"              = NOW()
     WHERE id = $1
`

const sqlSelectAITenants = `
    SELECT id, name, "bunnyLibraryId", "bunnyLibraryApiKey"
      FROM "Tenant"
     WHERE "aiEnabled" = true
`
