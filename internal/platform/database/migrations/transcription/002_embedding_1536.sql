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
--
-- No BEGIN/COMMIT: MigrateTranscription already runs this file inside a
-- transaction, and committing here would end that one early — the row in
-- schema_migrations_go would then land outside it.
--
-- The whole body is conditional on chunks.embedding still being the 768d
-- column, because the deletes here are unconditional and this file has never
-- applied through the runner: 000 failed at boot for as long as it carried a
-- psql meta-command, so the runner never reached 002 and never wrote its row
-- in schema_migrations_go. A database that was migrated by hand with
-- `psql -f` — which is what these headers used to instruct — is therefore
-- already at 1536 while the runner still believes 002 is pending. Without the
-- guard, the first boot after the 000 fix would delete live embeddings that
-- the new pipeline produced.
--
-- Re-running when the column is already vector(1536) is exactly the case where
-- there is nothing to migrate, so the guard skips the index rebuild too.
DO $$
BEGIN
    IF (
        SELECT format_type(a.atttypid, a.atttypmod)
          FROM pg_attribute a
         WHERE a.attrelid = 'public.chunks'::regclass
           AND a.attname  = 'embedding'
           AND NOT a.attisdropped
    ) = 'vector(1536)' THEN
        RAISE NOTICE 'chunks.embedding is already vector(1536); nothing to migrate';
        RETURN;
    END IF;

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
END $$;
