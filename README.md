# MemberClass Backend - Golang

MemberClass application backend developed in Go, following Vertical Slice Architecture: one folder per feature, each owning its HTTP handling, its business rules and its SQL.

## 📋 Table of Contents

- [Architecture](#-architecture)
- [Project Structure](#-project-structure)
- [Technologies](#-technologies)
- [Patterns and Conventions](#-patterns-and-conventions)
- [Prerequisites](#-prerequisites)
- [Configuration](#-configuration)
- [Running the Application](#-running-the-application)
- [Features](#-features)
- [Testing](#-testing)
- [API Documentation](#-api-documentation)

## 🏗️ Architecture

This project follows **Vertical Slice Architecture**: one folder per feature,
where an endpoint reads top-to-bottom in a single file (`parse → rules → SQL`).
There is no domain/application/infrastructure split and no DI framework.

### Layout

- `internal/features/` — one folder per feature, grouped by audience:
  `api/` (tenant-facing), `admin/` (internal), `workers/` (background).
  Each slice owns its handlers, its rules and its SQL.
- `internal/platform/` — code that talks to the outside world: config, logger,
  Redis, Postgres, Spaces, rate limiting, and the Bunny / iLovePDF / Resend
  clients. Each package declares its contract next to its implementation.
- `internal/shared/` — what several slices need: the request's tenant, the
  session payload, middlewares, HTTP helpers, typed errors, pagination.
- `internal/app/` — the composition root. `app.go` wires everything by hand;
  `router.go` mounts the slices.

### Principles

- **A slice owns its SQL.** No repository layer, no port interface between a
  slice and its queries.
- **Duplicate before abstracting.** Shared code moves to `internal/shared/`
  only once the duplication actually hurts.
- **Config is resolved once.** `os.Getenv` lives in `internal/platform/config`
  and nowhere else; a deployment missing a required variable fails to start.
- **Tests use go-sqlmock and local fakes.** Nothing is generated, so
  `go test ./...` works on a fresh clone.

## 📁 Project Structure

```
memberclass-backend-golang/
├── cmd/
│   ├── api/                    # The service
│   └── analytics/              # CLI for rollups and backfills
│
├── internal/
│   ├── app/                    # Composition root (wiring + router)
│   │
│   ├── features/
│   │   ├── api/                # Tenant-facing endpoints
│   │   │   ├── activity_summary/  ai/  auth/  comment/  docs/
│   │   │   ├── social/  sso/  student/  user/  user_activities/
│   │   │   └── video/  vitrine/
│   │   ├── admin/              # Internal endpoints
│   │   │   ├── lesson_pdf/     # PDF → page images
│   │   │   └── member_import/  # Bulk member import
│   │   └── workers/            # Background pipelines
│   │       ├── analytics/      # Rollup jobs + cron scheduler
│   │       ├── notifications/  # FCM push dispatch
│   │       └── transcription/  # Video → Whisper → pgvector
│   │
│   ├── platform/               # Talks to the outside world
│   │   ├── bunny/  cache/  config/  database/  ilovepdf/
│   │   └── logger/  ratelimit/  resend/  storage/
│   │
│   └── shared/                 # Cross-slice rules and types
│       ├── constants/  httpx/  memberclasserrors/  middleware/
│       └── pagination/  session/  tenant/  utils/
│
├── scripts/smoke.sh            # Endpoint smoke test for a deployment
├── docker-compose.yml          # Docker configuration
├── Dockerfile                  # Docker image
├── Makefile                    # Automation commands
├── memberclass-api.postman_collection.json  # Postman collection
└── README.md                   # This file
```

## 🚀 Technologies

### Language and Framework

- **Go 1.25.1** - Main language
- **Chi Router v5** - HTTP routing

### Database and Cache

- **PostgreSQL** - Main database
- **Redis** - Cache and rate limiting

### External Services

- **Bunny CDN** - CDN for video uploads
- **iLovePDF** - PDF processing
- **AWS S3** - File storage

### Testing

- **Testify** - Testing framework
- **Mockery** - Mock generation
- **sqlmock** - Database mocking

### Tools

- **Docker** - Containerization
- **Swagger/OpenAPI** - API documentation
- **Postman** - API testing

## 📐 Patterns and Conventions

### Naming

- **Handlers**: `{resource}_handler.go` (e.g., `auth_handler.go`)
- **Use Cases**: `{resource}_usecase.go` (e.g., `auth_usecase.go`)
- **Repositories**: `{resource}_repository.go` (e.g., `user_repository.go`)
- **DTOs**: `{action}_{resource}_{request|response}.go` (e.g., `auth_request.go`)

### Code Structure

- Each handler has its own file
- Use cases contain business logic
- Repositories abstract data access
- DTOs separated for request and response

### Testing

- Test files: `{file}_test.go`
- Minimum coverage: 85% for use cases
- Use of mocks for dependency isolation

## 📋 Prerequisites

- Go 1.25.1 or higher
- PostgreSQL 12+
- Redis 6+
- Docker and Docker Compose (optional)
- Make (optional, for automated commands)

## ⚙️ Configuration

### 1. Clone the repository

```bash
git clone <repository-url>
cd memberclass-backend-golang
```

### 2. Configure environment variables

Copy the example file and configure the variables:

```bash
cp .env.example .env
```

Edit the `.env` file with your configurations:

```env
# Application Configuration
PORT=8181
LOG_LEVEL=INFO

# Database Configuration
DB_DRIVER=postgres

# Database Connection String (configure with your existing database)
DB_DSN="postgresql://root@192.168.18.2:26257/defaultdb?sslmode=disable"

# Redis Configuration
UPSTASH_REDIS_URL=
UPSTASH_REDIS_TOKEN=

# Bunny CDN Configuration (if needed)
BUNNY_API_KEY=
BUNNY_BASE_URL=https://video.bunnycdn.com/library/
BUNNY_TIMEOUT_SECONDS=30

# DigitalOcean Spaces Configuration
DO_SPACES_ID=
DO_SPACES_SECRET=
DO_SPACES_BUCKET=
DO_SPACES_URL=

# iLovePDF Configuration
ILOVEPDF_BASE_URL=https://api.ilovepdf.com/v1
ILOVEPDF_API_KEYS=

# Auth Configuration
INTERNAL_AI_API_KEY=
NEXTAUTH_SECRET=
GO_API_JWT_SECRET=
GO_API_JWT_LEGACY_FALLBACK=true
PUBLIC_DOMAIN_URL=memberclass.com.br

# Memberclass Transcription (Railway pgvector + OpenAI)
# See docs/plans/2026-05-13-transcription-go-vsa.md for setup details.
DB_TRANSCRIPTION_DSN=
OPENAI_API_KEY=
TRANSCRIPTION_WORKER_CONCURRENCY=2
TRANSCRIPTION_POLL_INTERVAL_SECONDS=30

```

### 3. Install dependencies

```bash
go mod download
```

### 4. Setup development environment (optional)

```bash
make dev-setup
```

This command will:

- Install Mockery for mock generation
- Generate all necessary mocks

### Environment Variables Reference

The application uses the following environment variables:

**Application:**

- `PORT` - Application port (default: 8181)
- `LOG_LEVEL` - Log level (INFO, DEBUG, ERROR)

**Database:**

- `DB_DRIVER` - Database driver (postgres)
- `DB_DSN` - Database connection string (PostgreSQL connection string)

**Redis:**

- `UPSTASH_REDIS_URL` - Redis REST URL
- `UPSTASH_REDIS_TOKEN` - Redis REST token

**Bunny CDN:**

- `BUNNY_API_KEY` - Bunny CDN API key
- `BUNNY_BASE_URL` - Bunny CDN base URL (default: https://video.bunnycdn.com/library/)
- `BUNNY_TIMEOUT_SECONDS` - Bunny CDN timeout in seconds (default: 30)

**DigitalOcean Spaces:**

- `DO_SPACES_ID` - DigitalOcean Spaces access key ID
- `DO_SPACES_SECRET` - DigitalOcean Spaces secret access key
- `DO_SPACES_BUCKET` - DigitalOcean Spaces bucket name
- `DO_SPACES_URL` - DigitalOcean Spaces endpoint URL

**iLovePDF:**

- `ILOVEPDF_BASE_URL` - iLovePDF API base URL (default: https://api.ilovepdf.com/v1)
- `ILOVEPDF_API_KEYS` - iLovePDF API keys (comma-separated list)

**Authentication:**

- `INTERNAL_AI_API_KEY` - Internal API key for AI endpoints validation
- `NEXTAUTH_SECRET` - Legacy signing key for go-token Bearer JWTs, still accepted because the frontend falls back to it. Must match the frontend byte-for-byte. Read by nothing once `GO_API_JWT_LEGACY_FALLBACK=false`
- `GO_API_JWT_SECRET` - Verifies the go-token Bearer JWTs on `/imports/*`, `/sso/*` and `/videos/*`. **Optional**, because the frontend's copy is: it signs with `NEXTAUTH_SECRET` when its own is unset, so both keys are accepted while this is set. At least 32 bytes when set — boot fails otherwise
- `GO_API_JWT_LEGACY_FALLBACK` - `false` stops accepting `NEXTAUTH_SECRET` on go-tokens. Set it once the frontend signs with `GO_API_JWT_SECRET`; until then the go-token key is still the session key, and the boot log says so (default `true`)
- `PUBLIC_DOMAIN_URL` - Customer-facing frontend root domain (bare host). Builds the `From` address of transactional email and the magic-link host for tenants without a `customDomain`. Falls back to `NEXT_PUBLIC_DOMAIN_URL`. Replaced `PUBLIC_ROOT_DOMAIN`, which is no longer read

**Memberclass Transcription (Railway pgvector + OpenAI):**

- `DB_TRANSCRIPTION_DSN` - Railway Postgres (pgvector template) connection string; required for the transcription slice to claim and process jobs
- `OPENAI_API_KEY` - OpenAI key used for whisper-1 (transcription) and text-embedding-3-small (embeddings)
- `TRANSCRIPTION_WORKER_CONCURRENCY` - goroutines processing transcription jobs in parallel (default 2)
- `TRANSCRIPTION_POLL_INTERVAL_SECONDS` - how often the worker polls the jobs table (default 30)

## 🏃‍♂️ Running the Application

### Local Development

#### Option 1: Using Make (Recommended)

```bash
# Run the application
make run

# Or build and run
make build
./bin/main
```

#### Option 2: Direct command

```bash
go run ./cmd/api
```

### Docker

#### Option 1: Using Make

```bash
# Build and run with Docker Compose
make docker-build
make docker-run
```

#### Option 2: Direct commands

```bash
# Build the image
docker build -t memberclass-backend .

# Run with Docker Compose
docker-compose up
```

## 🎯 Features

### Authentication and Authorization

- **POST /api/v1/auth** - Generate magic login link
  - API key validation via SHA-256
  - Magic token generation with bcrypt
  - Response caching (Redis)
  - Rate limiting per tenant

### AI and Transcriptions

- **PATCH /api/v1/ai/lessons/{lessonId}** - Update transcription status
  - Internal API key validation
  - AI enabled check for tenant
  - Rate limiting per lessonId

- **GET /api/v1/ai/tenants** - List tenants with AI enabled
  - Internal API key validation
  - Filter tenants with `aiEnabled = true`
  - Global rate limiting

### Comments

- **GET /api/v1/comments** - List comments
  - Filters: email, status, courseId, answered
  - Pagination
  - Rate limiting per tenant

- **PATCH /api/v1/comments/{commentID}** - Update comment
  - Publish/unpublish
  - Reply to comments
  - Rate limiting per tenant

### Users

- **GET /api/v1/user/informations** - User information
  - User data
  - Linked deliveries
  - Last access

- **GET /api/v1/user/activities** - User activities
  - Activity history
  - Pagination
  - Rate limiting per tenant

- **GET /api/v1/user/activity/summary** - Activity summary
  - Consolidated statistics
  - Rate limiting per tenant

- **GET /api/v1/user/lessons/completed** - Completed lessons
  - List of watched lessons
  - Pagination
  - Rate limiting per tenant

- **GET /api/v1/users/purchases** - User purchases
  - Purchase history
  - Pagination
  - Rate limiting per tenant

### Reports

- **GET /api/v1/student/report** - Student report
  - Student data
  - Linked deliveries
  - Watched lessons
  - Date filters
  - Pagination
  - Response caching
  - Rate limiting per tenant

### Social

- **POST /api/v1/social** - Create/update social post
  - Post creation
  - Update existing posts
  - Rate limiting per tenant

### Documentation

- **GET /docs/** - Swagger UI interface
- **GET /docs/swagger.yaml** - OpenAPI specification

### PDF Processing (Internal)

- **POST /api/lessons/pdf-process** - Process lesson PDF
- **POST /api/lessons/process-all-pdfs** - Process all pending PDFs
- **POST /api/lessons/{lessonId}/pdf-regenerate** - Regenerate PDF
- **GET /api/lessons/{lessonId}/pdf-pages** - Get PDF pages

## 🧪 Testing

### Run all tests

```bash
go test ./...
```

### Run tests with coverage

```bash
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Run tests for a specific package

```bash
go test ./internal/features/api/vitrine/...
```

### Smoke test a deployment

`scripts/smoke.sh` hits every route and reports the status of each. It proves
routes are mounted and enforcing their credentials; it does not compare
response bodies, so payload shapes remain a manual check.

```bash
BASE_URL=https://api.example.com MC_API_KEY=... INTERNAL_API_KEY=... ./scripts/smoke.sh
```

## 📚 API Documentation

### Swagger UI

Access the interactive documentation at:

```
http://localhost:8080/docs/
```

### Postman Collection

Import the `memberclass-api.postman_collection.json` collection into Postman to test all endpoints.

### OpenAPI Specification

The `internal/features/api/docs/swagger.yaml` file contains the complete API specification in OpenAPI 3.0.3 format.

It is embedded in the binary and served as a Go `text/template`: the brand name and the public base URL come from `PUBLIC_API_NAME` and `PUBLIC_API_URL` at request time, so a single image serves any deployment. Both are optional — unset, the docs render unbranded and boot logs a warning naming the variable. See [internal/features/api/docs/docs.go](internal/features/api/docs/docs.go).

## 🔒 Rate Limiting

The project implements rate limiting at multiple levels:

- **Per Tenant**: Limits requests per tenant (60 req/60s)
- **Per IP**: Limits requests per IP address (50 req/60s)
- **Per Endpoint**: Each endpoint has its own limit
- **Global**: For internal endpoints (60 req/60s)

### Rate Limit Headers

Responses include the following headers:

- `X-RateLimit-Limit`: Total limit
- `X-RateLimit-Remaining`: Remaining requests
- `X-RateLimit-Reset`: Reset timestamp
- `Retry-After`: Seconds until retry is allowed

## 🛠️ Useful Commands

### Make

```bash
make run              # Run application
make build            # Build application
make test             # Run tests
make test-coverage    # Tests with coverage
make docker-build     # Build Docker image
make docker-run       # Run with Docker Compose
make dev-setup        # Setup development environment.
```

## 📝 License

The MIT License.
