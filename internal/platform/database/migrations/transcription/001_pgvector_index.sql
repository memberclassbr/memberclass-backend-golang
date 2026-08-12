-- Migration for the Railway pgvector database (DB_TRANSCRIPTION_DSN).
-- This DB is NOT managed by the in-app MigrationService (which targets the
-- memberclass DB). Run manually before deploying the transcription slice:
--
--     psql "$DB_TRANSCRIPTION_DSN" -f migrations/transcription/001_pgvector_index.sql
--
-- Prereq: the Railway service must be created from the "PostgreSQL pgvector"
-- template; the vanilla Postgres image does not ship the vector binary.
--
-- All statements are idempotent.

-- 1. Vector extension (no-op if Supabase already has it enabled).
CREATE EXTENSION IF NOT EXISTS vector;

-- 2. HNSW index for cosine similarity search on chunks.embedding.
--    m=16, ef_construction=64 are pgvector defaults; tune later if recall drops.
--    Pick HNSW over IVFFlat: better recall at <1M chunks, no training step.
CREATE INDEX IF NOT EXISTS chunks_embedding_hnsw_cosine
    ON chunks USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

-- 3. Idempotência de reprocessamento: o pipeline UPSERTa videos por
--    (tenant_id, source_url). Sem este índice o ON CONFLICT não tem alvo.
--
--    PRE-FLIGHT: verifique se há duplicatas antes de aplicar, senão falha.
--      SELECT tenant_id, source_url, COUNT(*) FROM videos
--        GROUP BY 1, 2 HAVING COUNT(*) > 1;
CREATE UNIQUE INDEX IF NOT EXISTS videos_unique_tenant_source
    ON videos (tenant_id, source_url);

-- 4. Acelera o claim de jobs PENDING pelo worker pool.
CREATE INDEX IF NOT EXISTS jobs_pending_priority
    ON jobs (status, priority DESC, created_at ASC)
    WHERE status = 'PENDING';

-- 5. Suporta o orphan recovery scan (rows presas em RUNNING após crash).
CREATE INDEX IF NOT EXISTS jobs_running_started_at
    ON jobs (status, started_at)
    WHERE status = 'RUNNING';
