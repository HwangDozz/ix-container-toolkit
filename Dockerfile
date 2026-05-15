# ─────────────────────────────────────────────────────────────────────────────
# DRA Driver Image
# Runs as a DaemonSet to publish ResourceSlices and handle NodePrepareResource.
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
    -o /out/accelerator-dra-driver ./cmd/accelerator-dra-driver

FROM debian:bookworm-slim

COPY --from=builder /out/accelerator-dra-driver /usr/local/bin/accelerator-dra-driver
COPY profiles /profiles

ENTRYPOINT ["/usr/local/bin/accelerator-dra-driver"]
