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
FROM gcr.io/distroless/static:nonroot

LABEL org.opencontainers.image.title="ledger" \
      org.opencontainers.image.description="Ledger service API" \
      org.opencontainers.image.source="https://github.com/tinoosan/ledger" \
      org.opencontainers.image.licenses="MIT"

ENV LOG_FORMAT=json \
    LOG_LEVEL=INFO

COPY --from=builder /out/ledger /usr/local/bin/ledger

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/ledger"]

