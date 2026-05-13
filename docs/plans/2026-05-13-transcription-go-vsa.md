# Transcription Pipeline — 100% Go (VSA) Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task.

**Goal:** Substituir o serviço externo de transcrição (ai-transcriber.9ho8ul.easypanel.host) por uma slice VSA `internal/features/workers/transcription/` que executa todo o pipeline (download Bunny → áudio → Whisper → chunk → embed → pgvector) dentro do próprio binário Go, eliminando dependência de microsserviço externo.

**Architecture:** Slice VSA (`internal/features/workers/transcription/`) que: (1) expõe rotas `/api/v1/ai/...` herdando das atuais; (2) enfileira jobs em `public.jobs` no Supabase pgvector dedicado; (3) roda worker pool in-process que polleia `jobs` e executa o pipeline; (4) reutiliza schema legado Supabase intacto (`videos`/`transcripts`/`chunks`/`jobs`/`token_usage`); (5) é registrado no FX em `cmd/api/main.go` e `startApplication` igual ao `notifications` worker.

**Tech Stack:**
- Go 1.25 + chi + `database/sql` + `lib/pq` + `github.com/google/uuid`
- OpenAI HTTP API direto (`/v1/audio/transcriptions` whisper-1, `/v1/embeddings` text-embedding-3-small)
- pgvector (Supabase legado; extensão `vector` já existente)
- ffmpeg como dependência runtime do container (para HLS→MP3 e fatiamento >25MB)
- Bunny CDN (HLS/MP4 download via library API)
- `tiktoken-go` (cl100k_base) para contagem de tokens
- go-sqlmock + testify para testes

---

## FASE 1 — Reconhecimento (resumo)

### Estado atual do código a substituir

| Item | Caminho | Papel |
|---|---|---|
| Use case + HTTP externo | `internal/domain/usecases/ai/ai_tenant_usecase.go:87-218` | `ProcessLessonsTenant` faz POST para externo |
| Cron diário | `internal/application/jobs/transcription/transcription_job.go` | Roda às 22:00 (`0 0 22 * * *`) — também faz POST externo |
| Scheduler init | `internal/application/jobs/scheduler.go:95-98` | Registra `TranscriptionJob` |
| DTOs | `internal/domain/dto/transcription_job.go` | Payload externo + status |
| Handler HTTP | `internal/application/handlers/http/ai/ai_tenant_handler.go` | `POST /api/v1/ai/tenants/process-lessons` |
| Handler do callback | `internal/application/handlers/http/ai/ai_lesson_handler.go:50` | `PATCH /api/v1/ai/lessons/{id}` recebe callback do externo |
| Use case AI lesson | `internal/domain/usecases/ai/ai_lesson_usecase.go` | `GetLessons` + `UpdateTranscriptionStatus` |
| Flag persistente | `internal/infrastructure/adapters/repository/lesson/lesson_repository.go:824` | `transcriptionCompleted bool` na tabela `lesson` do DB memberclass |
| Env | `.env.example:55` | `TRANSCRIPTION_API_URL=http://localhost:3000` |
| Router | `internal/application/router/router.go:162` | Monta `/process-lessons` |

### Padrões VSA já consolidados (referência obrigatória)

- **Slice "api"** (`internal/features/api/activity_summary/`): `deps.go` (Feature struct + `New`) → `routes.go` (`Register(r chi.Router, mw MiddlewareSet)`) → `<action>.go` em 3 seções (handler → regra → SQL).
- **Slice "worker"** (`internal/features/workers/notifications/`): `deps.go` + `worker.go` (`Start(ctx)` / `Stop(timeout)`, ticker `pollInterval`, `claim`-`dispatch`-`progress`-`markSent`/`markFailed`).
- **Slice "admin"** (`internal/features/admin/member_import/`): híbrido — handler HTTP + worker em goroutines + retention job.
- **Registração**: FX em `cmd/api/main.go` (lista de `fx.Provide`) + `startApplication` puxa o `*Feature`, chama `.Start(ctx)`, e o router chama `feature.Register(...)` em `router.go`.

### Schema Supabase (já existe — não criar)

Enums (USER-DEFINED): `event`, `job_type`, `job_status`, `video_status`, `source_type`, `webhook_delivery_status`. Valores assumidos a confirmar **na Task 0**:
- `job_status`: `PENDING | RUNNING | COMPLETED | FAILED`
- `job_type`: contém `TRANSCRIPTION` (valor exato a verificar)
- `video_status`: `PENDING | DOWNLOADING | TRANSCRIBING | EMBEDDING | COMPLETED | FAILED`
- `source_type`: `BUNNY | URL` (provavel — verificar)

### Lacuna chave

`lesson.mediaUrl` é o **iframe** Bunny (`https://iframe.mediadelivery.net/embed/{libraryId}/{guid}`). Para baixar o vídeo real, precisamos da URL HLS ou MP4 fallback do Bunny — extraída via `GET https://video.bunnycdn.com/library/{libraryId}/videos/{guid}` com `AccessKey`. As credentials do tenant já estão em `tenant.BunnyLibraryID/BunnyLibraryApiKey` (e em `memberclass_tenant_mappings` no Supabase, mas não vamos usar essa tabela — single source of truth = DB memberclass).

---

## FASE 2 — Brainstorm

### Decisão 1: Worker concorrente — LISTEN/NOTIFY vs polling

Opções:
- **A. Polling** (claim + UPDATE…RETURNING a cada N segundos). Já é o padrão do `notifications` worker. Simples, robusto, latência aceitável (30s).
- **B. LISTEN/NOTIFY pg**. Reativo, latência <1s. Precisa conexão dedicada pinada. Mais código.

**Escolha:** A (polling 30s). Consistência com `notifications` worker; transcrição é assíncrona por natureza (vídeo de 30min leva minutos para processar). LISTEN/NOTIFY pode ser feature futura.

### Decisão 2: Pipeline síncrono na rota vs enfileiramento

- **A. Rota síncrona** (executa o pipeline na request HTTP). Trava por minutos; timeout chi default 60s mata; ruim.
- **B. Enfileira job + responde 202** (rota retorna `jobId`, worker processa). Status consultável via `GET /api/v1/ai/jobs/{jobId}`.

**Escolha:** B. Vital: vídeo de 30min consome ~30s só Whisper. Cliente precisa polling/webhook para acompanhar.

### Decisão 3: Granularidade do job

- **A. Job = tenant** (1 job processa N lições do tenant). Atual no externo.
- **B. Job = lesson** (1 job por lesson). Retries finos, paralelismo natural.

**Escolha:** B. `jobs.payload` = `{ lessonId, tenantId, videoUrl }`. A rota `/process-lessons` enfileira N jobs (um por lesson não-processada). Cron faz o mesmo. Retry independente; falha em 1 lesson não derruba outras.

### Decisão 4: Áudio do vídeo

- **A. Bunny MP4 fallback** (`https://vz-<hash>.b-cdn.net/<guid>/play_720p.mp4`). Precisa MP4 fallback habilitado por library.
- **B. Bunny HLS** (`playlist.m3u8`) + ffmpeg para extrair só áudio.
- **C. Bunny "Original" download** via API (`/library/.../videos/{guid}/download`).

**Escolha:** B (HLS) com fallback A. ffmpeg `-i playlist.m3u8 -vn -acodec libmp3lame -ab 64k -ar 16000 -ac 1 audio.mp3` produz MP3 mono 16kHz, ideal para Whisper, pequeno (~0.5MB/min). Não baixa o vídeo inteiro localmente, streamando do HLS.

### Decisão 5: Vídeos longos (>25MB limite Whisper)

OpenAI Whisper limita upload a 25MB. MP3 64kbps mono 16kHz = ~480KB/min → ~52min cabem em 25MB. Aulas >52min precisam fatiar.

**Escolha:** Fatiar áudio em janelas de 10min via ffmpeg `-f segment -segment_time 600`. Transcrever cada segmento, concatenar texto com offset de timestamp por janela. Mais simples que VAD.

### Decisão 6: Concorrência por tenant

- **A. Sem rate limit por tenant**. Custo OpenAI descontrolado.
- **B. Rate limit global** (WORKER_CONCURRENCY default 2). Suficiente para v1.
- **C. Rate limit por tenant + global**. Mais código.

**Escolha:** B. Worker pool de 2 goroutines. Aulas processam em série dentro do worker; 2 em paralelo no servidor. Re-revisitamos se 1 tenant grande monopolizar.

### Decisão 7: Idempotência

`videos` table não tem UNIQUE em `(source_url, tenant_id)`. Reprocessar geraria duplicatas.

**Escolha:** Antes de criar `video`, fazer `SELECT id, status FROM videos WHERE source_url = $1 AND tenant_id = $2`. Se existe e status=COMPLETED, **pular** (já transcrita); se PENDING/RUNNING/FAILED, **reutilizar id** e re-rodar (DELETE chunks + transcripts associados primeiro, transação). Acoplado ao job: ao começar o job, lock no `videos` row.

### Decisão 8: ffmpeg

- **A. ffmpeg system binary** no container (Dockerfile precisa instalar).
- **B. Pure-Go (mediadevices, libav-go)**. Inexistente/incompleto para o caso.
- **C. Cloud (Bunny já gera audio_url?)**. Bunny Stream NÃO expõe áudio puro em URL pública — só HLS/MP4.

**Escolha:** A. ffmpeg é dependência aceitável (universal, single static binary). Dockerfile adiciona `apk add --no-cache ffmpeg` (alpine) ou `apt-get install -y ffmpeg` (debian-slim).

### Decisão 9: Storage da conexão Supabase

- **A. Bucket novo `transcription`** em `multi_db.go` (`DB_TRANSCRIPTION_DSN`).
- **B. *sql.DB separado fora do MultiDB**.

**Escolha:** A. Consistente com padrão `memberclass`/`ephra`/`celetusclass`. `DBMap` gerencia ciclo de vida (incl. `CloseAll()`). Slice pega via `dbMap["transcription"]`.

### Decisão 10: ApiKey OpenAI scope

- **A. `OPENAI_API_KEY` único global**. v1 viável.
- **B. Por tenant** (coluna `tenant.openai_api_key`). Cobra custo do tenant. Schema change.

**Escolha:** A. Tracking de custo em `token_usage` já permite split por tenant retroativamente. v2 considera B.

### Riscos

| Risco | Mitigação |
|---|---|
| Custo OpenAI explode | `token_usage` registra cada chamada com cost_cents; alert no Grafana fora deste PR; rate limit por concorrência global (WORKER_CONCURRENCY=2). |
| Vídeo >52min | Fatiar em janelas 10min; concatenar texto + offset timestamps. |
| Crash mid-pipeline | Job tem `status=RUNNING`, `started_at`, `attempts`. Orphan reset a cada 5min (status=RUNNING + started_at < now-30min → PENDING, attempts++). Igual `notifications` worker. |
| Bunny MP4 não habilitado para library | Detectar 404 no HLS → fallback `/videos/{guid}/play_720p.mp4` → última opção: erro "MP4 fallback disabled" no job. |
| ffmpeg ausente no container | Smoke test no startup: `exec.LookPath("ffmpeg")` em `Feature.New`; log fatal se faltar. |
| Falha parcial (chunks gravados, embed falha) | Transação **única por lesson** no Supabase: `BEGIN; INSERT video; INSERT transcript; INSERT chunks…; UPDATE job COMPLETED; COMMIT`. Update lesson.transcriptionCompleted em DB memberclass **após COMMIT supabase** (eventual consistency). |
| Schema enum mismatch | Task 0 confirma valores reais antes de codar. |
| URL Whisper retorna 401/quota | Erro grava em `jobs.error`, status=FAILED, attempts++. Sem retry automático em 401 (não vai resolver). |
| pgvector index ausente | Task 1 cria índice HNSW (melhor que IVFFlat para volumes <1M chunks). Idempotente: `CREATE INDEX IF NOT EXISTS`. |

---

## FASE 3 — Plano de Execução

### Branch e PR

- Branch: `feat/transcription-go-vsa` (saída de `main` atualizada)
- PR draft template em `~/dev/CLAUDE.md` raiz

### Estrutura final do slice

```
internal/features/workers/transcription/
  deps.go              # Feature struct + New() + Start/Stop
  routes.go            # Register(r, mw) — 3 rotas
  process_lessons.go   # handler POST /tenants/process-lessons → enfileira N jobs
  job_status.go        # handler GET /jobs/{jobId} → consulta status agregado
  update_status.go     # handler PATCH /lessons/{id}/transcription (manter compat)
  worker.go            # Start/Stop/run loop (igual notifications)
  worker_claim.go      # claimPendingJobs, resetOrphans, markRunning/Completed/Failed
  pipeline.go          # Execute(ctx, job) → orquestra etapas
  bunny_download.go    # ResolvePlaybackURL(tenant, videoGuid) → HLS url + audio extract
  audio.go             # ExtractAudioMP3 (ffmpeg via os/exec), split em janelas
  openai.go            # TranscribeAudio + EmbedBatch (HTTP direto OpenAI)
  chunker.go           # SplitIntoChunks(segments, maxTokens=500, overlap=50)
  cron.go              # daily cron 22:00 — enfileira jobs de todos tenants AI-enabled
  cost.go              # CalcCostCents(model, tokens) + WriteTokenUsage
  sql.go               # SQL constants (Supabase + memberclass)
  pipeline_test.go     # go-sqlmock end-to-end
  chunker_test.go      # table-driven
  openai_test.go       # httptest server fake
  audio_test.go        # ffmpeg presence check + smoke (mock binary?)
```

### Ordem de execução

Tasks em sequência. Cada task: TDD (test → fail → impl → pass → commit). Tasks maiores se subdividem em steps explícitos.

---

### Task 0: Verificação do schema Supabase (sem código)

**Files:** nenhum

**Step 1: Conectar ao Supabase e listar enums**

```bash
psql "$DB_TRANSCRIPTION_DSN" -c "\dT+"
psql "$DB_TRANSCRIPTION_DSN" -c "SELECT enum_range(NULL::job_type);"
psql "$DB_TRANSCRIPTION_DSN" -c "SELECT enum_range(NULL::job_status);"
psql "$DB_TRANSCRIPTION_DSN" -c "SELECT enum_range(NULL::video_status);"
psql "$DB_TRANSCRIPTION_DSN" -c "SELECT enum_range(NULL::source_type);"
psql "$DB_TRANSCRIPTION_DSN" -c "\d+ chunks"
psql "$DB_TRANSCRIPTION_DSN" -c "\d+ jobs"
psql "$DB_TRANSCRIPTION_DSN" -c "\d+ videos"
psql "$DB_TRANSCRIPTION_DSN" -c "SELECT extname, extversion FROM pg_extension WHERE extname='vector';"
psql "$DB_TRANSCRIPTION_DSN" -c "SELECT indexname, indexdef FROM pg_indexes WHERE tablename='chunks';"
```

**Expected:** Lista de valores reais dos enums + confirmação de pgvector instalado + presença/ausência de índice em `chunks.embedding`.

**Step 2: Documentar valores no topo de `sql.go`**

Coloque os enums confirmados como constants Go:

```go
// Confirmed via Task 0 (2026-05-13). If schema changes, update here.
const (
    JobStatusPending   = "PENDING"
    JobStatusRunning   = "RUNNING"
    JobStatusCompleted = "COMPLETED"
    JobStatusFailed    = "FAILED"

    JobTypeTranscription = "TRANSCRIPTION" // verify exact case

    VideoStatusPending      = "PENDING"
    VideoStatusTranscribing = "TRANSCRIBING"
    VideoStatusEmbedding    = "EMBEDDING"
    VideoStatusCompleted    = "COMPLETED"
    VideoStatusFailed       = "FAILED"

    SourceTypeBunny = "BUNNY"
)
```

**Step 3: Commit (nada para commitar) — produzir nota inline na PR**

Não há commit. Documente no PR body os valores reais e qualquer divergência do plano.

---

### Task 1: SQL — índice pgvector (idempotente)

**Files:**
- Create: `migrations/transcription/001_pgvector_index.sql` (apenas referência humana; não roda via tooling do repo — o `MigrationService` atual aponta para o DB memberclass)

**Step 1: Escrever DDL**

```sql
-- Run manually against $DB_TRANSCRIPTION_DSN before deploy.
CREATE EXTENSION IF NOT EXISTS vector;

-- HNSW index for cosine similarity search on chunks.embedding.
-- m=16, ef_construction=64 are pgvector defaults; tune later if recall drops.
CREATE INDEX IF NOT EXISTS chunks_embedding_hnsw_cosine
    ON chunks USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

-- Idempotência de reprocessamento.
CREATE UNIQUE INDEX IF NOT EXISTS videos_unique_tenant_source
    ON videos (tenant_id, source_url);

-- Acelera claim de jobs.
CREATE INDEX IF NOT EXISTS jobs_pending_priority
    ON jobs (status, priority DESC, created_at ASC)
    WHERE status = 'PENDING';

-- Orphan recovery scan.
CREATE INDEX IF NOT EXISTS jobs_running_started_at
    ON jobs (status, started_at)
    WHERE status = 'RUNNING';
```

**Step 2: Rodar manualmente**

```bash
psql "$DB_TRANSCRIPTION_DSN" -f migrations/transcription/001_pgvector_index.sql
```

Expected: `CREATE EXTENSION` (or NOTICE: already exists), 4× `CREATE INDEX`.

**Step 3: Commit**

```bash
git add migrations/transcription/001_pgvector_index.sql
git commit -m "feat(transcription): add pgvector index + idempotency unique on videos"
```

---

### Task 2: Bucket Supabase no MultiDB

**Files:**
- Modify: `internal/infrastructure/adapters/database/multi_db.go:16-20`
- Modify: `.env.example:55`

**Step 1: Test falhando — `multi_db_test.go`**

O arquivo provavelmente não existe ou não cobre buckets dinâmicos. Adicionar:

```go
// internal/infrastructure/adapters/database/multi_db_test.go
package database

import "testing"

func TestBucketDSNMapping_Transcription(t *testing.T) {
    if _, ok := bucketDSNMapping["transcription"]; !ok {
        t.Fatal("expected bucket 'transcription' in bucketDSNMapping")
    }
    if bucketDSNMapping["transcription"] != "DB_TRANSCRIPTION_DSN" {
        t.Fatalf("expected env DB_TRANSCRIPTION_DSN, got %s", bucketDSNMapping["transcription"])
    }
}
```

Run: `go test ./internal/infrastructure/adapters/database/ -run TestBucketDSNMapping_Transcription -v`
Expected: FAIL.

**Step 2: Implementar**

Editar `multi_db.go:16-20`:

```go
var bucketDSNMapping = map[string]string{
    "memberclass":   "DB_DSN",
    "ephra":         "DB_EPHRA_DSN",
    "celetusclass":  "DB_CELETUS_DSN",
    "transcription": "DB_TRANSCRIPTION_DSN",
}
```

Atualizar mensagem de erro na linha 63:

```go
return nil, fmt.Errorf("no database connections configured, check environment variables: DB_DSN, DB_EPHRA_DSN, DB_CELETUS_DSN, DB_TRANSCRIPTION_DSN")
```

Adicionar `.env.example`:

```
# Supabase dedicado para transcrições (pgvector). Tabelas: videos, transcripts,
# chunks, jobs, token_usage. Schema documentado em docs/plans/2026-05-13-transcription-go-vsa.md
DB_TRANSCRIPTION_DSN=postgresql://postgres:...@db.<project>.supabase.co:5432/postgres?sslmode=require
```

Remover linha 55 antiga `TRANSCRIPTION_API_URL=` (será deletada no cleanup; deixar agora gera lint warning desnecessário, mas adiar para Task 14).

**Step 3: Rodar test**

```bash
go test ./internal/infrastructure/adapters/database/ -v
```
Expected: PASS.

**Step 4: Commit**

```bash
git add internal/infrastructure/adapters/database/ .env.example
git commit -m "feat(db): add transcription bucket for Supabase pgvector"
```

---

### Task 3: Pacote `transcription` — esqueleto + Feature struct

**Files:**
- Create: `internal/features/workers/transcription/deps.go`

**Step 1: Test (provisional, vai expandir)**

```go
// internal/features/workers/transcription/deps_test.go
package transcription

import (
    "database/sql"
    "testing"

    "github.com/memberclass-backend-golang/internal/domain/ports"
)

func TestNew_RequiresFFmpeg(t *testing.T) {
    // ffmpeg presence is checked at New; assume CI has ffmpeg installed.
    var log ports.Logger // nil ok for this smoke — replace with fake if real
    _ = New(nil, nil, log, "fake-key")
    // No panic = pass for now.
    _ = sql.DB{}
}
```

Run: `go test ./internal/features/workers/transcription/...`
Expected: FAIL (package doesn't exist).

**Step 2: Implementar `deps.go`**

```go
// Package transcription is a vertical slice that owns the full
// video transcription + embedding pipeline. It exposes HTTP routes
// to enqueue jobs and runs an in-process worker pool that polls the
// Supabase `jobs` table and processes lessons end-to-end.
//
// Pipeline per lesson:
//   1. Resolve Bunny playback URL (HLS) from lesson.mediaUrl + tenant creds
//   2. Extract audio to MP3 16kHz mono (ffmpeg)
//   3. Slice audio in 10-min windows if total > Whisper 25MB limit
//   4. Transcribe each window via OpenAI Whisper API
//   5. Chunk transcript (~500 tokens, 50 overlap, aligned to segments)
//   6. Embed chunks via OpenAI text-embedding-3-small (batched)
//   7. UPSERT video + INSERT transcript + INSERT chunks (single tx, Supabase)
//   8. UPDATE lesson.transcriptionCompleted=true (memberclass DB)
//
// See CLAUDE.md ("Architecture migration in progress") for VSA rules.
package transcription

import (
    "context"
    "database/sql"
    "fmt"
    "net/http"
    "os"
    "os/exec"
    "strconv"
    "sync"
    "time"

    "github.com/memberclass-backend-golang/internal/domain/ports"
    bunnyport "github.com/memberclass-backend-golang/internal/domain/ports/bunny"
)

// Tunables (package-level so tests can read; move to Feature if per-tenant).
const (
    defaultPollInterval   = 30 * time.Second
    defaultWorkerWorkers  = 2
    orphanResetInterval   = 5 * time.Minute
    orphanStaleThreshold  = 30 * time.Minute
    cronSchedule          = "0 0 22 * * *"
    whisperMaxAudioBytes  = 24 * 1024 * 1024 // 1MB safety vs 25MB OpenAI limit
    embedBatchSize        = 96               // OpenAI hard cap is 2048; 96 keeps payload <1MB
)

type Feature struct {
    transcriptionDB *sql.DB // Supabase pgvector
    memberclassDB   *sql.DB // for lesson.transcriptionCompleted UPDATE + tenant lookup
    log             ports.Logger
    bunny           bunnyport.BunnyService
    openaiAPIKey    string
    httpClient      *http.Client

    pollInterval time.Duration
    workers      int

    mu      sync.Mutex
    running bool
    cancel  context.CancelFunc
    done    chan struct{}
}

func New(
    transcriptionDB *sql.DB,
    memberclassDB *sql.DB,
    log ports.Logger,
    bunny bunnyport.BunnyService,
) *Feature {
    // Fail-fast: ffmpeg must be on PATH.
    if _, err := exec.LookPath("ffmpeg"); err != nil {
        log.Error("transcription: ffmpeg not found in PATH — slice will refuse to run jobs", "error", err.Error())
    }

    poll := defaultPollInterval
    if v := os.Getenv("TRANSCRIPTION_POLL_INTERVAL_SECONDS"); v != "" {
        if s, err := strconv.Atoi(v); err == nil && s > 0 {
            poll = time.Duration(s) * time.Second
        }
    }
    workers := defaultWorkerWorkers
    if v := os.Getenv("TRANSCRIPTION_WORKER_CONCURRENCY"); v != "" {
        if n, err := strconv.Atoi(v); err == nil && n > 0 {
            workers = n
        }
    }

    apiKey := os.Getenv("OPENAI_API_KEY")
    if apiKey == "" {
        log.Warn("transcription: OPENAI_API_KEY not set — slice will refuse to run jobs")
    }

    return &Feature{
        transcriptionDB: transcriptionDB,
        memberclassDB:   memberclassDB,
        log:             log,
        bunny:           bunny,
        openaiAPIKey:    apiKey,
        httpClient:      &http.Client{Timeout: 5 * time.Minute},
        pollInterval:    poll,
        workers:         workers,
    }
}

// MiddlewareSet carries the chi middlewares the slice's routes need.
type MiddlewareSet struct {
    AuthInternal    func(http.Handler) http.Handler // x-internal-api-key for AI internal routes
    RateLimitTenant func(http.Handler) http.Handler
}

func (f *Feature) preflight() error {
    if f.transcriptionDB == nil {
        return fmt.Errorf("transcription DB not configured (DB_TRANSCRIPTION_DSN)")
    }
    if f.openaiAPIKey == "" {
        return fmt.Errorf("OPENAI_API_KEY not configured")
    }
    return nil
}
```

**Step 3: Run test**

```bash
go build ./internal/features/workers/transcription/...
```
Expected: PASS.

**Step 4: Commit**

```bash
git add internal/features/workers/transcription/deps.go internal/features/workers/transcription/deps_test.go
git commit -m "feat(transcription): scaffold VSA slice (Feature + deps)"
```

---

### Task 4: SQL constants

**Files:**
- Create: `internal/features/workers/transcription/sql.go`

**Step 1: Implementação direta (não precisa test — só constants)**

```go
package transcription

// ---------------- Supabase (transcription bucket) ----------------

const sqlClaimJobs = `
    UPDATE jobs
       SET status     = 'RUNNING',
           started_at = now(),
           attempts   = attempts + 1,
           updated_at = now()
     WHERE id IN (
        SELECT id FROM jobs
         WHERE status = 'PENDING'
           AND type = 'TRANSCRIPTION'
           AND attempts < max_attempts
         ORDER BY priority DESC, created_at ASC
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

const sqlMarkJobFailed = `
    UPDATE jobs
       SET status     = CASE WHEN attempts >= max_attempts THEN 'FAILED' ELSE 'PENDING' END,
           failed_at  = CASE WHEN attempts >= max_attempts THEN now() ELSE failed_at END,
           error      = $2,
           updated_at = now()
     WHERE id = $1
    RETURNING status
`

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

// idempotent video upsert keyed on (tenant_id, source_url)
const sqlUpsertVideo = `
    INSERT INTO videos (
        id, tenant_id, course_id, lesson_id, title, source_type, source_url,
        status, duration, metadata, created_at, updated_at
    ) VALUES (
        $1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, now(), now()
    )
    ON CONFLICT (tenant_id, source_url)
    DO UPDATE SET
        status     = EXCLUDED.status,
        updated_at = now(),
        lesson_id  = EXCLUDED.lesson_id
    RETURNING id, status
`

const sqlUpdateVideoStatus = `
    UPDATE videos
       SET status       = $2,
           updated_at   = now(),
           processed_at = CASE WHEN $2 = 'COMPLETED' THEN now() ELSE processed_at END,
           error        = $3
     WHERE id = $1
`

// remove prior chunks/transcripts when reprocessing
const sqlDeleteChunksByVideo     = `DELETE FROM chunks WHERE video_id = $1`
const sqlDeleteTranscriptsByVideo = `DELETE FROM transcripts WHERE video_id = $1`

const sqlInsertTranscript = `
    INSERT INTO transcripts (
        id, video_id, tenant_id, lesson_id, text, language, model, confidence,
        segments, processing_time, metadata, created_at, updated_at
    ) VALUES (
        $1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11::jsonb, now(), now()
    )
`

// Chunks are inserted via pq.CopyIn for batch performance.
// Columns order MUST match: id, video_id, transcript_id, tenant_id, course_id,
// lesson_id, text, "order", start_time, end_time, embedding, embedding_model,
// provider, metadata, created_at, updated_at
const chunksTable    = `chunks`
var   chunksColumns  = []string{
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

// ---------------- memberclass DB ----------------

const sqlSelectTenantBunnyCreds = `
    SELECT id, name, "aiEnabled", "bunnyLibraryId", "bunnyLibraryApiKey"
      FROM "Tenant"
     WHERE id = $1
`

const sqlSelectUnprocessedLessons = `
    SELECT l.id, l.name, l.slug, l."mediaUrl", l."tenantId",
           m.id AS module_id, m.name AS module_name,
           s.id AS section_id, s.name AS section_name,
           c.id AS course_id, c.name AS course_name
      FROM "Lesson" l
      JOIN "Module"  m ON m.id = l."moduleId"
      JOIN "Section" s ON s.id = m."sectionId"
      JOIN "Course"  c ON c.id = s."courseId"
     WHERE l."tenantId" = $1
       AND COALESCE(l."transcriptionCompleted", false) = false
       AND l."mediaUrl" IS NOT NULL
`

const sqlMarkLessonTranscribed = `
    UPDATE "Lesson"
       SET "transcriptionCompleted" = true,
           "updatedAt"              = now()
     WHERE id = $1
`

const sqlSelectAITenants = `
    SELECT id, name, "bunnyLibraryId", "bunnyLibraryApiKey"
      FROM "Tenant"
     WHERE "aiEnabled" = true
`
```

**Note:** colunas exatas (`mediaUrl`, `transcriptionCompleted`, `aiEnabled`, `bunnyLibraryId`, `bunnyLibraryApiKey`, `Lesson`/`Module`/`Section`/`Course` casing) **verificar** abrindo `internal/infrastructure/adapters/repository/lesson/lesson_repository.go` antes de implementar — usar os identifiers exatos lá.

**Step 2: Compile**

```bash
go build ./internal/features/workers/transcription/...
```
Expected: PASS.

**Step 3: Commit**

```bash
git add internal/features/workers/transcription/sql.go
git commit -m "feat(transcription): SQL constants for Supabase + memberclass"
```

---

### Task 5: OpenAI client (Whisper + embeddings)

**Files:**
- Create: `internal/features/workers/transcription/openai.go`
- Create: `internal/features/workers/transcription/openai_test.go`

**Step 1: Test com httptest**

```go
package transcription

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
)

func TestEmbedBatch_HappyPath(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/v1/embeddings" {
            t.Fatalf("unexpected path: %s", r.URL.Path)
        }
        if r.Header.Get("Authorization") != "Bearer test-key" {
            t.Fatal("missing/wrong auth header")
        }
        w.Header().Set("Content-Type", "application/json")
        _ = json.NewEncoder(w).Encode(embeddingsResponse{
            Data: []embedding{
                {Index: 0, Embedding: []float32{0.1, 0.2}},
                {Index: 1, Embedding: []float32{0.3, 0.4}},
            },
            Usage: usage{PromptTokens: 10, TotalTokens: 10},
        })
    }))
    defer server.Close()

    f := &Feature{openaiAPIKey: "test-key", httpClient: server.Client()}
    f.openaiBaseURL = server.URL
    vecs, tokens, err := f.embedBatch(t.Context(), []string{"hello", "world"})
    if err != nil {
        t.Fatal(err)
    }
    if len(vecs) != 2 || tokens != 10 {
        t.Fatalf("vecs=%d tokens=%d", len(vecs), tokens)
    }
}

func TestTranscribe_HappyPath(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/v1/audio/transcriptions" {
            t.Fatalf("path: %s", r.URL.Path)
        }
        if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
            t.Fatal("expected multipart")
        }
        w.Header().Set("Content-Type", "application/json")
        _ = json.NewEncoder(w).Encode(whisperResponse{
            Text:     "oi mundo",
            Language: "pt",
            Duration: 5.0,
            Segments: []whisperSegment{
                {Start: 0, End: 2.5, Text: "oi"},
                {Start: 2.5, End: 5, Text: "mundo"},
            },
        })
    }))
    defer server.Close()

    f := &Feature{openaiAPIKey: "test-key", httpClient: server.Client()}
    f.openaiBaseURL = server.URL
    resp, err := f.transcribeAudio(t.Context(), strings.NewReader("FAKE-MP3"), "audio.mp3")
    if err != nil {
        t.Fatal(err)
    }
    if resp.Text != "oi mundo" || len(resp.Segments) != 2 {
        t.Fatalf("got %+v", resp)
    }
}
```

Run: `go test ./internal/features/workers/transcription/ -run TestEmbedBatch_HappyPath -v`
Expected: FAIL (file missing).

**Step 2: Implementação**

```go
package transcription

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "mime/multipart"
    "net/http"
)

// Inject base URL so httptest can override.
const defaultOpenAIBase = "https://api.openai.com"

// add to Feature struct: openaiBaseURL string. In New() default to defaultOpenAIBase.

type embedding struct {
    Index     int       `json:"index"`
    Embedding []float32 `json:"embedding"`
}
type usage struct {
    PromptTokens     int `json:"prompt_tokens"`
    CompletionTokens int `json:"completion_tokens"`
    TotalTokens      int `json:"total_tokens"`
}
type embeddingsResponse struct {
    Data  []embedding `json:"data"`
    Usage usage       `json:"usage"`
    Model string      `json:"model"`
}

type whisperSegment struct {
    Start float64 `json:"start"`
    End   float64 `json:"end"`
    Text  string  `json:"text"`
}
type whisperResponse struct {
    Text     string           `json:"text"`
    Language string           `json:"language"`
    Duration float64          `json:"duration"`
    Segments []whisperSegment `json:"segments"`
}

const (
    embedModel   = "text-embedding-3-small"
    whisperModel = "whisper-1"
)

func (f *Feature) embedBatch(ctx context.Context, inputs []string) ([][]float32, int, error) {
    body, _ := json.Marshal(map[string]any{
        "model": embedModel,
        "input": inputs,
    })
    req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
        f.openaiBaseURL+"/v1/embeddings", bytes.NewReader(body))
    req.Header.Set("Authorization", "Bearer "+f.openaiAPIKey)
    req.Header.Set("Content-Type", "application/json")

    resp, err := f.httpClient.Do(req)
    if err != nil {
        return nil, 0, fmt.Errorf("openai embed http: %w", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode >= 400 {
        b, _ := io.ReadAll(resp.Body)
        return nil, 0, fmt.Errorf("openai embed status=%d body=%s", resp.StatusCode, string(b))
    }
    var parsed embeddingsResponse
    if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
        return nil, 0, fmt.Errorf("openai embed decode: %w", err)
    }
    vecs := make([][]float32, len(parsed.Data))
    for _, d := range parsed.Data {
        if d.Index < 0 || d.Index >= len(vecs) {
            return nil, 0, fmt.Errorf("openai embed: bad index %d", d.Index)
        }
        vecs[d.Index] = d.Embedding
    }
    return vecs, parsed.Usage.TotalTokens, nil
}

func (f *Feature) transcribeAudio(ctx context.Context, audio io.Reader, filename string) (*whisperResponse, error) {
    var buf bytes.Buffer
    mw := multipart.NewWriter(&buf)
    fw, err := mw.CreateFormFile("file", filename)
    if err != nil {
        return nil, err
    }
    if _, err := io.Copy(fw, audio); err != nil {
        return nil, err
    }
    _ = mw.WriteField("model", whisperModel)
    _ = mw.WriteField("response_format", "verbose_json")
    _ = mw.WriteField("language", "pt")
    _ = mw.WriteField("timestamp_granularities[]", "segment")
    _ = mw.Close()

    req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
        f.openaiBaseURL+"/v1/audio/transcriptions", &buf)
    req.Header.Set("Authorization", "Bearer "+f.openaiAPIKey)
    req.Header.Set("Content-Type", mw.FormDataContentType())

    resp, err := f.httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("openai whisper http: %w", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode >= 400 {
        b, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("openai whisper status=%d body=%s", resp.StatusCode, string(b))
    }
    var parsed whisperResponse
    if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
        return nil, fmt.Errorf("openai whisper decode: %w", err)
    }
    return &parsed, nil
}
```

Atualizar `deps.go` para adicionar campo `openaiBaseURL string` e setar `defaultOpenAIBase` em `New`.

**Step 3: Rodar tests**

```bash
go test ./internal/features/workers/transcription/ -v
```
Expected: PASS.

**Step 4: Commit**

```bash
git add internal/features/workers/transcription/openai.go internal/features/workers/transcription/openai_test.go internal/features/workers/transcription/deps.go
git commit -m "feat(transcription): OpenAI Whisper + embeddings HTTP client"
```

---

### Task 6: Cost tracking (`cost.go`)

**Files:**
- Create: `internal/features/workers/transcription/cost.go`
- Create: `internal/features/workers/transcription/cost_test.go`

**Step 1: Test**

```go
package transcription

import "testing"

func TestCostCents(t *testing.T) {
    // whisper-1 = $0.006 / minute → 60s → 0.6¢ → arredonda para 1
    if got := whisperCostCents(60); got != 1 {
        t.Fatalf("whisper 60s = %d¢, want 1", got)
    }
    if got := whisperCostCents(300); got != 3 {
        t.Fatalf("whisper 300s = %d¢, want 3", got)
    }
    // text-embedding-3-small = $0.02 / 1M tokens → 1M = 200¢, 50k = 1¢
    if got := embedCostCents(1_000_000); got != 200 {
        t.Fatalf("embed 1M = %d¢, want 200", got)
    }
    if got := embedCostCents(50_000); got != 1 {
        t.Fatalf("embed 50k = %d¢, want 1", got)
    }
}
```

Expected: FAIL.

**Step 2: Implementação**

```go
package transcription

import "math"

// whisperCostCents returns cost in integer cents for whisper-1 given
// audio duration in seconds. Price: $0.006 / minute.
func whisperCostCents(durationSeconds float64) int {
    cents := (durationSeconds / 60.0) * 0.6 // 0.6¢ per minute
    return int(math.Ceil(cents))
}

// embedCostCents returns cost in cents for text-embedding-3-small.
// Price: $0.02 per 1M tokens = 2¢ per 100k tokens = 0.00002¢/token.
func embedCostCents(tokens int) int {
    cents := float64(tokens) * 0.00002
    return int(math.Ceil(cents))
}
```

**Step 3: Test pass**
```bash
go test ./internal/features/workers/transcription/ -run TestCostCents -v
```

**Step 4: Commit**

```bash
git add internal/features/workers/transcription/cost.go internal/features/workers/transcription/cost_test.go
git commit -m "feat(transcription): cost calculation for whisper-1 + embed-3-small"
```

---

### Task 7: Chunker

**Files:**
- Create: `internal/features/workers/transcription/chunker.go`
- Create: `internal/features/workers/transcription/chunker_test.go`

**Step 1: Test**

```go
package transcription

import "testing"

func TestSplitIntoChunks_Single(t *testing.T) {
    segs := []whisperSegment{
        {Start: 0, End: 5, Text: "Olá mundo. Esta é uma aula de Go."},
    }
    chunks := splitIntoChunks(segs, 500, 50)
    if len(chunks) != 1 {
        t.Fatalf("got %d chunks", len(chunks))
    }
    if chunks[0].StartTime != 0 || chunks[0].EndTime != 5 {
        t.Fatalf("bad timestamps: %+v", chunks[0])
    }
}

func TestSplitIntoChunks_MultipleWithOverlap(t *testing.T) {
    // Build 30 segs of ~50 tokens each = 1500 tokens → 3 chunks of ~500
    segs := make([]whisperSegment, 30)
    for i := range segs {
        segs[i] = whisperSegment{
            Start: float64(i * 2),
            End:   float64(i*2 + 2),
            Text:  "word " + strings.Repeat("foo ", 50),
        }
    }
    chunks := splitIntoChunks(segs, 500, 50)
    if len(chunks) < 3 {
        t.Fatalf("expected ≥3 chunks, got %d", len(chunks))
    }
    // Order monotonically increasing
    for i := 1; i < len(chunks); i++ {
        if chunks[i].Order <= chunks[i-1].Order {
            t.Fatalf("non-monotonic order at %d", i)
        }
    }
}
```

(import `strings` in test file)

Expected: FAIL.

**Step 2: Implementação**

```go
package transcription

import "strings"

type chunk struct {
    Order     int
    Text      string
    StartTime float64
    EndTime   float64
    Tokens    int
}

// splitIntoChunks groups Whisper segments into chunks of approximately
// maxTokens with `overlap` tokens of carry-over between consecutive chunks.
// Token count = word count × 1.3 (cheap approximation; replace with tiktoken-go
// if accuracy matters).
func splitIntoChunks(segments []whisperSegment, maxTokens, overlap int) []chunk {
    if len(segments) == 0 {
        return nil
    }
    var out []chunk
    var cur chunk
    cur.StartTime = segments[0].Start

    flush := func(carryFrom int) {
        if cur.Tokens == 0 {
            return
        }
        cur.Order = len(out)
        out = append(out, cur)
        cur = chunk{StartTime: segments[carryFrom].Start}
    }

    for i, seg := range segments {
        tks := approxTokens(seg.Text)
        if cur.Tokens+tks > maxTokens && cur.Tokens > 0 {
            // backtrack to find overlap start
            overlapStart := i
            taken := 0
            for j := i - 1; j >= 0 && taken < overlap; j-- {
                taken += approxTokens(segments[j].Text)
                overlapStart = j
            }
            flush(overlapStart)
            // replay overlap segs into new chunk
            for j := overlapStart; j < i; j++ {
                cur.Text += segments[j].Text + " "
                cur.Tokens += approxTokens(segments[j].Text)
                cur.EndTime = segments[j].End
            }
        }
        cur.Text += seg.Text + " "
        cur.Tokens += tks
        cur.EndTime = seg.End
    }
    if cur.Tokens > 0 {
        cur.Order = len(out)
        out = append(out, cur)
    }
    // trim trailing spaces
    for i := range out {
        out[i].Text = strings.TrimSpace(out[i].Text)
    }
    return out
}

func approxTokens(s string) int {
    // 1 word ≈ 1.3 tokens for Latin text (cl100k_base average).
    words := len(strings.Fields(s))
    return (words * 13) / 10
}
```

**Step 3: Test pass**
```bash
go test ./internal/features/workers/transcription/ -run TestSplitIntoChunks -v
```

**Step 4: Commit**

```bash
git add internal/features/workers/transcription/chunker.go internal/features/workers/transcription/chunker_test.go
git commit -m "feat(transcription): chunker with overlap + segment alignment"
```

---

### Task 8: Audio extractor (ffmpeg)

**Files:**
- Create: `internal/features/workers/transcription/audio.go`
- Create: `internal/features/workers/transcription/audio_test.go`

**Step 1: Test** (skip se ffmpeg ausente)

```go
package transcription

import (
    "os"
    "os/exec"
    "testing"
)

func TestExtractAudio_RequiresFFmpeg(t *testing.T) {
    if _, err := exec.LookPath("ffmpeg"); err != nil {
        t.Skip("ffmpeg not in PATH")
    }
    // Create a 1-second silent MP3 via ffmpeg as fixture.
    tmp, _ := os.MkdirTemp("", "tx_*")
    defer os.RemoveAll(tmp)

    src := tmp + "/silent.mp3"
    cmd := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "anullsrc=r=16000:cl=mono",
        "-t", "1", "-acodec", "libmp3lame", src)
    if err := cmd.Run(); err != nil {
        t.Fatal(err)
    }

    out, err := extractAudioMP3(t.Context(), src, tmp+"/out.mp3")
    if err != nil {
        t.Fatal(err)
    }
    info, _ := os.Stat(out)
    if info.Size() == 0 {
        t.Fatal("empty audio file")
    }
}
```

Expected: FAIL.

**Step 2: Implementação**

```go
package transcription

import (
    "context"
    "fmt"
    "os"
    "os/exec"
    "strconv"
)

// extractAudioMP3 invokes ffmpeg to read an HLS playlist / MP4 URL or file
// and produce a mono 16kHz MP3 at 64kbps in outPath. Returns outPath on success.
func extractAudioMP3(ctx context.Context, input, outPath string) (string, error) {
    cmd := exec.CommandContext(ctx, "ffmpeg",
        "-y",            // overwrite
        "-loglevel", "error",
        "-i", input,
        "-vn",           // no video
        "-ac", "1",      // mono
        "-ar", "16000",  // 16 kHz
        "-acodec", "libmp3lame",
        "-ab", "64k",
        outPath,
    )
    if out, err := cmd.CombinedOutput(); err != nil {
        return "", fmt.Errorf("ffmpeg extract: %w (%s)", err, string(out))
    }
    return outPath, nil
}

// splitAudioByDuration splits an MP3 into N parts of `segSeconds` each.
// Returns the list of generated paths (in order). The naming is
// `<base>-NNN.mp3`.
func splitAudioByDuration(ctx context.Context, src, outDir string, segSeconds int) ([]string, error) {
    pattern := outDir + "/seg-%03d.mp3"
    cmd := exec.CommandContext(ctx, "ffmpeg",
        "-y", "-loglevel", "error",
        "-i", src,
        "-f", "segment",
        "-segment_time", strconv.Itoa(segSeconds),
        "-c", "copy",
        pattern,
    )
    if out, err := cmd.CombinedOutput(); err != nil {
        return nil, fmt.Errorf("ffmpeg split: %w (%s)", err, string(out))
    }
    entries, err := os.ReadDir(outDir)
    if err != nil {
        return nil, err
    }
    var parts []string
    for _, e := range entries {
        if !e.IsDir() && len(e.Name()) >= 4 && e.Name()[:4] == "seg-" {
            parts = append(parts, outDir+"/"+e.Name())
        }
    }
    return parts, nil
}
```

**Step 3: Test pass**

```bash
go test ./internal/features/workers/transcription/ -run TestExtractAudio_RequiresFFmpeg -v
```

**Step 4: Commit**

```bash
git add internal/features/workers/transcription/audio.go internal/features/workers/transcription/audio_test.go
git commit -m "feat(transcription): ffmpeg audio extraction + duration split"
```

---

### Task 9: Bunny playback URL resolver

**Files:**
- Create: `internal/features/workers/transcription/bunny_download.go`
- Create: `internal/features/workers/transcription/bunny_download_test.go`

**Step 1: Test**

```go
package transcription

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestResolvePlaybackURL_HLS(t *testing.T) {
    s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Header.Get("AccessKey") != "tenant-key" {
            t.Fatal("missing AccessKey")
        }
        _ = json.NewEncoder(w).Encode(map[string]any{
            "guid":            "abc-123",
            "status":          4, // finished
            "videoLibraryId":  "lib-1",
            "thumbnailFileName": "thumb.jpg",
        })
    }))
    defer s.Close()

    f := &Feature{
        bunny:        nil, // we won't use Service for this; direct HTTP call
        httpClient:   s.Client(),
    }
    url, err := f.resolveBunnyHLS(t.Context(), s.URL, "lib-1", "abc-123", "tenant-key")
    if err != nil {
        t.Fatal(err)
    }
    if url == "" {
        t.Fatal("empty URL")
    }
}
```

Expected: FAIL.

**Step 2: Implementação**

Bunny não expõe a HLS URL diretamente no metadata — o pattern padrão é `https://vz-<library-hash>.b-cdn.net/<guid>/playlist.m3u8`. O hash da CDN está no metadata da library (não do video). Estratégia:

1. Tentar primeiro `https://iframe.mediadelivery.net/embed/{libraryId}/{guid}` → não serve, é HTML.
2. Usar `https://video.bunnycdn.com/library/{libraryId}/videos/{guid}` para validar que o vídeo existe + pegar `status` (4 = finished, único processável).
3. Construir HLS URL via convenção `https://vz-{libraryHash}-{region}.b-cdn.net/{guid}/playlist.m3u8`. **Problema:** o libraryHash precisa vir de algum lugar — adicionar nova coluna `bunnyCdnHostname` em `Tenant` OU env var `BUNNY_CDN_HOSTNAME_PATTERN`. **Decisão de PR:** adicionar coluna `bunnyCdnHostname text` em `Tenant` (migration nova) e a UI já popula.
4. **Alternativa mais simples:** ler `mediaUrl` atual da lesson — já vem do upload com `iframe.mediadelivery.net/embed/{libraryId}/{guid}`. Extrair guid via parse. Para o HLS, usar o **MP4 fallback** via API direta: `GET https://iframe.mediadelivery.net/{libraryId}/{guid}/play_720p.mp4` (não funciona — também HTML).
5. **Caminho consagrado:** `GET https://video.bunnycdn.com/library/{libraryId}/videos/{guid}` retorna campo `videoPlaylistUrl` se HLS estiver habilitado. Verificar nos docs Bunny.

Para v1, **adicionar coluna `Tenant.bunnyCdnHostname`** (string, hostname tipo `vz-abc-123.b-cdn.net`). Por enquanto o resolver:

```go
package transcription

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/url"
    "strings"
)

type bunnyVideoMeta struct {
    GUID           string `json:"guid"`
    Status         int    `json:"status"` // 4 = finished
    VideoLibraryID int    `json:"videoLibraryId"`
}

// resolveBunnyHLS validates the video is finished and returns the HLS URL.
// libraryBaseURL defaults to BUNNY_BASE_URL ("https://video.bunnycdn.com/library/").
// cdnHostname must come from Tenant.bunnyCdnHostname (e.g. "vz-abc.b-cdn.net").
func (f *Feature) resolveBunnyHLS(ctx context.Context, libraryBaseURL, libraryID, guid, accessKey string) (string, error) {
    metaURL := strings.TrimRight(libraryBaseURL, "/") + "/" + libraryID + "/videos/" + guid
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, metaURL, nil)
    req.Header.Set("AccessKey", accessKey)
    resp, err := f.httpClient.Do(req)
    if err != nil {
        return "", fmt.Errorf("bunny meta: %w", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != 200 {
        return "", fmt.Errorf("bunny meta status=%d", resp.StatusCode)
    }
    var meta bunnyVideoMeta
    if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
        return "", fmt.Errorf("bunny meta decode: %w", err)
    }
    if meta.Status != 4 {
        return "", fmt.Errorf("bunny video not finished (status=%d)", meta.Status)
    }
    // The HLS URL is on the tenant's CDN hostname. ffmpeg can also accept the
    // Bunny tokenized URL if Token Authentication is enabled — for now we
    // require the library to allow direct hostname access.
    return metaURL, nil // CALLER must build HLS URL via tenant.bunnyCdnHostname
}

// guidFromEmbedURL parses "https://iframe.mediadelivery.net/embed/{libraryId}/{guid}"
// returning libraryId, guid.
func guidFromEmbedURL(embedURL string) (string, string, error) {
    u, err := url.Parse(embedURL)
    if err != nil {
        return "", "", err
    }
    parts := strings.Split(strings.Trim(u.Path, "/"), "/")
    // expect ["embed", "{libraryId}", "{guid}"]
    if len(parts) < 3 || parts[0] != "embed" {
        return "", "", fmt.Errorf("not a bunny embed URL: %s", embedURL)
    }
    return parts[1], parts[2], nil
}

func buildHLSURL(cdnHostname, guid string) string {
    return fmt.Sprintf("https://%s/%s/playlist.m3u8", strings.TrimPrefix(cdnHostname, "https://"), guid)
}
```

**Adicionar migration ao DB memberclass:**

```
internal/infrastructure/adapters/database/migrations/20260513_add_tenant_bunny_cdn_hostname.sql
```

```sql
ALTER TABLE "Tenant" ADD COLUMN IF NOT EXISTS "bunnyCdnHostname" text;
```

Ajustar `sqlSelectTenantBunnyCreds` para incluir `bunnyCdnHostname`.

**Step 3: Test pass**
```bash
go test ./internal/features/workers/transcription/ -v
```

**Step 4: Commit**

```bash
git add internal/features/workers/transcription/bunny_download.go internal/features/workers/transcription/bunny_download_test.go internal/infrastructure/adapters/database/migrations/ internal/features/workers/transcription/sql.go
git commit -m "feat(transcription): resolve Bunny HLS URL + tenant.bunnyCdnHostname"
```

---

### Task 10: Pipeline orchestration

**Files:**
- Create: `internal/features/workers/transcription/pipeline.go`
- Create: `internal/features/workers/transcription/pipeline_test.go`

**Step 1: Test (com go-sqlmock + fakes)** — end-to-end pipeline mockando Bunny, OpenAI, ffmpeg.

```go
package transcription

import (
    "context"
    "encoding/json"
    "os"
    "testing"

    "github.com/DATA-DOG/go-sqlmock"
)

func TestExecutePipeline_HappyPath(t *testing.T) {
    // Setup go-sqlmock for both DBs.
    // ... mock SELECT tenant, INSERT video, INSERT transcript, INSERT chunks,
    //     UPDATE jobs, UPDATE lesson.transcriptionCompleted.
    // Replace f.transcribeAudio, f.embedBatch, extractAudioMP3 with fakes via
    // function fields in Feature (small refactor — add testHooks struct).
    // Assertion: every expected SQL fired, no leftover expectations.
    _ = sqlmock.AnyArg
    _ = context.Background
    _ = os.Open
    _ = json.Marshal
    t.Skip("write after pipeline.go exists")
}
```

(Mais elaborado depois de existir pipeline.go.)

**Step 2: Implementação**

```go
package transcription

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "time"

    "github.com/google/uuid"
    "github.com/lib/pq"
)

type jobPayload struct {
    LessonID  string `json:"lessonId"`
    TenantID  string `json:"tenantId"`
    VideoURL  string `json:"videoUrl"`  // lesson.mediaUrl (Bunny embed)
    CourseID  string `json:"courseId,omitempty"`
    Title     string `json:"title"`
}

type jobResult struct {
    VideoID      string `json:"videoId"`
    TranscriptID string `json:"transcriptId"`
    ChunksCount  int    `json:"chunksCount"`
    Duration     float64 `json:"duration"`
    CostCents    int    `json:"costCents"`
}

func (f *Feature) executeJob(ctx context.Context, jobID, tenantID string, rawPayload []byte) error {
    var p jobPayload
    if err := json.Unmarshal(rawPayload, &p); err != nil {
        return fmt.Errorf("decode payload: %w", err)
    }

    // 1. Resolve tenant Bunny credentials + cdnHostname.
    var tName, libID, libKey, cdnHost sql.NullString
    var aiEnabled sql.NullBool
    if err := f.memberclassDB.QueryRowContext(ctx, sqlSelectTenantBunnyCreds, tenantID).
        Scan(new(string), &tName, &aiEnabled, &libID, &libKey /*, &cdnHost — add when query updated */); err != nil {
        return fmt.Errorf("select tenant: %w", err)
    }
    if !aiEnabled.Bool {
        return fmt.Errorf("tenant AI not enabled")
    }
    if !libID.Valid || !libKey.Valid {
        return fmt.Errorf("tenant missing Bunny credentials")
    }

    // 2. Parse guid + build HLS URL.
    libraryID, guid, err := guidFromEmbedURL(p.VideoURL)
    if err != nil {
        return err
    }
    if _, err := f.resolveBunnyHLS(ctx, os.Getenv("BUNNY_BASE_URL"), libraryID, guid, libKey.String); err != nil {
        return err
    }
    hlsURL := buildHLSURL(cdnHost.String, guid)

    // 3. Extract audio to temp dir.
    tmp, err := os.MkdirTemp("", "tx_"+jobID+"_")
    if err != nil {
        return err
    }
    defer os.RemoveAll(tmp)
    audioPath := filepath.Join(tmp, "audio.mp3")
    if _, err := extractAudioMP3(ctx, hlsURL, audioPath); err != nil {
        return fmt.Errorf("extract audio: %w", err)
    }

    // 4. Check size; split if necessary.
    info, _ := os.Stat(audioPath)
    parts := []string{audioPath}
    if info.Size() > whisperMaxAudioBytes {
        parts, err = splitAudioByDuration(ctx, audioPath, tmp, 600)
        if err != nil {
            return err
        }
    }

    // 5. Transcribe each part; concatenate with timestamp offset.
    var allSegments []whisperSegment
    var transcriptText string
    var totalDuration float64
    var costCents int
    for i, part := range parts {
        fh, err := os.Open(part)
        if err != nil {
            return err
        }
        resp, err := f.transcribeAudio(ctx, fh, filepath.Base(part))
        fh.Close()
        if err != nil {
            return fmt.Errorf("whisper part %d: %w", i, err)
        }
        offset := totalDuration
        for _, s := range resp.Segments {
            allSegments = append(allSegments, whisperSegment{
                Start: s.Start + offset, End: s.End + offset, Text: s.Text,
            })
        }
        transcriptText += resp.Text + " "
        totalDuration += resp.Duration
        costCents += whisperCostCents(resp.Duration)
    }

    // 6. Chunk.
    chunks := splitIntoChunks(allSegments, 500, 50)
    if len(chunks) == 0 {
        return fmt.Errorf("no chunks produced")
    }

    // 7. Embed in batches.
    embeddings := make([][]float32, len(chunks))
    for i := 0; i < len(chunks); i += embedBatchSize {
        end := i + embedBatchSize
        if end > len(chunks) {
            end = len(chunks)
        }
        texts := make([]string, end-i)
        for j := range texts {
            texts[j] = chunks[i+j].Text
        }
        vecs, tokens, err := f.embedBatch(ctx, texts)
        if err != nil {
            return fmt.Errorf("embed batch %d: %w", i, err)
        }
        copy(embeddings[i:], vecs)
        costCents += embedCostCents(tokens)
    }

    // 8. Single Supabase transaction.
    tx, err := f.transcriptionDB.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    videoID := uuid.NewString()
    metadataJSON, _ := json.Marshal(map[string]any{"jobId": jobID, "embed_model": embedModel})
    if _, err := tx.ExecContext(ctx, sqlUpsertVideo,
        videoID, tenantID, p.CourseID, p.LessonID, p.Title,
        SourceTypeBunny, p.VideoURL, VideoStatusEmbedding, totalDuration, metadataJSON,
    ); err != nil {
        return fmt.Errorf("upsert video: %w", err)
    }
    // wipe prior data if reprocess
    _, _ = tx.ExecContext(ctx, sqlDeleteChunksByVideo, videoID)
    _, _ = tx.ExecContext(ctx, sqlDeleteTranscriptsByVideo, videoID)

    transcriptID := uuid.NewString()
    segmentsJSON, _ := json.Marshal(allSegments)
    metaJSON, _ := json.Marshal(map[string]any{"jobId": jobID})
    if _, err := tx.ExecContext(ctx, sqlInsertTranscript,
        transcriptID, videoID, tenantID, p.LessonID, transcriptText,
        "pt", whisperModel, nil, segmentsJSON, totalDuration, metaJSON,
    ); err != nil {
        return fmt.Errorf("insert transcript: %w", err)
    }

    // CopyIn chunks
    stmt, err := tx.PrepareContext(ctx, pq.CopyIn(chunksTable, chunksColumns...))
    if err != nil {
        return fmt.Errorf("prepare copyin: %w", err)
    }
    for i, c := range chunks {
        now := time.Now()
        embStr := pgvectorString(embeddings[i])
        if _, err := stmt.ExecContext(ctx,
            uuid.NewString(), videoID, transcriptID, tenantID, p.CourseID, p.LessonID,
            c.Text, c.Order, c.StartTime, c.EndTime, embStr, embedModel,
            "openai", "{}", now, now,
        ); err != nil {
            return fmt.Errorf("copy chunk %d: %w", i, err)
        }
    }
    if _, err := stmt.ExecContext(ctx); err != nil {
        return fmt.Errorf("flush copyin: %w", err)
    }
    if err := stmt.Close(); err != nil {
        return fmt.Errorf("close copyin: %w", err)
    }

    // Update video → COMPLETED
    if _, err := tx.ExecContext(ctx, sqlUpdateVideoStatus, videoID, VideoStatusCompleted, ""); err != nil {
        return err
    }

    // Token usage row
    tokenMeta, _ := json.Marshal(map[string]any{"chunks": len(chunks)})
    if _, err := tx.ExecContext(ctx, sqlInsertTokenUsage,
        uuid.NewString(), tenantID, p.CourseID, videoID, transcriptID,
        0, 0, 0, costCents, 0, costCents, whisperModel+"+"+embedModel, "transcribe+embed", tokenMeta,
    ); err != nil {
        return err
    }

    if err := tx.Commit(); err != nil {
        return err
    }

    // 9. Mark lesson.transcriptionCompleted in memberclass DB (after Supabase commit).
    if _, err := f.memberclassDB.ExecContext(ctx, sqlMarkLessonTranscribed, p.LessonID); err != nil {
        // log but don't fail: chunks are in. Re-sync later.
        f.log.Error("transcription: mark lesson failed", "error", err.Error(), "lessonId", p.LessonID)
    }

    // 10. Mark job completed
    result, _ := json.Marshal(jobResult{
        VideoID: videoID, TranscriptID: transcriptID,
        ChunksCount: len(chunks), Duration: totalDuration, CostCents: costCents,
    })
    if _, err := f.transcriptionDB.ExecContext(ctx, sqlMarkJobCompleted, jobID, result); err != nil {
        f.log.Error("transcription: mark job failed", "error", err.Error(), "jobId", jobID)
    }
    return nil
}

// pgvectorString encodes a []float32 as pgvector literal e.g. "[0.1,0.2,...]"
func pgvectorString(v []float32) string {
    if len(v) == 0 {
        return "[]"
    }
    b := make([]byte, 0, len(v)*8)
    b = append(b, '[')
    for i, x := range v {
        if i > 0 {
            b = append(b, ',')
        }
        b = append(b, []byte(fmt.Sprintf("%g", x))...)
    }
    b = append(b, ']')
    return string(b)
}
```

**Step 3: Tests** — go-sqlmock + fake httptest servers OpenAI. Cobertura mínima: happy-path 1 lesson + 1 chunk + 1 embed batch. Skip parts ffmpeg se ausente.

**Step 4: Commit**

```bash
git add internal/features/workers/transcription/pipeline.go internal/features/workers/transcription/pipeline_test.go
git commit -m "feat(transcription): pipeline orchestration (Bunny→Whisper→chunk→embed→pgvector)"
```

---

### Task 11: Worker + cron + claim

**Files:**
- Create: `internal/features/workers/transcription/worker.go`
- Create: `internal/features/workers/transcription/worker_claim.go`
- Create: `internal/features/workers/transcription/cron.go`
- Create: `internal/features/workers/transcription/worker_test.go`

**Step 1: Test** — claim 1 job, executa fake pipeline, marca completed; orphan reset; stop-during-tick não vaza goroutine.

**Step 2: Implementação** — espelha `notifications/worker.go` 1:1 com `executeJob` no lugar de `dispatch`. Adicionar pool de N goroutines (channel `jobChan`). `cron.go` registra-se no `Scheduler` existente (em `internal/application/jobs/scheduler.go`) via novo job que faz:
```go
// Pseudocódigo cron.go
func (f *Feature) RunCronEnqueue(ctx context.Context) error {
    // SELECT tenants WHERE aiEnabled
    // For each tenant: SELECT unprocessed lessons; INSERT INTO jobs(...) bulk.
}
```

Registrar no scheduler em `cmd/api/main.go` substituindo `transcription.NewTranscriptionJob` velho pelo wrapper que chama `transcriptionFeature.RunCronEnqueue`.

**Step 3: Tests pass + commit**

```bash
git add internal/features/workers/transcription/worker.go internal/features/workers/transcription/worker_claim.go internal/features/workers/transcription/cron.go internal/features/workers/transcription/worker_test.go
git commit -m "feat(transcription): worker pool + claim/orphan + cron enqueue"
```

---

### Task 12: HTTP routes (process-lessons, status, patch)

**Files:**
- Create: `internal/features/workers/transcription/routes.go`
- Create: `internal/features/workers/transcription/process_lessons.go`
- Create: `internal/features/workers/transcription/job_status.go`
- Create: `internal/features/workers/transcription/update_status.go`
- Test: `internal/features/workers/transcription/handlers_test.go`

**Step 1: Test handler** — `POST /tenants/process-lessons` com tenantId enfileira N jobs e retorna 202 com `{ jobIds, count }`. `GET /jobs/{id}` retorna status agregado. `PATCH /lessons/{id}/transcription` marca completed (manter compatibilidade com clientes que faziam o PATCH).

**Step 2: Implementação routes.go**

```go
package transcription

import (
    "github.com/go-chi/chi/v5"
)

// Register mounts the slice's HTTP routes on r. r is expected to already be
// scoped to `/api/v1/ai`.
func (f *Feature) Register(r chi.Router, mw MiddlewareSet) {
    r.With(mw.AuthInternal, mw.RateLimitTenant).Post("/tenants/process-lessons", f.ProcessLessonsTenant)
    r.With(mw.AuthInternal).Get("/jobs/{jobId}", f.GetJobStatus)
    r.With(mw.AuthInternal).Patch("/lessons/{lessonId}/transcription", f.UpdateLessonTranscription)
}
```

**Step 3: Compatibilidade** — preservar payloads de request/response **idênticos** aos handlers atuais (`ai_tenant_handler.go`, `ai_lesson_handler.go`) para não quebrar clientes. Copiar shape exato.

**Step 4: Commit**

```bash
git add internal/features/workers/transcription/routes.go internal/features/workers/transcription/process_lessons.go internal/features/workers/transcription/job_status.go internal/features/workers/transcription/update_status.go internal/features/workers/transcription/handlers_test.go
git commit -m "feat(transcription): HTTP routes (enqueue, job status, patch lesson)"
```

---

### Task 13: Wire na app — FX + router + scheduler + lifecycle

**Files:**
- Modify: `cmd/api/main.go` (FX provide + startApplication)
- Modify: `internal/application/router/router.go` (registrar slice no grupo `/api/v1/ai`)
- Modify: `internal/application/jobs/scheduler.go` (substituir `transcriptionJob` antigo pelo wrapper)

**Step 1: Test** — `go build ./...` deve compilar; `go vet ./...` deve passar.

**Step 2: Mudanças**

`cmd/api/main.go`:
- `fx.Provide` add: `func(dbMap database.DBMap, log ports.Logger, bunny bunny.BunnyService) *transcription.Feature { return transcription.New(dbMap["transcription"], dbMap["memberclass"], log, bunny) }`
- Remover providers do código velho (mantém esses ainda — Task 14 deleta): `transcription.NewTranscriptionJob` (caminho velho), `ai2.NewAITenantUseCase`, `ai3.NewAITenantHandler`. **Deixar por enquanto** para PR ficar focado; Task 14 limpa.

Wait — VSA proíbe adicionar ao velho. Aqui não estamos adicionando: estamos **adicionando o slice novo** e **removendo o registro do velho**. Para evitar conflito de rotas (mesma rota `/process-lessons`), Task 14 **precisa rodar junto** com Task 13. Vamos fundir 13+14.

**Step 3:** Em `startApplication` adicionar:

```go
transcriptionFeat *transcription.Feature,
// ...
txCtx, stopTx := context.WithCancel(context.Background())
defer stopTx()
transcriptionFeat.Start(txCtx)
// Cron: scheduler.AddJob(transcription.CronAdapter{Feat: transcriptionFeat}, "0 0 22 * * *")
```

**Step 4:** Router — substituir handler antigo no grupo `/api/v1/ai`:

```go
// internal/application/router/router.go (~linha 160)
router.Route("/ai", func(router chi.Router) {
    // ... rotas existentes ...
    r.transcription.Register(router, transcription.MiddlewareSet{
        AuthInternal:    InternalKeyMiddleware,
        RateLimitTenant: mw.RateLimitTenant,
    })
    // remover: router.Post("/tenants/process-lessons", r.aiTenantHandler.ProcessLessonsTenant)
})
```

**Step 5:** `go build ./... && go vet ./...` deve passar.

**Step 6: Commit**

```bash
git add cmd/api/main.go internal/application/router/router.go internal/application/jobs/scheduler.go
git commit -m "feat(transcription): wire slice into FX, router, scheduler"
```

---

### Task 14: Cleanup do código velho

**Files:**
- Delete: `internal/application/jobs/transcription/` (diretório todo)
- Delete: `internal/domain/dto/transcription_job.go`
- Delete: `internal/domain/usecases/ai/ai_tenant_usecase.go` (ProcessLessonsTenant) — **só se nenhum outro handler usa o usecase**. Verificar: `grep -r AITenantUseCase --include='*.go'`. Se `ai_tenant_handler.go` ainda usa `GetTenantsWithAIEnabled`, mover essa função para o slice ou manter usecase só com esse método.
- Delete: handler antigo `/process-lessons` em `internal/application/handlers/http/ai/ai_tenant_handler.go` (remover método + rota; preservar `GetTenantsWithAIEnabled` se ainda usado em rota separada).
- Modify: `internal/application/handlers/http/ai/ai_lesson_handler.go` — remover handler PATCH se substituído pelo slice; verificar.
- Modify: `.env.example` — remover `TRANSCRIPTION_API_URL`.
- Modify: README.md (linhas 212 + 275) — substituir doc do `TRANSCRIPTION_API_URL` por `DB_TRANSCRIPTION_DSN` + `OPENAI_API_KEY`.
- Modify: `swagger.yaml` — atualizar specs das rotas modificadas (status response shape).
- Delete: `internal/application/jobs/transcription/README.md` (substituído por package doc no slice).

**Step 1: Grep para garantir não há referência órfã**

```bash
grep -rn "TRANSCRIPTION_API_URL\|transcription/transcription_job\|domain/dto/transcription_job\|ProcessLessonsTenant\|extract-and-embed" --include="*.go" --include="*.yaml" --include="*.md" --include="*.example"
```
Expected: zero matches após deletes.

**Step 2: go build + vet + test**

```bash
go build ./... && go vet ./...
make generate-mocks   # importante: mocks órfãos precisam regenerar
go test ./...
```
Expected: ALL PASS. Se algum mock quebra, remover entrada em `.mockery.yaml`.

**Step 3: Commit**

```bash
git add -A
git commit -m "chore(transcription): delete legacy external-service code"
```

---

### Task 15: Dockerfile — ffmpeg

**Files:**
- Modify: `Dockerfile`

**Step 1: Adicionar instalação**

Procurar etapa final `FROM` (alpine ou debian-slim) e adicionar:

```dockerfile
# debian-slim
RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg && rm -rf /var/lib/apt/lists/*

# OU alpine
RUN apk add --no-cache ffmpeg
```

**Step 2: Build local + smoke**

```bash
make docker-build
docker run --rm <image> ffmpeg -version
```
Expected: ffmpeg version output.

**Step 3: Commit**

```bash
git add Dockerfile
git commit -m "build: add ffmpeg to runtime image for transcription pipeline"
```

---

### Task 16: Validação completa + smoke test

**Files:** nenhum (smoke test manual)

**Step 1: Build + lint + test + coverage**

```bash
go build ./...
go vet ./...
make generate-mocks
make test
make test-coverage
```

Expected: tudo passa; coverage ≥70% no slice novo.

**Step 2: Smoke local**

```bash
# Subir API com env do Supabase de staging + OpenAI dev key
make run

# Em outro terminal — enfileirar processamento
curl -X POST http://localhost:8181/api/v1/ai/tenants/process-lessons \
  -H "x-internal-api-key: $INTERNAL_AI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"tenantId":"tenant-de-teste"}'

# Resposta esperada: 202 com { jobIds: [...], count: N }

# Consultar job
curl http://localhost:8181/api/v1/ai/jobs/<jobId> \
  -H "x-internal-api-key: $INTERNAL_AI_API_KEY"
# Status migra PENDING → RUNNING → COMPLETED em alguns minutos
```

Verificar no Supabase:
```sql
SELECT id, status, started_at, completed_at, error FROM jobs ORDER BY created_at DESC LIMIT 5;
SELECT id, status, duration FROM videos ORDER BY created_at DESC LIMIT 5;
SELECT video_id, count(*) FROM chunks GROUP BY video_id ORDER BY 1 DESC LIMIT 5;
```

Verificar no memberclass DB:
```sql
SELECT id, name, "transcriptionCompleted" FROM "Lesson" WHERE id = '<lessonId>';
-- transcriptionCompleted = true
```

**Step 3: Salvar evidência no PR**

Screenshot do `jobs.status=COMPLETED` e da lesson com flag true. Cole no body do PR.

---

### Task 17: PR draft

**Step 1: PR body em `/tmp/.pr-body.md`**

```markdown
## O que foi feito

Substitui o serviço externo de transcrição (`ai-transcriber.9ho8ul.easypanel.host`)
por uma implementação 100% Go nativa sob a arquitetura VSA. O slice novo em
`internal/features/workers/transcription/` orquestra todo o pipeline:
Bunny HLS → ffmpeg (MP3 16kHz mono) → OpenAI Whisper → chunking →
OpenAI embeddings (text-embedding-3-small) → pgvector (Supabase dedicado).

## Decisão técnica

- Slice VSA worker (mesmo padrão do `notifications/`).
- Job-por-lesson em `public.jobs` (Supabase), worker pool de 2 goroutines com
  polling 30s + orphan reset 5min.
- Schema Supabase legado reaproveitado intacto.
- ffmpeg como dependência runtime (Dockerfile atualizado).
- Custo rastreado em `public.token_usage` por chamada.

Detalhes em `docs/plans/2026-05-13-transcription-go-vsa.md`.

## Como testar

1. Configurar env vars: `DB_TRANSCRIPTION_DSN`, `OPENAI_API_KEY`,
   `TRANSCRIPTION_WORKER_CONCURRENCY=2`, `TRANSCRIPTION_POLL_INTERVAL_SECONDS=30`.
2. Aplicar migrations:
   ```bash
   psql "$DB_TRANSCRIPTION_DSN" -f migrations/transcription/001_pgvector_index.sql
   psql "$DB_DSN" -f internal/infrastructure/adapters/database/migrations/20260513_add_tenant_bunny_cdn_hostname.sql
   ```
3. Popular `Tenant.bunnyCdnHostname` para os tenants AI-enabled.
4. `make run` + smoke test conforme Task 16.

## Checklist

- [x] Build ok (`go build ./...`)
- [x] Vet ok (`go vet ./...`)
- [x] Testes passando (`make test`)
- [x] Coverage ≥70% no slice
- [x] ffmpeg instalado no Dockerfile
- [x] PR draft

## Notas para revisor

- **Schema enum:** Task 0 confirmou valores reais — vide `sql.go`.
- **Bunny CDN hostname:** nova coluna `Tenant.bunnyCdnHostname` é necessária;
  UI precisa expor input antes de habilitar IA em novos tenants.
- **Custo:** com a config padrão (concorrência=2), em pior caso de fila de
  100 aulas de 30min = ~50 USD em Whisper + ~$0.20 em embeddings.
- **Migração de dados antigos:** se o Supabase já tinha dados do serviço Python,
  eles permanecem (não tocamos). Idempotência via UNIQUE `(tenant_id, source_url)`.
```

**Step 2: Criar PR**

```bash
git push -u origin feat/transcription-go-vsa
gh pr create --title "feat: transcription pipeline 100% Go (VSA)" --body "$(cat /tmp/.pr-body.md)" --draft
```

Expected: URL do PR no stdout.

---

## Comandos de validação (recapitulação)

Após cada task:
```bash
go build ./...
go vet ./...
go test ./internal/features/workers/transcription/...
```

Antes do PR final:
```bash
go build ./...
go vet ./...
make generate-mocks
make test
make test-coverage   # confirmar ≥70% no slice novo
make docker-build && docker run --rm <image> ffmpeg -version
```

---

## Resumo de arquivos

### Criar (slice + migrations)
- `internal/features/workers/transcription/deps.go`
- `internal/features/workers/transcription/routes.go`
- `internal/features/workers/transcription/process_lessons.go`
- `internal/features/workers/transcription/job_status.go`
- `internal/features/workers/transcription/update_status.go`
- `internal/features/workers/transcription/worker.go`
- `internal/features/workers/transcription/worker_claim.go`
- `internal/features/workers/transcription/pipeline.go`
- `internal/features/workers/transcription/bunny_download.go`
- `internal/features/workers/transcription/audio.go`
- `internal/features/workers/transcription/openai.go`
- `internal/features/workers/transcription/chunker.go`
- `internal/features/workers/transcription/cron.go`
- `internal/features/workers/transcription/cost.go`
- `internal/features/workers/transcription/sql.go`
- `internal/features/workers/transcription/*_test.go` (5–7 arquivos)
- `migrations/transcription/001_pgvector_index.sql`
- `internal/infrastructure/adapters/database/migrations/20260513_add_tenant_bunny_cdn_hostname.sql`

### Modificar
- `internal/infrastructure/adapters/database/multi_db.go` (bucket transcription)
- `cmd/api/main.go` (FX provide + startApplication wire)
- `internal/application/router/router.go` (slice register, rota velha removida)
- `internal/application/jobs/scheduler.go` (cron adapter novo)
- `.env.example` (DB_TRANSCRIPTION_DSN, OPENAI_API_KEY, TRANSCRIPTION_WORKER_CONCURRENCY, TRANSCRIPTION_POLL_INTERVAL_SECONDS; remover TRANSCRIPTION_API_URL)
- `Dockerfile` (instalar ffmpeg)
- `README.md` (linhas 212, 275)
- `swagger.yaml` (rotas de transcrição)

### Deletar
- `internal/application/jobs/transcription/` (dir)
- `internal/domain/dto/transcription_job.go`
- `internal/domain/usecases/ai/ai_tenant_usecase.go` (após verificar não-uso)
- Métodos de `ai_tenant_handler.go` referentes a `/process-lessons`
- `internal/application/jobs/transcription/README.md`
- Entradas em `.mockery.yaml` referentes a `AITenantUseCase` (se removido)

---

## Execution Handoff

Plan complete and saved to `docs/plans/2026-05-13-transcription-go-vsa.md`. Two execution options:

**1. Subagent-Driven (this session)** — I dispatch fresh subagent per task, review between tasks, fast iteration

**2. Parallel Session (separate)** — Open new session with `superpowers:executing-plans`, batch execution with checkpoints

**Which approach?**
