-- CloudBridge schema — migration 001
-- Apply with: psql -U cloudbridge -d cloudbridge -f migrations/001_init.sql
-- Also auto-applied via docker-entrypoint-initdb.d in docker-compose.

BEGIN;

-- ── updated_at trigger ────────────────────────────────────────────────────────
-- Single reusable function; attached to every table that carries updated_at.
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;

-- ── namespaces ────────────────────────────────────────────────────────────────
-- A Namespace maps to one NFS export or SMB share. It owns a set of files and
-- defines how they are replicated to cloud object storage.
CREATE TABLE IF NOT EXISTS namespaces (
    id               UUID         NOT NULL DEFAULT gen_random_uuid(),
    name             VARCHAR(255) NOT NULL,
    protocol         VARCHAR(8)   NOT NULL
                         CHECK (protocol IN ('nfs', 'smb')),
    source_path      TEXT         NOT NULL,           -- on-prem mount point
    cloud_backend    VARCHAR(8)   NOT NULL DEFAULT 'none'
                         CHECK (cloud_backend IN ('s3', 'gcs', 'none')),
    cloud_bucket     TEXT         NOT NULL DEFAULT '',
    cloud_prefix     TEXT         NOT NULL DEFAULT '',
    replication_mode VARCHAR(8)   NOT NULL DEFAULT 'async'
                         CHECK (replication_mode IN ('sync', 'async', 'tiered')),
    status           VARCHAR(16)  NOT NULL DEFAULT 'active'
                         CHECK (status IN ('active', 'degraded', 'offline')),
    node_count       INT          NOT NULL DEFAULT 1 CHECK (node_count >= 0),
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    CONSTRAINT namespaces_pkey    PRIMARY KEY (id),
    CONSTRAINT namespaces_name_uq UNIQUE (name)
);

CREATE TRIGGER trg_namespaces_updated_at
    BEFORE UPDATE ON namespaces
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ── files ─────────────────────────────────────────────────────────────────────
-- FileMetadata record for every file in the system.
-- Byte content lives on the local NFS mount (tier=hot) or in object storage
-- (tier=warm/cold), referenced by cloud_key.
CREATE TABLE IF NOT EXISTS files (
    id               UUID         NOT NULL DEFAULT gen_random_uuid(),
    namespace_id     UUID         NOT NULL,
    path             TEXT         NOT NULL,  -- relative path within the namespace root
    size_bytes       BIGINT       NOT NULL DEFAULT 0,
    checksum         VARCHAR(64)  NOT NULL DEFAULT '', -- SHA-256 hex (64 chars)
    tier             VARCHAR(8)   NOT NULL DEFAULT 'hot'
                         CHECK (tier IN ('hot', 'warm', 'cold')),
    access_count     BIGINT       NOT NULL DEFAULT 0,
    last_accessed_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    cloud_synced     BOOLEAN      NOT NULL DEFAULT FALSE,
    cloud_key        TEXT         NOT NULL DEFAULT '',  -- empty when tier = hot
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    CONSTRAINT files_pkey         PRIMARY KEY (id),
    CONSTRAINT files_ns_path_uq   UNIQUE (namespace_id, path),
    CONSTRAINT files_namespace_fk FOREIGN KEY (namespace_id)
                                  REFERENCES namespaces (id)
                                  ON DELETE RESTRICT
);

CREATE TRIGGER trg_files_updated_at
    BEFORE UPDATE ON files
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ── sync_jobs ─────────────────────────────────────────────────────────────────
-- Durable work queue for cloud replication and tiering operations.
-- The in-memory worker pool polls this table; Postgres acts as the broker.
CREATE TABLE IF NOT EXISTS sync_jobs (
    id                UUID         NOT NULL DEFAULT gen_random_uuid(),
    namespace_id      UUID         NOT NULL,
    file_id           UUID         NOT NULL,
    operation         VARCHAR(16)  NOT NULL
                          CHECK (operation IN ('upload', 'download', 'delete', 'tier_move')),
    status            VARCHAR(16)  NOT NULL DEFAULT 'pending'
                          CHECK (status IN ('pending', 'queued', 'running', 'completed', 'failed', 'cancelled')),
    retry_count       INT          NOT NULL DEFAULT 0,
    error_message     TEXT         NOT NULL DEFAULT '',
    bytes_transferred BIGINT       NOT NULL DEFAULT 0,
    started_at        TIMESTAMPTZ,           -- NULL until a worker claims the job
    completed_at      TIMESTAMPTZ,           -- NULL until terminal state
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    CONSTRAINT sync_jobs_pkey        PRIMARY KEY (id),
    CONSTRAINT sync_jobs_namespace_fk FOREIGN KEY (namespace_id)
                                      REFERENCES namespaces (id)
                                      ON DELETE CASCADE,
    CONSTRAINT sync_jobs_file_fk      FOREIGN KEY (file_id)
                                      REFERENCES files (id)
                                      ON DELETE CASCADE
);

-- ── Indexes ───────────────────────────────────────────────────────────────────

-- Fast path look-up: resolve a file by namespace + relative path.
-- Covers the UNIQUE constraint and point-lookup queries.
CREATE INDEX IF NOT EXISTS idx_files_namespace_path
    ON files (namespace_id, path);

-- Tiering scheduler: scan hot/warm files that haven't been accessed recently.
CREATE INDEX IF NOT EXISTS idx_files_tier_last_accessed
    ON files (tier, last_accessed_at);

-- Worker polling: claim the next batch of pending/queued jobs ordered by age.
CREATE INDEX IF NOT EXISTS idx_sync_jobs_status_created
    ON sync_jobs (status, created_at)
    WHERE status IN ('pending', 'queued');

-- Per-namespace job monitoring and cancellation.
CREATE INDEX IF NOT EXISTS idx_sync_jobs_namespace_status
    ON sync_jobs (namespace_id, status);

COMMIT;
