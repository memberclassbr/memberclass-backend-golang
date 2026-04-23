# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 🚧 Architecture migration in progress — Vertical Slice Architecture

**Status (2026-04-23):** this repo is migrating from Clean Architecture + Uber FX to **Vertical Slice Architecture** (VSA). The goal is simplicity: one folder per feature where a developer can read an endpoint top-to-bottom in a single file (`parse → rules → SQL`) without hopping through handler → usecase → port → repository.

### Target structure

```
internal/features/
  <feature_name>/
    deps.go           # Feature struct + New(); shared deps for the slice
    routes.go         # Register(r chi.Router, mw MiddlewareSet)
    <action>.go       # handler + business rule + SQL in one file
    <action>_test.go  # go-sqlmock + local fakes (no mockery)
  shared/             # cross-slice helpers introduced on-demand
```

### Rules for new work during the migration

- **New features MUST be created under `internal/features/<name>/`**, never in the old layered structure.
- **Do NOT add to `internal/domain/ports/`, `internal/domain/usecases/`, `internal/domain/dto/`, `internal/application/handlers/http/`, or `internal/infrastructure/adapters/repository/`.** Those directories are being drained, not extended.
- **Do not add to `.mockery.yaml`.** Slices test against `go-sqlmock` + local fakes defined in the test file — no generated mocks.
- Slice files follow the 3-section layout: handler (HTTP parsing + response) at the top, business rule in the middle, SQL constants at the bottom. Everything in the same file when possible; split per-action if needed.
- Slices use `*sql.DB` directly. No repository port layer. If two slices share SQL, extract to `internal/features/shared/` only when the duplication actually appears.
- Slices import `ports.Logger`, `ports.Cache`, `constants.TenantContextKey`, `memberclasserrors.MemberClassError`, and `dto.PaginationMeta` from their **current locations** for now. Those will migrate into `features/shared/` in the cleanup PR at the end.
- Register each slice in [cmd/api/main.go](cmd/api/main.go) via `fx.Provide(<slice>.New)` and in [internal/application/router/router.go](internal/application/router/router.go) via `feature.Register(r, mw)`. FX stays until the last slice migrates.

### Migration progress

- [x] Pilot: `activity_summary`
- [x] `user_activities` — unified events timeline (login, lessonCompleted, download, comment, acceptTerms, quiz, certificate); fixed tenant leak in SQL + cache key
- [ ] Everything else (13 features remaining): auth, sso, user_informations, user_purchase, comment, social_comment, vitrine, student_report, ai_lesson, ai_tenant, video, bunny, lesson (+ pdf_processor + transcription job)
- [ ] Cleanup PR: delete `internal/domain/*`, `internal/application/handlers|middlewares|jobs|router`, `internal/infrastructure/adapters`, `internal/mocks/`, `.mockery.yaml`; remove `go.uber.org/fx` from `go.mod`.

When touching an existing old-structure feature for a bugfix, leave the structure alone — don't mix a refactor into an unrelated fix. Schedule the slice migration as its own PR.

## Common commands

Local dev (Go 1.25.1, requires `.env` at repo root):

- `make run` — start the API (`go run ./cmd/api`)
- `make build` — compile to `bin/main`
- `make generate-mocks` — regenerate mocks under `internal/mocks/` from `.mockery.yaml`. **Mocks are not checked in — you must run this before `go test` or most test files will not compile.**
- `make test` / `go test -v ./...` — run all tests
- `make test-coverage` — coverage to `coverage.out` + HTML report
- `make dev-setup` — installs mockery (`~/go/bin/mockery`) and generates mocks
- `make ci` — what CI runs: mocks + tests
- Single package: `go test ./internal/domain/usecases/auth/...`
- Single test: `go test ./internal/domain/usecases/auth/ -run TestAuthUseCase_GenerateMagicLink`

Docker: `make docker-build && make docker-run`.

## Environment

`.env.example` is the reference. Required for local run: `DB_DRIVER=postgres`, `DB_DSN`, plus optionally `DB_EPHRA_DSN` and `DB_CELETUS_DSN` (see multi-DB below). Redis is via Upstash (`UPSTASH_REDIS_URL`, `UPSTASH_REDIS_TOKEN`). Default port is `8181`.

## Architecture

Clean Architecture with Uber FX dependency injection. Composition root is [cmd/api/main.go](cmd/api/main.go) — every constructor is registered in one `fx.Provide` block and `startApplication` wires the HTTP server + cron scheduler. When adding a new repository / use case / handler, register its constructor in this list or FX will panic at startup with a missing-dependency error.

Layer boundaries (enforced by imports, not tooling):

- `internal/domain/` — entities, DTOs (`dto/request`, `dto/response`), use cases, and **interfaces live in `domain/ports/`** grouped by feature (`ports/auth`, `ports/lesson`, etc.). Use cases depend only on ports, never on concrete adapters.
- `internal/application/` — HTTP handlers (`handlers/http/<feature>/`), middlewares (`middlewares/auth`, `middlewares/rate_limit`), chi router wiring in [internal/application/router/router.go](internal/application/router/router.go), and cron jobs in `application/jobs/`.
- `internal/infrastructure/adapters/` — concrete implementations of ports: `repository/` (PostgreSQL via `database/sql` + `lib/pq`), `cache/` (Redis via `go-redis`), `storage/` (AWS SDK v2 against DigitalOcean Spaces), `external_services/` (Bunny CDN, iLovePDF), `logger/` (slog JSON), `rate_limiter/` (Redis-backed).
- `internal/mocks/` — generated, do not edit.

### Multi-database routing

This is the non-obvious part. The app can talk to **multiple PostgreSQL databases simultaneously** — one per "bucket". See [internal/infrastructure/adapters/database/multi_db.go](internal/infrastructure/adapters/database/multi_db.go):

- Buckets and their env vars are hard-coded in `bucketDSNMapping`: `memberclass` → `DB_DSN`, `ephra` → `DB_EPHRA_DSN`, `celetusclass` → `DB_CELETUS_DSN`.
- `NewMultiDB` opens one `*sql.DB` per configured DSN (missing env vars are skipped with a warning). At least one must be set.
- `DefaultDB` returns the `memberclass` connection for repositories that don't need multi-bucket routing.
- For multi-bucket aware repos, use a **resolver** pattern: e.g. [internal/infrastructure/adapters/repository/lesson/lesson_repo_resolver.go](internal/infrastructure/adapters/repository/lesson/lesson_repo_resolver.go) builds one `LessonRepository` per bucket and exposes `Resolve(bucket)`, `All()`, `Default()`, and `FindByLessonID(ctx, id)` (which probes all buckets). Add new multi-DB repositories by following this pattern and registering both the resolver and the default in FX.

### Rate limiting

Three Redis-backed limiters injected separately (`RateLimiterTenant`, `RateLimiterIP`, `RateLimiterUpload`) wrapped by three middlewares (`rate_limit.LimitByTenant`, `LimitByIP`, the upload variant). Routes compose them via `chi.With(...)` in [router.go](internal/application/router/router.go) — follow the existing patterns when adding endpoints.

### Auth

Two middlewares in [internal/application/middlewares/auth/](internal/application/middlewares/auth/):

- `AuthMiddleware` — tenant API key via header `mc-api-key`.
- `AuthExternalMiddleware` — used on `/auth` and `/sso/validate-token`; validates a tenant-facing external API key.

Internal AI endpoints (`/api/v1/ai/*`) validate `x-internal-api-key` against `INTERNAL_AI_API_KEY` inside their handlers (not a middleware).

### Scheduled jobs

[internal/application/jobs/scheduler.go](internal/application/jobs/scheduler.go) uses `robfig/cron/v3` with seconds precision. `InitJobs` in the same file is where new `ports.Job` implementations get registered. Current: transcription job at `0 0 22 * * *` daily. The scheduler is started and gracefully stopped by `startApplication`.

## Conventions

- File naming: `{resource}_handler.go`, `{resource}_usecase.go`, `{resource}_repository.go`, DTOs as `{action}_{resource}_{request|response}.go`. One handler per file.
- Errors: domain-specific errors live in `internal/domain/memberclasserrors/` as typed `MemberClassError{Code, Message}`. Handlers translate to HTTP via [internal/application/handlers/http/response.go](internal/application/handlers/http/response.go).
- Mocks: add new interfaces under `internal/domain/ports/<feature>/` and register them in [.mockery.yaml](.mockery.yaml), then `make generate-mocks`.
- CI ([.github/workflows/](.github/workflows/)) enforces `go build ./...`, mockery regen, `go test`, and a **70% coverage threshold** over a curated package list (see the `coverage` job). Tests failing to compile because of missing mocks is the usual symptom of skipping `make generate-mocks`.
- Swagger spec is hand-maintained in [swagger.yaml](swagger.yaml) and served at `/docs/`. The binary copies it into the container image via the Dockerfile.
