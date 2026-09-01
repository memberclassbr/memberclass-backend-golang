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
  `schema_migrations_go` table.

  These files started life as scripts run by hand with `psql -f`, and two habits
  from that life do not survive the move: a psql **meta-command** is not SQL and
  the server rejects the whole batch (`\gset` in `000` took a deployment down at
  boot with `pq: syntax error at or near "\"`), and an explicit **`BEGIN;`/
  `COMMIT;`** ends the transaction `applyMigration` already opened, which puts
  the `schema_migrations_go` row outside it. `TestMigrations_*` in
  [migrate_test.go](internal/platform/database/migrate_test.go) fails CI on
  either, since neither shows up until a deployment with a transcription DSN
  boots.

  `schema_migrations_go` is **younger than the schema it tracks**, which is the
  other half of the same story: these files were applied by hand before the
  runner existed, so a database can be fully migrated and still have an empty
  bookkeeping table. The runner would then read every file as pending and apply
  it a second time — merely wasteful on an empty database, destructive on a
  populated one, since `002` deletes every row in `chunks`, `transcripts` and
  `videos`. That is exactly why production broke where development passed: same
  schema, only one of them with data.

  So every migration declares a **probe** in `migrationProbes`
  ([migrate.go](internal/platform/database/migrate.go)) — one boolean query
  looking for the artefact that migration leaves behind. A pending migration
  whose probe says "already there" is recorded as applied without being run,
  once per database, and logged as `Adopted transcription migration`. A probe
  must never raise (`to_regclass`, never `::regclass`) because a probe that
  errors fails the boot, and it must be specific, because a false positive
  retires a migration that never ran. `TestMigrationProbes_CoverEveryMigration`
  makes adding a migration force the question.

  The migrations **alter** that schema; they do not create it. `videos`,
  `chunks`, `transcripts`, `jobs` and `token_usage` exist in no `CREATE TABLE`
  in this repository, so a transcription database that has never held them
  fails at `000`. Point `DB_TRANSCRIPTION_DSN` at an existing one, or leave it
  unset — it is optional, and unset simply disables the slice.

  One environment constraint is set by the runner rather than by any file:
  `applyMigration` opens each transaction with
  `max_parallel_maintenance_workers = 0` and `maintenance_work_mem = '32MB'`.
  Postgres in a container gets Docker's default 64MB `/dev/shm`, and pgvector
  sizes the shared memory segment for a parallel HNSW build from
  `maintenance_work_mem` — which on a managed instance does not fit.

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
[swagger.yaml](internal/features/api/docs/swagger.yaml). The legacy name goes when its callers do —
dropping it earlier would log out every integration in one deploy.

### Tenant API keys: `"TenantApiKey"`

One key per area became **N named keys per area**, owned by the Next.js panel,
which creates, renames, deletes and expires them. This service only reads them.
`AuthExternalMiddleware` resolves `sha256(key) → ("TenantApiKey".id, tenant)`:

- The hash is **unique globally**, not per tenant, because at the moment of
  authentication there is only a header — the tenant is the *result* of the
  lookup, not an input to it.
- **Expiry is compared in the authenticating query itself**
  (`expiresAt IS NULL OR expiresAt > now()`), never by a job that materialises
  a status. Any gap between the two is a window where an expired key still
  works. `now()` is the **database's** clock: the panel derives its "Expirada"
  label from that same clock, and a drifting container would make the screen
  and the gate disagree.
- An expired key answers exactly like an unknown one — `INVALID_API_KEY`.
  Telling them apart tells someone guessing that a value was once real. A
  missing header is the one thing distinguished, as `MISSING_API_KEY`: a caller
  that sent no credential already knows it sent none.
- **A key found there falls back to `Tenant.token_api_auth`**, the
  single-key-per-area column this replaced. **Nothing backfills that column
  into `"TenantApiKey"`** — the panel's schema says so in as many words, and no
  such script exists. An area leaves the old column behind by creating named
  keys in the panel, not by being migrated, so until it does, the fallback is
  the only path its integrations have: without it they answer 401 the moment
  this service deploys, indistinguishably from a wrong key.

  The fallback is taken on **any** failure of the first lookup, not only on
  "no rows": the case it is really for is a database whose panel migration has
  not run, where `"TenantApiKey"` does not exist and the query raises.

  **The two stores being disjoint is what makes this safe**, and it is not a
  detail to preserve casually. The legacy query has no expiry predicate, so a
  hash sitting in *both* places would keep authenticating after the panel
  expired or deleted its `"TenantApiKey"` row — revoked on the screen, live at
  the door, and invisible either way, since the legacy path is counted nowhere.
  Anything that ever copies a hash into `"TenantApiKey"` has to clear
  `token_api_auth` in the same statement.

  A legacy key has **no id and no expiry**, so it is counted in no usage panel
  and nothing in the panel can retire it — a named key is what buys those, not
  the authentication. Two things say the fallback is still load-bearing: the
  counter `apikey.auth.legacy_fallback`, and a boot warning when
  `"TenantApiKey"` is empty while the old column is not (a warning, not an
  abort — a customer created after this shipped has both counts at zero
  legitimately). **The fallback is temporary**; removing it once both are
  clean is [issue #38](https://github.com/memberclassbr/memberclass-backend-golang/issues/38).

There are **no scopes**. Every key is valid for every endpoint of its area, so
the key id is never put in the request context — a value there is an invitation
to start scoping by it. It stays in the auth middleware's closure, which is the
only thing that needs it.

Rate limiting stays **per area, not per key**: all of an area's keys share one
quota. Per-key would silently multiply the quota of whoever creates more keys.

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

The tenant limiter had the same defect in a second place: it read `?tenantId=`
from the query string **before** the authenticated tenant, so a caller holding
a valid API key moved itself into an empty bucket by changing the value on
every request. The context wins now, and the query string is only read on
routes mounted without `AuthExternal`, which are already reachable without a
credential.

## API key usage

Per-key usage feeds a panel, not a bill, and that is why none of it is on the
request's durability path. `AuthExternalMiddleware` is the only place that
knows which key a request arrived with, so it counts on the way back out:
`internal/platform/apikeyusage` does one pipelined round trip to Redis —
`HINCRBY` into two hashes per UTC day, `apikey:usage:req:<date>` and
`apikey:usage:err:<date>`, field `<apiKeyId>|<endpoint>`.

It is mounted **above** the rate limiter, so a 429 is counted rather than lost.
`errors` is any status ≥ 400 and a strict subset of `requests`: a broken
integration is meant to show up as a spike in errors, not in volume.

`endpoint` is the **chi route pattern**, normalised to the panel's spelling
(`/api/v1/comments/{commentId}` → `comments/:commentId`). A request with no
pattern — a 404, a 405, a wildcard mount — is counted nowhere, because the only
identifier left for it is the raw path, which is the cardinality the pattern
exists to avoid.

The write can never fail the request, so it is wrapped in a 100ms deadline of
its own (`context.WithoutCancel`, since a client hanging up does not un-make
the request it already made) behind a small circuit breaker — without one,
"degrade quietly" means adding 100ms to every request in the service while
Redis is down. Failures are counted as `apikey.usage.record.errors` rather than
only swallowed: the failure mode is otherwise a panel that says "Nunca" with
nothing anywhere saying why.

[internal/features/workers/api_key_usage](internal/features/workers/api_key_usage)
folds those hashes into `"ApiKeyUsageDaily"` **hourly**, not at midnight. A
daily job would leave the current day blank, so an integration that breaks at
09:00 would not appear until the following night. Hourly is free because the
upsert **assigns** the day's total rather than adding to it — the counters in
Redis are cumulative, so re-running is idempotent, which is also what makes a
retry safe.

Four things that are not obvious:

- **A lock, `SET NX`, per day.** Every replica starts the scheduler, and two of
  them running `HGETALL` + `DEL` over one hash is lost data: whoever deletes
  first takes the counters out from under the other.
- **Only a day that has ended is deleted from Redis.** Today's hash is still
  being written to. The hashes carry a 72h TTL and the job scans three days
  back, so a job that fails for a whole day still finds them.
- **`gen_random_uuid()` supplies the row's `id`.** Prisma's `@default(cuid())`
  is generated by the client, not by CockroachDB, so an insert from Go has to
  bring its own; nothing reads it, since the row's identity is
  `(apiKeyId, date, endpoint)`.
- **`lastUsedAt` is stamped by the same pass**, not written during the request.
  The job already knows which keys were used, so no request pays a database
  write for telemetry. Granularity is a day — the panel renders a date — so the
  hourly lag is invisible.

The day is **UTC**, matching the panel and the `date` column, and this is the
one place in the service that does not use the tenant's local day the way
`analytics` does. Near midnight in Brazil the two disagree about which day a
request belongs to. Changing it would mean hourly Redis keys and a data
migration.

Retention is 90 days, deleted once a day at 03:00 UTC rather than hourly:
`"ApiKeyUsageDaily"` is indexed on `(tenantId, date)`, so a delete filtered on
`date` alone cannot use that index and scans.

## Background work

Started by `App.Run` and stopped in reverse order on shutdown, before the
database closes:

- **analytics** — daily and monthly rollups on a cron (`robfig/cron` with seconds).
- **notifications** — polls the Notification table, dispatches FCM pushes.
- **transcription** — polls its own jobs table; video → Whisper → chunk → embed → pgvector.
- **member_import** — startup reset for orphaned imports, plus a 24h retention job.
- **api_key_usage** — hourly, on the same cron; folds the per-key Redis counters
  into `"ApiKeyUsageDaily"`. See below.
- **bunny_usage** — daily, on the same cron; records per-area Bunny storage and
  traffic into `"TenantBunnyMonthlyUsage"`. See below.

## Bunny usage

`"TenantBunnyMonthlyUsage"` holds one row per area per **UTC** month, keyed
`(tenantId, year, month)`. The Prisma schema in the sibling `mult-memberclass`
repository owns those columns and the panel only reads them: **this worker is
the sole writer**, which is why the manual trigger the panel used to have was
removed rather than kept alongside.

Everything follows from traffic and storage not being the same kind of number.

| | Traffic | Storage |
|---|---|---|
| What it is | a flow accumulated over the month | a reading of one instant |
| Current month | `TrafficUsage` (`GET /videolibrary/{id}`) | `StorageUsage`, same call |
| Closed month | `TotalBandwidthUsed` (`GET /statistics`) | does not exist |
| History at Bunny | one **rolling** year | **none** |
| How it is written | overwritten | sampled, one per day |

Traffic is Bunny's own running total, which it zeroes at the UTC turn of the
1st, so writing it is overwriting it: a run repeated in one day writes the same
number, and a missed day repairs itself on the next. Storage has no past
anywhere — `GET /statistics` carries no storage field at all — so a month's
storage is only ever the samples taken during it. **A day the worker did not
run is a day of storage nobody can reconstruct**, and that asymmetry is the
reason the whole thing exists on a schedule rather than on demand.

Which is also why every `storage*` column is nullable and **null means "not
measured", never zero**. A backfilled month has no storage and never will; an
area whose library answered 404 has a number nobody knows. Neither is an area
that used nothing, and the screen has to be able to tell them apart.

### The daily run

Once a day at 05:30 UTC, holding a `SET NX` day lock so several replicas do not
each walk the account. For every area with a `bunnyLibraryId`:

- `GET /videolibrary/{bunnyLibraryId}` → `StorageUsage`, `TrafficUsage`,
  `PullZoneId`.
- One upsert writes the current month: traffic assigned, storage accumulated
  **only when `lastSampleDate` is not today** — without that guard a second run
  in one day would weight that day double in the average. The `WHERE closedAt
  IS NULL` on the `DO UPDATE` is the entire rewrite lock on a closed month.
- `Tenant.bunnyStorageBytes`, `bunnyTrafficBytes` and `bunnyUsageUpdatedAt` are
  mirrored, because that is where the manager reads from today
  (`actions/manager/get-areas.tsx`). They go when the tenant usage screen
  exists; until then, not writing them means that screen quietly stops moving.
- A **404** writes `source = "missing"` and touches no number, on an existing
  row least of all: an absent library is unknown usage, not zero usage.

`PullZoneId` arrives in the same response and is stored on `Tenant`, so
resolving it is a lookup an area pays for once.

### Closing a month

The time of day is irrelevant for the current month and **decisive for a
finished one**. 05:30 UTC on the 1st is 02:30 in Brasília, and `TrafficUsage`
was reset five and a half hours earlier — closing from the library counter
would drop the last ~21 hours of every month, in a number that is passed on to
the customer. So the closing pass reads the period instead:

```
GET /statistics?pullZone={id}&dateFrom=<1st>&dateTo=<last day>
```

`TotalBandwidthUsed` already sums the period; `BandwidthUsedChart` is the same
figure spread over days and does not need adding up. `closedAt` is then
stamped, and the daily pass stops touching the row. The two paths agree: on
pull zone `3697175`, `TotalBandwidthUsed` 129.377.181 against the library's
`TrafficUsage` 129.375.439 — the traffic served between the two calls.

Closing runs **before** sampling, and scans every open month rather than only
the last one, so a worker that was down for a week still closes what it missed. It
is bounded at 12 months because that is what `/statistics` answers for.

### Backfill

`cmd/analytics -cmd=bunny-backfill` — one call per pull zone per month, up to 12
months back. **Worth running as early as possible**: Bunny's window is a
*rolling* year, so every month of delay erases a month from the far end for
good. With ~121 libraries that is ~1.500 requests, about an hour at the
throttle.

It is **resumable, and that is the feature that matters**. An existing row is
skipped before its request is spent, so a run that stops halfway costs nothing
and running it again picks up exactly where it left off — including the gaps
inside an area it had only partly done.

A backfilled row carries traffic, `source = "backfilled"` and a `closedAt`;
every `storage*` column and `lastSampleDate` stay null. Months older than the
retention get **no row at all** — an absent row reads as "we don't know", and a
zeroed one would be a claim. An existing row is never overwritten, and is
skipped before the request is spent rather than after.

Two measured limits, both enforced in `internal/platform/bunny` before the call
rather than discovered from a response: a window wider than **40 days**
(`statistics.date_range_invalid`) and a `dateFrom` older than **one year**.

`-cmd=bunny-close --tenantId=X --month=YYYY-MM` reprocesses a month that is
already closed. It is the one write that ignores `closedAt`, which is why
nothing scheduled can reach it.

### The rate limit

Bunny's limit is **per account**, so every deployment, the transcription
slice's pull-zone lookups and anything else on the same key share one budget.
It is easy to underestimate from a single run: a backfill at a 200ms throttle
went clean for ~50 seconds and then took a 429 on **every** remaining call, 24
consecutive months across 5 tenants, writing nothing.

Two things came out of that, and they are a pair:

- **A 429 is waited out, not reported** — `Retry-After` when Bunny sends one,
  otherwise 2s doubling to 16s over four attempts
  ([account.go](internal/platform/bunny/account.go)). Without the wait a
  rate-limited run *accelerates*: a 429 comes back faster than a real response,
  so the caller's own throttle stops pacing anything and the run drives itself
  harder into the wall it just hit.
- **A 429 that survives the backoff is `ErrRateLimited`, and it aborts the
  run**, exactly like `ErrUnauthorized`. It is systemic — the next area meets
  the same wall — so continuing does not collect data, it just burns areas
  producing nothing.

The throttle is **1s**, roughly 26 requests a minute once the ~1.3s round trip
is counted. Aborting early is cheap precisely because both the backfill and the
daily pass are resumable; burning the account's budget is not.

### Alerting

Only on **systemic** failure. A 401/403 or an exhausted rate limit aborts the
run immediately — one bad account key fails every area, and carrying on would
bury the cause under a hundred identical errors. More than **20%** of areas
failing in one run fails the job. A per-area 404 does neither: it is expected,
it becomes `source = "missing"`, and alerting on it would train everyone to
ignore the alert. `bunny.usage.areas.{synced,failed,missing}` and
`bunny.usage.months.closed` are what a "the worker stopped running" alert
watches, since a job that stops emits nothing at all.

## Observability

OpenTelemetry, traces and metrics over OTLP/HTTP to a collector that runs
outside the deployment: the service is on Railway, the collector on a dedicated
observability VPS. [internal/platform/telemetry](internal/platform/telemetry)
owns the providers; `telemetry.Init` is called from `main` before `app.New`, and
both `main` functions are shaped as `os.Exit(run())` so the deferred flush
survives an error path — `os.Exit` and `log.Fatalf` skip defers.

Missing OTEL variables are not an error. The providers are simply never
installed and the service runs uninstrumented, which is what a laptop without a
collector needs.

**Everything is a push, and nothing scrapes this process.** That is not a
detail; it is the reason for most of what the package does. Whatever a scrape
would have supplied has to be supplied by the exporter instead, and the three
places that bites are below.

`service.instance.id` is always set, never omitted. Under a scrape Prometheus
stamps `instance` itself from the target address; under a push nothing does.
Absent, every replica emits the same series identity, the collector's remote
write receives N interleaved streams as one series, and since each replica has
its own zero for a cumulative counter the series walks backwards and `rate()`
returns noise. `ServiceInstanceID` in
[identity.go](internal/platform/telemetry/identity.go) resolves
`OTEL_SERVICE_INSTANCE_ID` → `RAILWAY_REPLICA_ID` → hostname → a per-process
UUID, and memoises the result so the tracer and the meter cannot disagree about
which process they are. Wrong-but-unique is harmless; *shared* corrupts the
data.

Temporality is pinned cumulative in code, ignoring
`OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE`. The collector forwards
through `prometheusremotewrite`, which drops delta histograms **silently** while
simple sums keep flowing — so the symptom is "counters healthy, latency panels
empty", which reads like an application bug and is not one. Pinning the selector
makes the class unreachable.

Endpoint values are normalised in `config.loadTelemetry`. The exporters treat
`OTEL_ENDPOINT_HOST` as a Host and build `https://<host>/v1/metrics` themselves,
so a scheme left in the value corrupts the Host header and every export fails
against a host nobody typed. A scheme is stripped; `http://` turns plaintext
export on; a path is dropped with a warning, since it would be ignored rather
than prefixed. `OTEL_INSECURE` does the same thing explicitly and warns at boot
— the hop crosses the public internet, so it puts `OTEL_TOKEN` in the clear.

What is instrumented:

- **HTTP traces** — `otelchi` on the root router, with `WithChiRoutes` so spans
  carry the route pattern (`/api/v1/vitrine/{vitrineId}`) rather than the raw
  path. The OTel middlewares sit above every auth middleware, so a 401 is still
  measured.
- **HTTP metrics** — `telemetry.HTTPServerMetrics`
  ([httpmetrics.go](internal/platform/telemetry/httpmetrics.go)), an `otelhttp`
  wrapper, replaced otelchi's metric middlewares. Those recorded **no status
  code at all** — the service had a latency histogram and no way to compute an
  error rate — and labelled what they did record with the pre-1.21 names
  (`http.method`, not `http.request.method`), which no semconv-based dashboard
  or alert matches. Three things the wrapper adds by hand:
  - `http.server.active_requests`, because no released `otelhttp` records it for
    a *server*, only for a client transport;
  - `http.route`, added through the `otelhttp` labeler on the way back out —
    chi resolves the pattern as the request descends, and the labeler is read
    after the handler returns, which is the only reason it can be attached to
    the duration and body-size histograms at all. An unmatched request keeps its
    status code and gets no route, since the raw path is the cardinality the
    pattern exists to avoid;
  - method normalisation to `_OTHER`, because `net/http` accepts any token as a
    method and this runs before chi has rejected anything.

  It is mounted *inside* the tracing middleware so histograms record with a live
  span in context — that is what puts trace exemplars on the latency buckets.
  `middleware.Recoverer` moved *below* both: above them a handler panic unwound
  straight past the instrumentation and the resulting 500 was recorded by
  nothing.
- **Metric cardinality** — `httpServerAttributeView` allow-lists the labels on
  `http.server.*`. `otelhttp` derives `server.address` from the `Host` header,
  which the caller controls: on a public API that is unbounded, and the bill
  lands on the VPS's Prometheus. It is an allow-list, so a new attribute worth
  keeping must be added there as well as emitted.
- **`/health`** — served but neither traced nor measured, via
  `telemetry.Instrumented`, which the tracing filter and the metrics wrapper
  share so the two families measure the same set of requests. The platform's
  healthcheck runs every few seconds and would otherwise be the busiest route in
  the service. The skip sits outside `otelhttp` rather than in its `WithFilter`,
  because a rejected filter still calls the wrapped handler and
  `active_requests` would keep counting.
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
- Swagger is hand-maintained in
  [internal/features/api/docs/swagger.yaml](internal/features/api/docs/swagger.yaml),
  served at `/docs/`. It is a `text/template` embedded in the binary, not a file
  next to it: `PUBLIC_API_NAME` / `PUBLIC_API_URL` fill the brand and the
  `servers[]` host per deployment. Keep every placeholder quoted — unquoted
  `{{` is invalid YAML.
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
