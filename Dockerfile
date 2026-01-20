# Build stage
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build arguments for versioning
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

# Build the main binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /build/maia ./cmd/maia

# Build the CLI binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s -X github.com/ar4mirez/maia/cmd/maiactl/cmd.version=${VERSION} -X github.com/ar4mirez/maia/cmd/maiactl/cmd.commit=${COMMIT}" \
    -o /build/maiactl ./cmd/maiactl

# Build the MCP server binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \
    -o /build/mcp-server ./cmd/mcp-server

# Final stage - minimal runtime image
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 maia && \
    adduser -u 1000 -G maia -s /bin/sh -D maia

# Create directories for data and config
RUN mkdir -p /data /config && \
    chown -R maia:maia /data /config

# Copy binaries from builder
COPY --from=builder /build/maia /usr/local/bin/maia
COPY --from=builder /build/maiactl /usr/local/bin/maiactl
COPY --from=builder /build/mcp-server /usr/local/bin/mcp-server

# Set environment variables
ENV MAIA_STORAGE_DATA_DIR=/data \
    MAIA_LOG_LEVEL=info \
    MAIA_LOG_FORMAT=json \
    MAIA_SERVER_HTTP_PORT=8080

# Switch to non-root user
USER maia

# Expose ports
EXPOSE 8080 9090

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:8080/health || exit 1

# Default command
ENTRYPOINT ["maia"]

# Default arguments (can be overridden)
CMD ["serve"]
