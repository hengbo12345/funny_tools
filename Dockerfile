# Multi-stage build for Mihomo Subscription Publisher
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Cache go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build statically linked binary
ARG GIT_COMMIT=""
ARG BUILD_DATE=""
RUN git config --global --add safe.directory /build 2>/dev/null || true && \
    COMMIT="${GIT_COMMIT:-$(git rev-parse HEAD 2>/dev/null || echo unknown)}" && \
    DATE="${BUILD_DATE:-$(date -u +'%Y-%m-%dT%H:%M:%SZ')}" && \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X mihomo-sub-publisher/internal/generator.GeneratorVersion=1.1.0 -X mihomo-sub-publisher/internal/generator.GitCommit=${COMMIT} -X mihomo-sub-publisher/internal/generator.BuildDate=${DATE}" \
    -o /build/publisher ./cmd/publisher

# Final minimal runtime image
FROM alpine:3.21

WORKDIR /app

# Install ca-certificates and tzdata for TLS and timezone handling
RUN apk add --no-cache ca-certificates tzdata bash && \
    mkdir -p /app/configs /app/data /app/cache

COPY --from=builder /build/publisher /app/publisher

EXPOSE 8080

ENTRYPOINT ["/app/publisher"]
CMD ["-config", "/app/configs/config.yaml", "-tokens", "/app/configs/tokens.yaml"]
