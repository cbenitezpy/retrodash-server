# Multi-stage Dockerfile for Bridge Server
# Stage 1: Build Go binary
FROM golang:1.24-bookworm AS builder

WORKDIR /app

# Copy go.mod and go.sum first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o bridge ./cmd/bridge

# Stage 2: Runtime with Chromium
FROM debian:bookworm-slim

# Install Chromium and dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    chromium \
    chromium-sandbox \
    fonts-liberation \
    fonts-noto-color-emoji \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user for security
RUN groupadd -r bridge && useradd -r -g bridge -G audio,video bridge

# Set up directories
WORKDIR /app
RUN mkdir -p /tmp/chrome-data && chown -R bridge:bridge /tmp/chrome-data

# Copy binary from builder
COPY --from=builder /app/bridge /app/bridge

# Set Chrome path for the application
ENV CHROME_PATH=/usr/bin/chromium

# Use non-root user
USER bridge

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the server
ENTRYPOINT ["/app/bridge"]
