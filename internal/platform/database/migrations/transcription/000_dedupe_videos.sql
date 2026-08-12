-- One-time dedupe of public.videos so the UNIQUE (tenant_id, source_url)
-- index added in 001_pgvector_index.sql can be created without conflict.
--
-- Survivor rule per (tenant_id, source_url):
--   1. status = 'COMPLETED' wins over anything else.
--   2. Among equal-status rows, the latest created_at wins.
--
-- FK chain (no ON DELETE CASCADE on the legacy schema, so delete bottom-up):
--   chunks.video_id  -> videos.id
--   transcripts.video_id -> videos.id
--   (token_usage.video_id is informational, no FK constraint to enforce)
--
-- Safe to re-run: it operates only on rows that are still duplicated.

BEGIN;

CREATE TEMP TABLE _video_losers AS
SELECT id
  FROM (
    SELECT id,
           row_number() OVER (
               PARTITION BY tenant_id, source_url
               ORDER BY (status = 'COMPLETED') DESC, created_at DESC, id
           ) AS rn
      FROM public.videos
  ) ranked
 WHERE rn > 1;

SELECT format('deleting %s duplicate videos', COUNT(*)) FROM _video_losers \gset

DELETE FROM public.chunks      WHERE video_id IN (SELECT id FROM _video_losers);
DELETE FROM public.transcripts WHERE video_id IN (SELECT id FROM _video_losers);
-- token_usage carries video_id without an FK; null it out so historical cost
-- rows survive the cleanup but no longer point at deleted videos.
UPDATE public.token_usage SET video_id = NULL WHERE video_id IN (SELECT id FROM _video_losers);
DELETE FROM public.videos      WHERE id IN (SELECT id FROM _video_losers);

DROP TABLE _video_losers;

COMMIT;
