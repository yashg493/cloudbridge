# CloudBridge

A scale-out hybrid file storage gateway in Go — bridges on-premises NFS workloads
with cloud object storage (AWS S3 / GCS) via automated tiering, a Prometheus-instrumented
REST API, and a bounded goroutine worker pool.

Built as a portfolio demonstration for distributed file-system engineering
(targeting Nutanix Files / hybrid-cloud storage teams).

---

## Architecture

```
                        ┌──────────────────────────────┐
  NFS Clients / API ──► │   CloudBridge Gateway (Go)   │
                        │   Gin HTTP server             │
                        │   Worker Pool (goroutines)    │
                        └──────┬───────────────┬────────┘
                               │               │
                        ┌──────▼──────┐  ┌────▼──────────────┐
                        │ PostgreSQL  │  │  S3 / GCS          │
                        │ (metadata)  │  │  (warm & cold data)│
                        └─────────────┘  └────────────────────┘
```

**Tiering policy** (automatic, via Scheduler):
- `hot`  → `warm`  after **7 days** without access  (S3 Standard-IA)
- `warm` → `cold`  after **30 days** without access (S3 Glacier / archive)
- Manual recall via `POST /files/:id/tier` triggers immediate tier-down

---

## Stack

| Layer | Technology |
|-------|-----------|
| API | Go 1.22, Gin v1.9 |
| DB | PostgreSQL 16, pgx/v5 |
| Workers | Native goroutine pool |
| Cloud | AWS SDK v2 (S3), GCS stub |
| Observability | Prometheus, zap (structured JSON logs) |
| Container | Docker multi-stage (distroless runtime) |
| Orchestration | Kubernetes + HPA (autoscaling/v2) |

---

## Quick Start

```bash
# 1. Start Postgres + LocalStack S3 + Prometheus
docker compose up -d postgres localstack

# 2. Run the gateway locally (no Docker needed)
go run ./cmd/gateway

# 3. Check health
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz

# 4. View metrics
curl http://localhost:8080/metrics
```

---

## API Reference

### Namespaces

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/namespaces` | Create namespace |
| `GET` | `/api/v1/namespaces` | List namespaces |
| `GET` | `/api/v1/namespaces/:id` | Get namespace |
| `PUT` | `/api/v1/namespaces/:id` | Update namespace |
| `DELETE` | `/api/v1/namespaces/:id` | Delete namespace |

### Files

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/namespaces/:ns/files` | Register file metadata |
| `GET` | `/api/v1/namespaces/:ns/files` | List files (paginated) |
| `GET` | `/api/v1/namespaces/:ns/files/:id` | Get file metadata |
| `DELETE` | `/api/v1/namespaces/:ns/files/:id` | Soft-delete file |
| `POST` | `/api/v1/namespaces/:ns/files/:id/tier` | Trigger tier transition |

### Observability

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/healthz` | Liveness probe |
| `GET` | `/readyz` | Readiness probe (checks DB) |
| `GET` | `/metrics` | Prometheus metrics |

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_USER` | `cloudbridge` | DB username |
| `DB_PASSWORD` | `cloudbridge` | DB password |
| `DB_NAME` | `cloudbridge` | Database name |
| `HTTP_PORT` | `8080` | API listen port |
| `WORKER_COUNT` | `10` | Worker goroutines in pool |
| `WORKER_QUEUE_SIZE` | `100` | Job queue buffer depth |
| `CLOUD_PROVIDER` | `s3` | `s3` or `gcs` |
| `AWS_REGION` | `us-east-1` | AWS region |
| `S3_BUCKET` | `cloudbridge-local` | S3 bucket name |
| `S3_ENDPOINT` | _(empty)_ | Custom endpoint (LocalStack: `http://localstack:4566`) |

---

## Development

```bash
# Apply schema migrations
psql -U cloudbridge -d cloudbridge -f migrations/001_init.sql

# Run all tests
go test ./...

# Build binary
go build -o cloudbridge ./cmd/gateway

# Lint (requires golangci-lint)
golangci-lint run ./...
```

---

## Project Structure

```
cloudbridge/
├── cmd/gateway/main.go          # entrypoint, graceful shutdown
├── internal/
│   ├── api/                     # Gin router + handlers + middleware
│   ├── worker/                  # goroutine pool, sync jobs, scheduler
│   ├── store/                   # pgxpool wrapper + repositories
│   ├── models/                  # domain types (File, Namespace, SyncJob)
│   ├── nfs/                     # NFS operation simulator
│   ├── cloud/                   # Provider interface, S3 impl, GCS stub
│   └── metrics/                 # Prometheus registry
├── migrations/001_init.sql      # DDL: tables, indexes, constraints
├── k8s/                         # Deployment, Service, ConfigMap, HPA
├── Dockerfile                   # multi-stage build → distroless runtime
└── docker-compose.yml           # local dev: Postgres + LocalStack + Prometheus
```
