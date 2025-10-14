# Build stage
FROM golang:1.25.1-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./cmd/api

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

WORKDIR /home/appuser

# Copy binary from builder
COPY --from=builder /app/main .

# Change ownership
RUN chown -R appuser:appuser /home/appuser

# Switch to non-root user
USER appuser

# Environment variables with default values
ENV PORT=8181 \
    LOG_LEVEL=DEBUG \
    DB_DRIVER=postgres \
    BUNNY_BASE_URL=https://video.bunnycdn.com/ \
    BUNNY_TIMEOUT_SECONDS=30

# Expose port
EXPOSE 8181

# Run the application
CMD ["./main"]
