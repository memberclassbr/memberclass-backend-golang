# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2025-09-25

### Added

- **Video Upload API** with multipart form data support
- **Tenant Management** with PostgreSQL database integration
- **Rate Limiting** per user for video uploads using Redis
- **Bunny CDN Integration** for video storage and streaming
- **File Type Detection** using mimetype library
- **Clean Architecture** implementation with proper layer separation
- **Comprehensive Unit Tests** with 75-100% coverage
- **Docker Support** with multi-stage builds
- **Environment Configuration** with .env support
- **Structured Logging** with different levels
- **Graceful Shutdown** with proper resource cleanup

### Technical Stack

- **Go 1.25.1** - Main programming language
- **Chi Router** - HTTP routing and middleware
- **PostgreSQL** - Primary database
- **Redis** - Caching and rate limiting
- **Bunny CDN** - Video storage and streaming
- **Uber FX** - Dependency injection
- **Testify** - Testing framework
- **Docker** - Containerization

### API Endpoints

- `POST /api/v1/videos/upload` - Upload video files

### Configuration

Environment variables required:
- `PORT` - Application port (default: 8080)
- `DB_DSN` - PostgreSQL connection string
- `UPSTASH_REDIS_REST_URL` - Redis connection URL
- `UPSTASH_REDIS_REST_TOKEN` - Redis authentication token
- `BUNNY_BASE_URL` - Bunny CDN base URL

### Development

Available commands:
- `make run` - Run application locally
- `make test` - Run all tests
- `make docker-build` - Build Docker image
- `make docker-run` - Run with Docker Compose

### Breaking Changes

- None (initial release)

### Migration Guide

- No migration needed (initial release)