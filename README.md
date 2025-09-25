# MemberClass Backend - Golang

MemberClass application backend developed in Go, following Clean Architecture principles and Domain-Driven Design (DDD).

## 🏗️ Architecture

This application follows **Clean Architecture** with the following layers:

### 📁 Project Structure

```
├── cmd/api/                    # Application entry point
├── internal/
│   ├── application/            # Application Layer
│   │   ├── handlers/http/      # HTTP Handlers (Controllers)
│   │   ├── middlewares/        # HTTP Middlewares
│   │   └── router/             # Route configuration
│   ├── domain/                 # Domain Layer (Core Business)
│   │   ├── entities/           # Business entities
│   │   ├── dto/                # Data Transfer Objects
│   │   ├── ports/              # Interfaces (Contracts)
│   │   ├── usecases/           # Use cases (Business Logic)
│   │   ├── utils/              # Domain utilities
│   │   └── memberclasserrors/  # Custom errors
│   └── infrastructure/         # Infrastructure Layer
│       └── adapters/           # Concrete implementations
│           ├── cache/          # Cache (Redis)
│           ├── database/       # Database
│           ├── external_services/ # External services (Bunny CDN)
│           ├── logger/         # Logging system
│           ├── rate_limiter/   # Rate limiting
│           └── repository/     # Data repositories
├── internal/mocks/             # Test mocks
├── docker-compose.yml          # Docker configuration
├── Dockerfile                  # Docker image
├── Makefile                    # Automation commands
└── .mockery.yaml              # Mockery configuration
```

### 🎯 Architecture Principles

- **Dependency Inversion**: Inner layers don't depend on outer layers
- **Interface Segregation**: Specific and well-defined interfaces
- **Single Responsibility**: Each component has a single responsibility
- **Dependency Injection**: Using Uber FX for dependency injection
- **Test-Driven Development**: Comprehensive test coverage

## 🚀 Technologies Used

- **Go 1.25.1** - Main language
- **Chi Router** - HTTP routing
- **PostgreSQL** - Main database
- **Redis** - Cache and rate limiting
- **Bunny CDN** - CDN for video uploads
- **Uber FX** - Dependency injection
- **Testify** - Testing framework
- **Mockery** - Mock generation
- **Docker** - Containerization

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
cp env.example .env
```

Edit the `.env` file with your configurations:

```env
# Application Configuration
PORT=8080
LOG_LEVEL=INFO

# Database Configuration
DB_DRIVER=postgres
DB_DSN=postgres://username:password@host:port/database_name?sslmode=disable

# Redis Configuration
UPSTASH_REDIS_REST_URL=your_redis_url
UPSTASH_REDIS_REST_TOKEN=your_redis_token

# Bunny CDN Configuration (optional)
BUNNY_BASE_URL=https://video.bunnycdn.com
BUNNY_TIMEOUT_SECONDS=30
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

### Environment Variables in Docker

Docker Compose uses the following environment variables:

- `PORT` - Application port (default: 8080)
- `DB_DRIVER` - Database driver
- `DB_DSN` - Database connection string
- `LOG_LEVEL` - Log level
- `BUNNY_BASE_URL` - Bunny CDN base URL
- `BUNNY_TIMEOUT_SECONDS` - Bunny CDN timeout
- `UPSTASH_REDIS_REST_URL` - Redis URL
- `UPSTASH_REDIS_REST_TOKEN` - Redis token