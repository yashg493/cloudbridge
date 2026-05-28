# ── Stage 1: builder ─────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Cache dependencies before copying source (invalidated only when go.mod changes).
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
      -ldflags="-s -w" \
      -trimpath \
      -o cloudbridge \
      ./cmd/gateway

# ── Stage 2: final ───────────────────────────────────────────────────────────
# alpine:3.19 provides ca-certificates and a real shell while staying small.
FROM alpine:3.19

RUN apk add --no-cache ca-certificates

# Non-root user for least-privilege execution.
RUN adduser -D appuser

WORKDIR /app

# Binary from builder.
COPY --from=builder /app/cloudbridge ./cloudbridge

# Migration SQL is read at startup (MIGRATIONS_PATH defaults to migrations/001_init.sql).
COPY --from=builder /app/migrations/ ./migrations/

EXPOSE 8080

USER appuser

ENTRYPOINT ["./cloudbridge"]
