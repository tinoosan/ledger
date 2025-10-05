# Multi-stage build for the ledger service
# Stage 1: build static binary
FROM golang:1.24.4-alpine AS builder

WORKDIR /src

# Cache dependencies
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source
COPY . .

# Build static binary (no CGO)
RUN --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags "-s -w" -o /out/ledger ./cmd


# Stage 2: minimal runtime image (nonroot)
FROM gcr.io/distroless/static:nonroot AS release

LABEL org.opencontainers.image.title="ledger" \
      org.opencontainers.image.description="Ledger service API" \
      org.opencontainers.image.source="https://github.com/tinoosan/ledger" \
      org.opencontainers.image.licenses="MIT"

ENV LOG_FORMAT=json \
    LOG_LEVEL=INFO \
    # Optional JWT auth (HS256). Leave empty to disable in the image; set via runtime envs.
    JWT_HS256_SECRET="" \
    JWT_ISSUER="" \
    JWT_AUDIENCE=""

COPY --from=builder /out/ledger /usr/local/bin/ledger

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/ledger"]

# Stage 3: dev/runtime image with shell and tools for troubleshooting
# Build with: docker build --target dev -t ledger:dev .
FROM alpine:3.20 AS dev

LABEL org.opencontainers.image.title="ledger (dev)" \
      org.opencontainers.image.description="Ledger service API - dev image with shell and troubleshooting tools" \
      org.opencontainers.image.source="https://github.com/tinoosan/ledger" \
      org.opencontainers.image.licenses="MIT"

# Add a non-root user and install useful tools (no secrets baked in)
RUN adduser -D -u 65532 app \
    && apk add --no-cache ca-certificates tzdata bash curl wget bind-tools iputils jq

ENV LOG_FORMAT=json \
    LOG_LEVEL=INFO \
    JWT_HS256_SECRET="" \
    JWT_ISSUER="" \
    JWT_AUDIENCE=""

COPY --from=builder /out/ledger /usr/local/bin/ledger

EXPOSE 8080
USER app
ENTRYPOINT ["/usr/local/bin/ledger"]
