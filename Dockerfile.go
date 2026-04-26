# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY src-go/ ./src-go/

# Build binary with CGO for SQLite
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o pastebin ./src-go

# Runtime stage
FROM alpine:latest

# Install tini for proper init handling and SQLite runtime
RUN apk add --no-cache tini ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/pastebin /app/pastebin

# Create config, data, and logs directories
RUN mkdir -p /config /data /logs

# Environment variables
ENV CONFIG_DIR=/config
ENV DATA_DIR=/data
ENV LOGS_DIR=/logs
ENV DB_TYPE=sqlite
ENV DB_PATH=/data/pastebin.db

# Expose port
EXPOSE 3010

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:3010/healthz || exit 1

# Use tini as init
ENTRYPOINT ["/sbin/tini", "--"]

# Run the application
CMD ["/app/pastebin", "--service"]
