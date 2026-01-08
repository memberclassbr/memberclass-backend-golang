# Transcription Jobs

This directory contains jobs related to video transcription processing.

## Jobs Overview

### TranscriptionJob

**Schedule:** Daily at 10 PM (22:00) - `0 0 22 * * *`

**Purpose:**
The TranscriptionJob is responsible for discovering and submitting lessons that need transcription processing to the external transcription API.

**How it works:**
1. Fetches all tenants that have AI enabled
2. For each tenant, checks if there's already a pending transcription job (prevents duplicates)
3. Retrieves unprocessed lessons for the tenant (lessons with `transcriptionCompleted = false`)
4. Builds a payload with lesson data and tenant ID
5. Sends the payload to the transcription API endpoint (`/api/v2/extract-and-embed`)
6. Saves the job information to Redis for tracking

**Key Features:**
- Prevents duplicate jobs for the same tenant
- Only processes lessons that haven't been transcribed yet
- Stores job metadata in Redis for status tracking

**Dependencies:**
- `AITenantUseCase` - To fetch tenants with AI enabled
- `AILessonUseCase` - To fetch unprocessed lessons
- `Cache` (Redis) - To store job information and prevent duplicates
- `TRANSCRIPTION_API_URL` environment variable

**Redis Keys Used:**
- `transcription:jobs:list` - List of all active job IDs
- `transcription:job:{jobID}` - Individual job data (tenant ID, lesson IDs, creation date)

---

### TranscriptionStatusCheckerJob

**Schedule:** Every 10 minutes - `0 */10 * * * *`

**Purpose:**
The TranscriptionStatusCheckerJob monitors the status of active transcription jobs and updates lesson records when transcription is completed.

**How it works:**
1. Retrieves the list of pending transcription jobs from Redis
2. For each job, checks the status with the transcription API (`/api/jobs/{jobID}/status`)
3. For each lesson in the job:
   - If status is `COMPLETED`: Updates the lesson's `transcriptionCompleted` flag to `true` in the database
   - If status is `FAILED`: Logs the error message
   - If status is still processing: Keeps the job in the active list
4. When all lessons in a job are completed, removes the job from Redis
5. Updates the active jobs list in Redis

**Key Features:**
- Automatically updates lesson transcription status when processing completes
- Removes completed jobs from Redis to free up space
- Handles failed transcriptions with error logging
- Continues monitoring jobs that are still in progress

**Dependencies:**
- `AILessonUseCase` - To update lesson transcription status
- `Cache` (Redis) - To read job information and manage active jobs list
- `TRANSCRIPTION_API_URL` environment variable

**Redis Keys Used:**
- `transcription:jobs:list` - List of all active job IDs
- `transcription:job:{jobID}` - Individual job data

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
2. Check for pending jobs (prevent duplicates)
    ↓
3. Fetch unprocessed lessons
    ↓
4. Send to transcription API
    ↓
5. Save job to Redis
    ↓
    [Job is now being processed by external API]
    ↓
TranscriptionStatusCheckerJob (runs every 10 minutes)
    ↓
1. Get list of active jobs from Redis
    ↓
2. Check status of each job via API
    ↓
3. Update lesson records when completed
    ↓
4. Remove completed jobs from Redis
```

## Error Handling

- If `TRANSCRIPTION_API_URL` is not configured, jobs will log an error and skip execution
- API errors are logged with status codes and response bodies
- Failed lesson transcriptions are logged but don't stop the job from checking other lessons
- Redis connection errors are handled gracefully

