# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binaries
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-w -s" -o qubicdb ./cmd/qubicdb
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-w -s" -o qubicdb-cli ./cmd/qubicdb-cli

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 qubicdb && \
    adduser -u 1000 -G qubicdb -s /bin/sh -D qubicdb

# Create data directory
RUN mkdir -p /app/data && chown -R qubicdb:qubicdb /app

# Copy binaries from builder
COPY --from=builder /app/qubicdb .
COPY --from=builder /app/qubicdb-cli .

# Switch to non-root user
USER qubicdb

# Environment defaults
ENV QUBICDB_HTTP_ADDR=":6060"
ENV QUBICDB_DATA_PATH="/app/data"
ENV QUBICDB_ADMIN_ENABLED="true"
ENV QUBICDB_ALLOWED_ORIGINS="http://localhost:6060"
ENV QUBICDB_MCP_ENABLED="false"
ENV QUBICDB_MCP_PATH="/mcp"
ENV QUBICDB_MCP_STATELESS="true"
ENV QUBICDB_MCP_RATE_LIMIT_RPS="30"
ENV QUBICDB_MCP_RATE_LIMIT_BURST="60"
ENV QUBICDB_MCP_ENABLE_PROMPTS="true"

# Expose port
EXPOSE 6060

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:6060/health || exit 1

# Run
ENTRYPOINT ["./qubicdb"]
