-- Migrate chunks.embedding from vector(768) to vector(1536).
--
-- The legacy data was produced by Gemini text-embedding-004 (768 dims).
-- The new in-process pipeline uses OpenAI text-embedding-3-small (1536
-- dims). Embeddings from different models live in different spaces, so
-- the legacy rows are unusable for similarity search alongside new ones;
-- the cleanest path is to drop them and let the admin UI re-trigger
-- transcription per selected lesson.
--
-- All statements scoped to the legacy chunks/transcripts/videos rows
-- only. Other tables (jobs, token_usage, memberclass_tenant_mappings,
-- webhook_*) are left untouched.

BEGIN;

-- Order matters because of the chunks → transcripts → videos FK chain
-- (no ON DELETE CASCADE on the legacy schema).
DELETE FROM chunks;
DELETE FROM transcripts;
DELETE FROM videos;

-- ALTER refuses to widen vector dimension when rows exist; safe now that
-- the table is empty.
ALTER TABLE chunks
    ALTER COLUMN embedding TYPE vector(1536);

-- Old HNSW index was built against the 768d space and is now bogus.
-- Drop both: the one Bunny/Gemini era left behind, AND the one our
-- 001 migration created (which would still be 768d). Recreate against
-- the new 1536d vector type.
DROP INDEX IF EXISTS idx_chunks_embedding_hnsw;
DROP INDEX IF EXISTS chunks_embedding_hnsw_cosine;

CREATE INDEX chunks_embedding_hnsw_cosine
    ON chunks USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

COMMIT;
