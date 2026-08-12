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
cares about: `tenant` (the request's tenant + context key), `middleware`,
`tenantrole`, `magiclink`, `httpx`, `memberclasserrors`, `pagination`,
`datefilter`, `constants`, `utils`.

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

Three credentials, each guarding a different surface, and **all three travel in
a header**:

| Credential | Header | Guards |
|---|---|---|
| Tenant external API key | `x-api-key` (legacy: `mc-api-key`) | most of `/api/v1/*` |
| Internal API key | `x-internal-api-key` | `/api/v1/ai/*`, `/api/lessons/*` |
| go-token Bearer JWT | `Authorization: Bearer` | `/imports/*`, `/sso/*`, `/videos/*` |

The middlewares live in [internal/shared/middleware](internal/shared/middleware).

There used to be a fourth: the NextAuth session cookie, on `GET /api/comments`.
Both are gone. That route was a second mount of the `/api/v1/comments` listing
with no callers left, and being cookie-authenticated it was the only thing here
reachable with a credential a browser attaches by itself — which is what forced
the CORS policy to allow credentials. Removing it is what lets that policy be a
literal `*`. **No ambient credential reaches this service any more, so CSRF has
nothing to work with.**

The tenant key moved from `mc-api-key` to `x-api-key`. Both are accepted and
`x-api-key` wins when a caller sends both; only `x-api-key` is documented in
[swagger.yaml](swagger.yaml). The legacy name goes when its callers do —
dropping it earlier would log out every integration in one deploy.

The internal API key is checked inside the handlers that use it, not by a
middleware; those checks reject an empty incoming key so an unset
`INTERNAL_AI_API_KEY` cannot leave an endpoint open.

### The frontend-origin surface: `/imports`, `/sso`, `/videos`

These three sit at the root, without the `/api` prefix, because the same thing
is true of all of them: they are called by the Next.js admin frontend with a
short-lived Bearer JWT minted at `/api/auth/go-token?tenantId=X` on the Next
side, using `GO_API_JWT_SECRET`.

**The token is scoped to one tenant.** Before minting, the frontend checks that
the session's user holds a row in `"UsersOnTenants"` for X and refuses with 403
if not; the token then carries `tenantId` and the `role` read from that row:

```json
{ "sub": "...", "email": "...", "tenantId": "...", "role": "...",
  "aud": "memberclass-go-api", "jti": "...", "iat": 0, "exp": 0 }
```

`tenantId` is where every one of these handlers gets its tenant, and the only
place. The request bodies still accept the field, but it is *checked against the
claim* and never read as the source — a body is chosen by the caller and a claim
is not, so reading it would give the scope straight back. A body naming a
different tenant is 403, not a silent redirect into the token's own.
`tenantrole.Authorize` takes no tenant argument at all, so no call site can
reintroduce this.

`role` is now trustworthy in a way it was not before — it is read from the
database for the tenant the token names — but it is still a snapshot up to the
token's lifetime old, so [internal/shared/tenantrole](internal/shared/tenantrole)
re-reads the row on every request. A demotion lands on the next call rather than
whenever the token happens to expire.

The middleware rejects, rather than defaults, on every claim it needs: no
`aud`, no `tenantId`, no `exp`, wrong audience. A claim that is merely absent is
the shape a forged or stale token takes.

`aud` stops another service that verifies HS256 against the same secret from
taking a go-token as its own.

That secret is migrating off `NEXTAUTH_SECRET`, which the session cookie's key
also derives from — sharing one means a leaked go-token key is also a forged
session. The frontend treats its `GO_API_JWT_SECRET` as optional and falls back
to `NEXTAUTH_SECRET`, so this side does too, in three states:

| State | Config | Accepted |
|---|---|---|
| 1 | `GO_API_JWT_SECRET` unset | `NEXTAUTH_SECRET` |
| 2 | it set | **both** |
| 3 | `GO_API_JWT_LEGACY_FALLBACK=false` | only `GO_API_JWT_SECRET` |

State 2 is what lets either side deploy first. **State 3 is the only one that
buys anything**, and `config.Load` warns at boot until a deployment reaches it.
The 32-byte floor applies to the new variable when set — the Bearer
verification is hand-rolled HMAC, so nothing in the crypto path would otherwise
stop a short, offline-brute-forceable key. It is not applied to
`NEXTAUTH_SECRET`, which is already deployed at whatever length it has.

`jti` backs a Redis denylist (`go-token:revoked:<jti>`) for a logout or an
access revoked inside the token's window. The check fails **open** on a Redis
error and skips tokens with no `jti`: what it shortens is a window already
bounded by `exp`, and failing closed would take every admin route down whenever
Redis blinks. Role changes never depend on it.

Which roles pass is declared per route, at the call site:

| Route | Roles |
|---|---|
| `POST /imports/members` | `owner`, `admin` |
| `POST /videos/upload` | any role in the tenant |
| `POST /sso/generate-token` | any role **for their own account**; `owner`/`admin` to mint for another `userId` |

The SSO split is not incidental. The token that endpoint mints is redeemed at
`validate-token` for the target user's identity on the tenant's external site,
so minting one for somebody else is impersonation rather than delegation.

`POST /api/v1/sso/validate-token` stays under `/api/v1` behind the tenant API
key: it is called by the tenant's own site, which holds no NextAuth session and
has no way to mint a Bearer.

`POST /videos/upload` used to carry no credential at all, and
`POST /sso/generate-token` used to be gated by `x-internal-api-key`. Both moved
in one cutover — the old paths are gone, not dual-mounted — so the frontend has
to deploy alongside this service.

### Magic links

Two places mint passwordless login links — `POST /api/v1/auth` and the
first-access emails from `member_import`. They share
[internal/shared/magiclink](internal/shared/magiclink), which is the Go half of
a contract the Next.js frontend owns.

A link is `https://<tenant domain>/api/auth/magic/<shortCode>`. The short code
is the only thing in the URL: the member's address used to travel in the query
string and no longer does, because a login URL ends up in mail archives, proxy
logs and shared screenshots. The frontend resolves the code, stamps `usedAt` and
finishes the sign-in — so a guessed code posted straight at `/login` does not
authenticate, since only the magic route sets that column.

The host is the tenant's `customDomain`, or its subdomain under
`PUBLIC_DOMAIN_URL`. That variable is the only domain this service knows:
`PUBLIC_ROOT_DOMAIN` used to sit beside it holding the backend's own host:port,
every deployment set the two to the same value, and the one path that read it
built member-facing links pointing at the backend. It is gone.

A reset link is the same URL with `?next=reset`, which tells the frontend to
leave `usedAt` alone; there the reset handler is what claims the row. Only the
delivery email emits it. `?redirect=<path>` is a post-login destination and is
accepted as a same-site relative path only — that check is the whole of the
open-redirect defence.

Both slices write two hashes for one token, and they are not interchangeable:

| Column | Hash | Why |
|---|---|---|
| `"MagicToken"."token"` | sha256 hex | Compared against what the frontend computes; a salted hash never matches |
| `"User"."magicToken"` | bcrypt | Legacy column, honoured by the old `/login?token=` route |

Minting the `"MagicToken"` row is not optional. If the insert fails, the request
fails — falling back to the old link shape would put the member's address back
in a URL without anyone noticing.

`"MagicToken"."method"` is the audit trail for how a link came to exist; each
minting path uses its own value (`api_magic_link`, `admin_import`) rather than
borrowing the frontend's.

## CORS

One policy on the root router: a literal `*`, no Origin reflection, no
`Access-Control-Allow-Credentials`.

The last two are the point. Reflecting the Origin *and* allowing credentials —
which is what this sent until the go-token work — is the pair browsers read as
"any site may make an authenticated cross-origin request here and read the
reply". `GET /api/comments` authenticated with the `next-auth.session-token`
cookie, so any page could have driven it with a visitor's session attached.

Two changes closed that, and they belong together: the cookie route was removed,
and the policy became a literal `*` with credentials off. Either alone would
have been weaker — the wildcard is the enforcement, because a browser will not
send credentials to a wildcard origin at all.

It stays a wildcard rather than an allowlist because tenants bring their own
custom domains and the set is not enumerable from a deployment. Nothing is given
away by that now: every credential this service accepts travels in a header a
browser will not attach on its own, so a cross-origin request from an attacker's
page arrives unauthenticated — a request from nobody.

If a route ever needs to be callable cross-origin from a browser *with* a
credential, give it a header credential. Do not widen this back.

## Rate limiting

Three Redis-backed limiters in [internal/platform/ratelimit](internal/platform/ratelimit)
(tenant, IP, upload bytes), wrapped by middlewares. Routes compose them through
each slice's `MiddlewareSet` — the router owns construction, slices only
declare what they need.

The upload-bytes limiter is keyed on the go-token's `sub`, read off the context
`BearerMiddleware` populates. It used to be keyed on a `user_id` request
header, which meant the quota was keyed on a value the caller writes — a client
could reset its own quota by sending a different one each time, and the header
was mandatory on a route where the token already names the user. That header is
gone; sending it now does nothing. `CheckUploadLimit` therefore only works
mounted below `RequireAuth`, and answers 401 rather than charging a default
bucket if it is not.

## Background work

Started by `App.Run` and stopped in reverse order on shutdown, before the
database closes:

- **analytics** — daily and monthly rollups on a cron (`robfig/cron` with seconds).
- **notifications** — polls the Notification table, dispatches FCM pushes.
- **transcription** — polls its own jobs table; video → Whisper → chunk → embed → pgvector.
- **member_import** — startup reset for orphaned imports, plus a 24h retention job.

## Observability

OpenTelemetry, traces and metrics over OTLP/HTTP to a collector that runs
outside the deployment. [internal/platform/telemetry](internal/platform/telemetry)
owns the providers; `telemetry.Init` is called from `main` before `app.New`, and
both `main` functions are shaped as `os.Exit(run())` so the deferred flush
survives an error path — `os.Exit` and `log.Fatalf` skip defers.

Missing OTEL variables are not an error. The providers are simply never
installed and the service runs uninstrumented, which is what a laptop without a
collector needs.

What is instrumented:

- **HTTP** — `otelchi` on the root router, with `WithChiRoutes` so spans carry
  the route pattern (`/api/v1/vitrine/{vitrineId}`) rather than the raw path.
  The OTel middlewares sit above every auth middleware, so a 401 is still
  measured.
- **SQL** — `otelsql` wraps both pools in `internal/platform/database`; each
  carries a `db.role` attribute (`tenant` / `transcription`) so the two are
  distinguishable. Pool gauges come from `RegisterDBStatsMetrics`.
- **Redis** — `redisotel` tracing and metrics on the go-redis client.
- **Outgoing HTTP** — `telemetry.Client` / `telemetry.Transport` for Bunny,
  iLovePDF, Resend, Spaces and OpenAI. Use them instead of a bare
  `&http.Client{}`, or the call becomes invisible.
- **cmd/analytics** — one root span per command.

Background workers are **not** instrumented yet. Their queries still produce
spans through `otelsql`, but with no parent: the notifications poller (10s) and
the transcription poller (30s) each emit a root trace per poll. The sampler is
`AlwaysSample` by design — sampling policy lives in the collector, so changing
the rate does not mean redeploying every customer — and the collector is what
drops that poller noise.

`middleware.RequestLogger` in [internal/shared/middleware](internal/shared/middleware)
replaced chi's `middleware.Logger`. It stamps `trace_id` and `span_id`, so a
span and its log line can be found from each other.

`GET /health` ([internal/features/api/health](internal/features/api/health))
pings the tenant database and Redis and answers 200 or 503. It carries no
credential, so the body never names which dependency failed — that goes to the
log. Point the platform's healthcheck at it.

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
