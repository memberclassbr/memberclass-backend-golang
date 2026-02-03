# MemberClass Backend - Golang

MemberClass application backend developed in Go, following Clean Architecture principles and Domain-Driven Design (DDD).

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

This project follows **Clean Architecture** with the following layers:

### Layers

1. **Domain Layer**
   - Contains pure business logic
   - Independent of external frameworks and libraries
   - Defines entities, interfaces (ports), and use cases

2. **Application Layer**
   - Orchestrates use cases
   - Contains HTTP handlers, middlewares, and route configuration
   - Depends only on the domain layer

3. **Infrastructure Layer**
   - Concrete implementations of interfaces defined in the domain
   - Repositories, cache adapters, external services
   - Depends on domain and application layers

### Architectural Principles

- **Dependency Inversion**: Inner layers don't depend on outer layers
- **Interface Segregation**: Specific and well-defined interfaces
- **Single Responsibility**: Each component has a single responsibility
- **Dependency Injection**: Using Uber FX for dependency injection
- **Test-Driven Development**: Comprehensive test coverage

## 📁 Project Structure

```
memberclass-backend-golang/
├── cmd/
│   └── api/                    # Application entry point
│       └── main.go
│
├── internal/
│   ├── application/            # Application Layer
│   │   ├── handlers/
│   │   │   └── http/           # HTTP Handlers (Controllers)
│   │   ├── middlewares/        # HTTP Middlewares
│   │   └── router/             # Route configuration
│   │   
│   ├── domain/                 # Domain Layer (Core Business)
│   │   ├── constants/          # Domain constants
│   │   ├── dto/                 # Data Transfer Objects
│   │   │   ├── request/        # Request DTOs
│   │   │   └── response/       # Response DTOs
│   │   ├── entities/           # Business entities
│   │   ├── memberclasserrors/  # Custom errors
│   │   ├── ports/              # Interfaces (Contracts)
│   │   ├── usecases/           # Use cases (Business Logic)
│   │   └── utils/              # Domain utilities
│   │
│   ├── infrastructure/         # Infrastructure Layer
│   │   └── adapters/           # Concrete implementations
│   │       ├── cache/          # Cache (Redis)
│   │       ├── database/       # Database configuration
│   │       ├── external_services/  # External services
│   │       │   ├── bunny/      # Bunny CDN integration
│   │       │   └── ilovepdf/   # iLovePDF integration
│   │       ├── logger/         # Logging system
│   │       ├── rate_limiter/   # Rate limiting
│   │       ├── repository/     # Data repositories
│   │       └── storage/        # Storage (S3)
│   │
│   └── mocks/                  # Test mocks
│
├── docker-compose.yml          # Docker configuration
├── Dockerfile                  # Docker image
├── Makefile                    # Automation commands
├── .mockery.yaml              # Mockery configuration
├── swagger.yaml                # OpenAPI documentation
├── memberclass-api.postman_collection.json  # Postman collection
└── README.md                   # This file
```

## 🚀 Technologies

### Language and Framework
- **Go 1.25.1** - Main language
- **Chi Router v5** - HTTP routing
- **Uber FX** - Dependency injection

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
PUBLIC_ROOT_DOMAIN=localhost:8181

#Memberclass Transcription

TRANSCRIPTION_API_URL=

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
- `PUBLIC_ROOT_DOMAIN` - Public root domain for magic links generation (default: localhost:8181)

**Memberclass Transcription**
-`TRANSCRIPTION_API_URL`- Url to app memberclass transcription

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
go test ./internal/domain/usecases/...
```

### Generate mocks

```bash
go run github.com/vektra/mockery/v2@latest
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

The `swagger.yaml` file contains the complete API specification in OpenAPI 3.0.3 format.

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
