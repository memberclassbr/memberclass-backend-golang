# Build stage
FROM golang:1.25.1-alpine AS builder

WORKDIR /app

# Copy go mod filess
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./cmd/api

# Final stage
FROM alpine:latest

# ca-certificates for HTTPS; tzdata for cron schedules.
# ffmpeg is required by the transcription slice — it extracts audio from
# Bunny HLS playlists and splits long files for Whisper. Without it the
# slice logs a warning at startup and refuses to run jobs.
RUN apk --no-cache add ca-certificates tzdata ffmpeg

# Create non-root user
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

WORKDIR /home/appuser

# Copy binary from builder
COPY --from=builder /app/main .
COPY --from=builder /app/swagger.yaml .

# Change ownership
RUN chown -R appuser:appuser /home/appuser

# Switch to non-root user
USER appuser

# Environment variables with default values
ENV PORT=8181 \
    LOG_LEVEL=DEBUG \
    DB_DRIVER=postgres \
    BUNNY_BASE_URL=https://video.bunnycdn.com/library/ \
    BUNNY_TIMEOUT_SECONDS=30

# Expose port
EXPOSE 8181

# Run the application
CMD ["./main"]
