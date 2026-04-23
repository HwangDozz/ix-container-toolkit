# ─────────────────────────────────────────────────────────────────────────────
# Stage 1: Build
# ─────────────────────────────────────────────────────────────────────────────
FROM golang:1.26.1-bookworm AS builder

ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 \
    go build -trimpath -ldflags="-s -w" \
    -o /out/accelerator-container-runtime ./cmd/accelerator-container-runtime && \
    go build -trimpath -ldflags="-s -w" \
    -o /out/accelerator-container-hook    ./cmd/accelerator-container-hook && \
    go build -trimpath -ldflags="-s -w" \
    -o /out/accelerator-installer         ./cmd/accelerator-installer

# ─────────────────────────────────────────────────────────────────────────────
# Stage 2: Installer image
# Runs as an init container to copy binaries and patch containerd config.
# ─────────────────────────────────────────────────────────────────────────────
FROM debian:bookworm-slim AS installer

COPY --from=builder /out/accelerator-container-runtime /usr/local/bin/accelerator-container-runtime
COPY --from=builder /out/accelerator-container-hook    /usr/local/bin/accelerator-container-hook
COPY --from=builder /out/accelerator-installer         /usr/local/bin/accelerator-installer
COPY profiles /profiles

ENTRYPOINT ["/usr/local/bin/accelerator-installer"]
