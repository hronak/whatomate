# Frontend build stage — pinned to BUILDPLATFORM (output is arch-agnostic JS).
FROM --platform=$BUILDPLATFORM node:24-alpine AS frontend-builder

WORKDIR /app/frontend

# Copy frontend dependency files first for caching
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

# Copy frontend source and build
COPY frontend/ .
RUN npm run build

# Go build stage — runs on BUILDPLATFORM, cross-compiles to TARGETOS/TARGETARCH
# so multi-arch builds don't emulate the Go toolchain under QEMU.
FROM --platform=$BUILDPLATFORM golang:1.26.6-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Embed frontend build into Go binary
COPY --from=frontend-builder /app/frontend/dist/ ./internal/frontend/dist/

# Build the binary
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -a -installsuffix cgo -o whatomate ./cmd/whatomate

# Final stage (Debian for glibc)
FROM debian:bookworm-slim

WORKDIR /app

# Install runtime dependencies (transcoding: ffmpeg)
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates tzdata ffmpeg \
    && rm -rf /var/lib/apt/lists/*

# Copy binary from builder
COPY --from=builder /app/whatomate .

# Copy config example (will be overridden by env vars in production)
COPY --from=builder /app/config.example.toml ./config.toml



# Create directories
RUN mkdir -p /app/uploads /app/audio

# Expose port
EXPOSE 8080

# Match the GoReleaser image's launch convention so the same orchestrator
# args work against both tags: the binary is the ENTRYPOINT and CMD holds
# only the default subcommand/flags. A Nomad job that sets
# `args = ["server", "-migrate", ...]` (no `command`) then appends to the
# entrypoint instead of replacing the binary with "server".
ENTRYPOINT ["./whatomate"]
CMD ["server", "-config", "config.toml", "-migrate"]
