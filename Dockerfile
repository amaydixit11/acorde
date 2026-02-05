# ==========================================
# Stage 1: Builder
# ==========================================
FROM golang:1.25-alpine AS builder

# Install build dependencies
# gcc, musl-dev: Required for CGO (SQLite)
# git: Required for fetching dependencies
RUN apk add --no-cache gcc musl-dev git

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Build binary
# -ldflags="-w -s": Strip DWARF and symbol table to reduce size
COPY . .
RUN go build -ldflags="-w -s" -o acorde ./cmd/acorde

# ==========================================
# Stage 2: Runner
# ==========================================
FROM alpine:latest

# Metadata
LABEL maintainer="acorde-team"
LABEL description="ACORDE - Local-first P2P Data Sync Engine"

# Install runtime dependencies
# ca-certificates: Required for HTTPS
# tzdata: Required for accurate timestamps
# curl: Useful for healthchecks
RUN apk add --no-cache ca-certificates tzdata curl

# Create non-root user
RUN adduser -D -g '' acorde

# Set working directory
WORKDIR /home/acorde

# Copy binary from builder
COPY --from=builder /app/acorde .

# Create data directory and set permissions
RUN mkdir -p data && chown -R acorde:acorde data

# Switch to non-root user
USER acorde

# Open ports
# 4001: P2P Sync (TCP/UDP)
# 7331: REST API
EXPOSE 4001/tcp 4001/udp
EXPOSE 7331/tcp

# Healthcheck
# Queries the /status endpoint
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD curl -f http://localhost:7331/status || exit 1

# Volume for persistent data
VOLUME ["/home/acorde/data"]

# Entrypoint
ENTRYPOINT ["./acorde"]

# Default command
CMD ["daemon", "--data", "./data", "--port", "4001", "--api-port", "7331", "--mdns=false"]
