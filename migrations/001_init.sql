-- CloudBridge initial schema
-- Apply with: psql -U cloudbridge -d cloudbridge -f migrations/001_init.sql
-- Also mounted as docker-entrypoint-initdb.d in docker-compose.

BEGIN;

-- Enable UUID generation
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ── namespaces ────────────────────────────────────────────────────────────────
-- Represents an NFS export / SMB share root. All files belong to a namespace.
CREATE TABLE IF NOT EXISTS namespaces (
    id          UUID        NOT NULL DEFAULT uuid_generate_v4(),
    name        VARCHAR(255) NOT NULL,
    description TEXT         NOT NULL DEFAULT '',
    mount_path  VARCHAR(512) NOT NULL,
    status      VARCHAR(16)  NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'inactive')),
    quota_bytes BIGINT,                        -- NULL = no quota enforced
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT namespaces_pkey       PRIMARY KEY (id),
    CONSTRAINT namespaces_name_uq    UNIQUE (name),
    CONSTRAINT namespaces_path_uq    UNIQUE (mount_path)
);

-- ── files ─────────────────────────────────────────────────────────────────────
-- Metadata record for every file known to CloudBridge.
-- Byte content is either on the local NFS mount (tier = hot) or in object
-- storage (tier = warm / cold), referenced by cloud_key.
CREATE TABLE IF NOT EXISTS files (
    id           UUID        NOT NULL DEFAULT uuid_generate_v4(),
    namespace_id UUID        NOT NULL,
    name         VARCHAR(512) NOT NULL,
    path         VARCHAR(1024) NOT NULL,        -- full path within namespace
    size_bytes   BIGINT       NOT NULL DEFAULT 0,
    tier         VARCHAR(8)   NOT NULL DEFAULT 'hot'
                     CHECK (tier IN ('hot', 'warm', 'cold')),
    status       VARCHAR(16)  NOT NULL DEFAULT 'active'
                     CHECK (status IN ('active', 'deleted', 'tiering')),
    cloud_key    TEXT,                          -- object key; NULL when tier = hot
    checksum     VARCHAR(128) NOT NULL DEFAULT '', -- SHA-256 hex
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    accessed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT files_pkey            PRIMARY KEY (id),
    CONSTRAINT files_ns_path_uq      UNIQUE (namespace_id, path),
    CONSTRAINT files_namespace_fk    FOREIGN KEY (namespace_id)
                                     REFERENCES namespaces (id)
                                     ON DELETE RESTRICT
);

-- ── sync_jobs ─────────────────────────────────────────────────────────────────
-- Work queue persisted in Postgres for durability across restarts.
-- The in-memory worker pool drains this table on startup and as jobs arrive.
CREATE TABLE IF NOT EXISTS sync_jobs (
    id           UUID        NOT NULL DEFAULT uuid_generate_v4(),
    file_id      UUID        NOT NULL,
    type         VARCHAR(16) NOT NULL
                     CHECK (type IN ('tier_up', 'tier_down', 'replicate', 'delete')),
    status       VARCHAR(16) NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    attempts     INT         NOT NULL DEFAULT 0,
    error_msg    TEXT,
    scheduled_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT sync_jobs_pkey     PRIMARY KEY (id),
    CONSTRAINT sync_jobs_file_fk  FOREIGN KEY (file_id)
                                  REFERENCES files (id)
                                  ON DELETE CASCADE
);

-- ── Indexes ───────────────────────────────────────────────────────────────────

-- Fast namespace membership queries
CREATE INDEX IF NOT EXISTS idx_files_namespace_id
    ON files (namespace_id);

-- Tiering scheduler: find hot/warm files by inactivity
CREATE INDEX IF NOT EXISTS idx_files_tier_accessed
    ON files (tier, accessed_at)
    WHERE status = 'active';

-- Tier distribution reporting
CREATE INDEX IF NOT EXISTS idx_files_tier_status
    ON files (tier, status);

-- Sync job worker: claim pending jobs
CREATE INDEX IF NOT EXISTS idx_sync_jobs_pending
    ON sync_jobs (scheduled_at)
    WHERE status = 'pending';

-- Job lookup by file
CREATE INDEX IF NOT EXISTS idx_sync_jobs_file_id
    ON sync_jobs (file_id);

COMMIT;
