# ─────────────────────────────────────────────────────────────────────────────
# Stage 1: Build
# ─────────────────────────────────────────────────────────────────────────────
FROM golang:1.22-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" \
    -o /out/ix-container-runtime ./cmd/ix-container-runtime && \
    go build -trimpath -ldflags="-s -w" \
    -o /out/ix-container-hook    ./cmd/ix-container-hook && \
    go build -trimpath -ldflags="-s -w" \
    -o /out/ix-installer         ./cmd/ix-installer

# ─────────────────────────────────────────────────────────────────────────────
# Stage 2: Installer image
# Runs as an init container to copy binaries and patch containerd config.
# ─────────────────────────────────────────────────────────────────────────────
FROM debian:bookworm-slim AS installer

COPY --from=builder /out/ix-container-runtime /usr/local/bin/ix-container-runtime
COPY --from=builder /out/ix-container-hook    /usr/local/bin/ix-container-hook
COPY --from=builder /out/ix-installer         /usr/local/bin/ix-installer

ENTRYPOINT ["/usr/local/bin/ix-installer"]
