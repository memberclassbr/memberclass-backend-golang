-- Adds PANDA_VIDEO to the video source enum so the transcription slice
-- can persist Panda-sourced videos. Safe to re-run.
ALTER TYPE video_source_type ADD VALUE IF NOT EXISTS 'PANDA_VIDEO';
