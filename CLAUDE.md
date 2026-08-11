# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Architecture — Vertical Slice Architecture

One folder per feature. An endpoint reads top-to-bottom in a single file
(`parse → rules → SQL`) without hopping through handler → usecase → port →
repository. The migration away from Clean Architecture + Uber FX is **complete**:
there is no `internal/domain`, no `internal/application`, no `internal/mocks`,
and no DI framework.

```
cmd/
  api/            the service (main.go is ~30 lines)
  analytics/      one-off CLI for rollups and backfills
internal/
  app/            composition root: app.go wires everything, router.go mounts it
  features/
    api/          tenant-facing endpoints
    admin/        internal endpoints, guarded by x-internal-api-key or a Bearer JWT
    workers/      background pipelines
  platform/       things that talk to the outside world
  shared/         rules and types more than one slice needs
```

### Slice layout

```
internal/features/<group>/<name>/
  deps.go          Feature struct + New(); the slice's dependencies
  routes.go        Register(r chi.Router, mw MiddlewareSet)
  <action>.go      handler + business rule + SQL, in that order, one file per action
  <action>_test.go go-sqlmock + local fakes defined in the test file
```

Each action file carries three sections in order:

1. **HTTP handler** — parse the request, resolve the tenant, map errors to status codes.
2. **Business rule** — the decision the endpoint exists to make.
3. **SQL** — query constants at the bottom, then the functions that run them.

Slices take `*sql.DB` directly. There is no repository layer and no port
interface between a slice and its queries. If two slices need the same query,
duplicate it until the duplication actually hurts, then extract to
`internal/shared/`.

### platform vs shared

`internal/platform` is code that talks to something outside the process:
`config`, `logger`, `cache` (Redis), `database`, `storage` (Spaces),
`ratelimit`, `bunny`, `ilovepdf`, `resend`. Each package declares its own
contract next to its implementation — there is no separate ports package.

`internal/shared` is code several slices need but nothing outside the process
cares about: `tenant` (the request's tenant + context key), `session` (the
NextAuth payload), `middleware`, `httpx`, `memberclasserrors`, `pagination`,
`constants`, `utils`.

### Rules for new work

- New features go under `internal/features/<group>/<name>/`.
- Register the slice in [internal/app/app.go](internal/app/app.go) and mount its
  routes in [internal/app/router.go](internal/app/router.go).
- No mock generation. Tests use `go-sqlmock` plus fakes written in the test file.
- Read config through `*config.Config`. Do not call `os.Getenv` outside
  [internal/platform/config](internal/platform/config).
- A new required environment variable goes in `config.Load`'s required set, so a
  deployment missing it fails to start instead of misbehaving.

## Common commands

Go 1.25.1, `.env` at the repo root.

- `make run` — start the API (`go run ./cmd/api`)
- `make build` — compile to `bin/main`
- `make test` / `go test ./...` — run all tests (nothing to generate first)
- `make test-coverage` — coverage to `coverage.out` + HTML report
- `make ci` — what CI runs: build + tests
- `make smoke` — hit every endpoint of a running deployment, see below
- Single package: `go test ./internal/features/api/vitrine/...`
- Single test: `go test ./internal/features/api/auth/ -run TestGenerateMagicLink_UsesCustomDomainWhenSet`

Docker: `make docker-build && make docker-run`.

## Deployment model

**One deployment per customer**, each with its own tenant database, its own
pgvector database and its own Spaces bucket. The service used to route between
several tenant databases at request time; it no longer does, and there is
exactly one `DB_DSN`.

Requests are still scoped by `tenantId` in SQL. A dedicated database is
isolation, not a reason to drop the filter — if a deployment is ever pointed at
the wrong database, the filter is what stops one customer reading another's
data.

### Schema ownership

- **Tenant database** — owned by Prisma in the sibling `mult-memberclass`
  repository. This service only reads and writes it; it never migrates it.
- **Transcription database** — owned here. The SQL files in
  [internal/platform/database/migrations/transcription](internal/platform/database/migrations/transcription)
  are embedded with `go:embed` and applied at boot, tracked in a
  `schema_migrations_go` table. A fresh deployment needs no manual step.

## Environment

[.env.example](.env.example) is the reference and is split into required and
optional blocks.

Missing **required** variables abort startup with every offender named at once.
Missing **optional** variables disable one feature and log a warning naming the
variable — check the boot log of a new deployment before assuming a feature is
on.

Default port is `8181`.

## Authentication

Four credentials, each guarding a different surface:

| Credential | Header / cookie | Guards |
|---|---|---|
| Tenant external API key | `mc-api-key` | most of `/api/v1/*` |
| Internal API key | `x-internal-api-key` | `/api/v1/ai/*`, `/api/v1/sso/generate-token`, `/api/lessons/*` |
| NextAuth session | `next-auth.session-token` cookie | `/api/comments` |
| NextAuth Bearer JWT | `Authorization: Bearer` | `/imports/*` |

The middlewares live in [internal/shared/middleware](internal/shared/middleware).
The internal API key is checked inside the handlers that use it, not by a
middleware; those checks reject an empty incoming key so an unset
`INTERNAL_AI_API_KEY` cannot leave an endpoint open.

**`POST /api/v1/videos/upload` has no credential check** — only the upload rate
limiter. That predates this structure and was left as it was; adding auth would
break the frontend that calls it.

## Rate limiting

Three Redis-backed limiters in [internal/platform/ratelimit](internal/platform/ratelimit)
(tenant, IP, upload bytes), wrapped by middlewares. Routes compose them through
each slice's `MiddlewareSet` — the router owns construction, slices only
declare what they need.

## Background work

Started by `App.Run` and stopped in reverse order on shutdown, before the
database closes:

- **analytics** — daily and monthly rollups on a cron (`robfig/cron` with seconds).
- **notifications** — polls the Notification table, dispatches FCM pushes.
- **transcription** — polls its own jobs table; video → Whisper → chunk → embed → pgvector.
- **member_import** — startup reset for orphaned imports, plus a 24h retention job.

## Conventions

- Errors: `internal/shared/memberclasserrors` holds typed
  `MemberClassError{Code, Message}`; each slice maps them to its own error codes.
- Error bodies come in two shapes and both are contract: `{error, message}` for
  405 and unmapped codes, `{ok, error, errorCode}` for the failures clients
  switch on. Check what an endpoint already returns before changing it.
- Swagger is hand-maintained in [swagger.yaml](swagger.yaml), served at `/docs/`,
  and copied into the image by the Dockerfile.
- CI ([.github/workflows/](.github/workflows/)) runs `go build ./...`,
  `go test ./...` and a coverage threshold over
  `internal/features`, `internal/platform`, `internal/shared` and `internal/app`.
  The threshold is temporarily **50%** while the slices rebuild the coverage the
  deleted mock-based suite used to provide; raise it back toward 70% as they do.

## Smoke testing a deployment

`scripts/smoke.sh` hits every route and reports the status of each. It checks
that routes are mounted and that each enforces its credential; it does **not**
compare response bodies against a baseline, so payload shapes are still a
manual pass.

```
BASE_URL=https://api.example.com \
MC_API_KEY=... INTERNAL_API_KEY=... BEARER_TOKEN=... \
TENANT_ID=... EMAIL=... \
./scripts/smoke.sh
```

Every variable is optional; requests whose credential is missing are skipped.
