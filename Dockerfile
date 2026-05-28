# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

# Required for CGO_ENABLED=0 static binaries + module downloads
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# Layer caching: download deps before copying source
COPY go.mod go.sum ./
RUN go mod download

# Build a fully-static binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
      -ldflags="-w -s -extldflags '-static'" \
      -trimpath \
      -o /cloudbridge \
      ./cmd/gateway

# ── Runtime stage ─────────────────────────────────────────────────────────────
# distroless/static has no shell, no package manager — minimal attack surface.
FROM gcr.io/distroless/static-debian12:nonroot AS runtime

# Copy timezone data and CA certs from builder (needed for TLS and time functions)
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

COPY --from=builder /cloudbridge /cloudbridge
# Migrations are read at startup via MIGRATIONS_PATH env var (default: /migrations/001_init.sql)
COPY --from=builder /build/migrations/ /migrations/

EXPOSE 8080

ENTRYPOINT ["/cloudbridge"]
