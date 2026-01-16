# Transcription Jobs

This directory contains jobs related to video transcription processing.

## Jobs Overview

### TranscriptionJob

**Schedule:** Daily at 10 PM (22:00) - `0 0 22 * * *`

**Purpose:**
The TranscriptionJob is responsible for discovering and submitting lessons that need transcription processing to the external transcription API.

**How it works:**
1. Fetches all tenants that have AI enabled
2. For each tenant, retrieves unprocessed lessons (lessons with `transcriptionCompleted = false`)
3. Builds a payload with lesson data and tenant ID
4. Sends the payload to the transcription API endpoint (`/api/v2/extract-and-embed`)

**Key Features:**
- Processes all tenants with AI enabled
- Only processes lessons that haven't been transcribed yet
- Simple and straightforward approach

**Dependencies:**
- `AITenantUseCase` - To fetch tenants with AI enabled
- `AILessonUseCase` - To fetch unprocessed lessons
- `TRANSCRIPTION_API_URL` environment variable

---

## Environment Variables

Both jobs require the following environment variable:

- `TRANSCRIPTION_API_URL` - Base URL of the transcription API service

## Data Flow

```
TranscriptionJob
    ↓
1. Fetch tenants with AI enabled
    ↓
2. Fetch unprocessed lessons for each tenant
    ↓
3. Send lessons to transcription API
    ↓
    [Job is now being processed by external API]
    ↓
    [Use API endpoint PATCH /api/v1/ai/lessons/{lessonId} to update transcriptionCompleted manually]
```

## Error Handling

- If `TRANSCRIPTION_API_URL` is not configured, jobs will log an error and skip execution
- API errors are logged with status codes and response bodies
- Failed lesson transcriptions are logged but don't stop the job from checking other lessons
- Redis connection errors are handled gracefully

