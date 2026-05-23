package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yashg493/cloudbridge/internal/models"
)

// SQL constants for SyncJobRepo. All queries use pgx named-arg (@name) syntax.
const (
	sqlSJCols = `id, namespace_id, file_id, operation, status, retry_count,
	              error_message, bytes_transferred, started_at, completed_at, created_at`

	sqlSJCreate = `
		INSERT INTO sync_jobs
			(id, namespace_id, file_id, operation, status, retry_count,
			 error_message, bytes_transferred, started_at, completed_at, created_at)
		VALUES
			(@id, @namespace_id, @file_id, @operation, @status, @retry_count,
			 @error_message, @bytes_transferred, @started_at, @completed_at, @created_at)`

	sqlSJGetByID = `SELECT ` + sqlSJCols + ` FROM sync_jobs WHERE id = @id`

	// UpdateStatus transitions the job status and auto-manages timing columns:
	//   started_at  → set to NOW() on the first transition to 'running'
	//   completed_at → set to NOW() when reaching any terminal state
	sqlSJUpdateStatus = `
		UPDATE sync_jobs
		SET
			status        = @status,
			error_message = @error_message,
			started_at    = CASE
			                    WHEN @status = 'running' AND started_at IS NULL
			                    THEN NOW()
			                    ELSE started_at
			                END,
			completed_at  = CASE
			                    WHEN @status = 'completed'
			                      OR @status = 'failed'
			                      OR @status = 'cancelled'
			                    THEN NOW()
			                    ELSE completed_at
			                END
		WHERE id = @id`

	sqlSJUpdateProgress = `
		UPDATE sync_jobs SET bytes_transferred = @bytes_transferred WHERE id = @id`

	// PollPending atomically claims up to @limit pending jobs and transitions them
	// to 'queued'. The inner SELECT uses FOR UPDATE SKIP LOCKED so concurrent workers
	// never claim the same row. The CTE is a single statement — no explicit transaction needed.
	sqlSJPollPending = `
		WITH claimed AS (
			UPDATE sync_jobs
			SET    status = 'queued'
			WHERE  id IN (
				SELECT id
				FROM   sync_jobs
				WHERE  status = 'pending'
				ORDER  BY created_at ASC
				LIMIT  @limit
				FOR    UPDATE SKIP LOCKED
			)
			RETURNING ` + sqlSJCols + `
		)
		SELECT ` + sqlSJCols + ` FROM claimed ORDER BY created_at ASC`

	// Optional status filter: pass status="" to return all statuses.
	sqlSJListByNS = `
		SELECT ` + sqlSJCols + `
		FROM   sync_jobs
		WHERE  namespace_id = @namespace_id
		  AND  (@status = '' OR status = @status)
		ORDER  BY created_at DESC
		LIMIT  @limit OFFSET @offset`

	sqlSJCountByStatus = `SELECT status, COUNT(*) FROM sync_jobs GROUP BY status`
)

// SyncJobRepo handles persistence of SyncJob records.
type SyncJobRepo struct {
	pool *pgxpool.Pool
}

// NewSyncJobRepo creates a SyncJobRepo backed by pool.
func NewSyncJobRepo(pool *pgxpool.Pool) *SyncJobRepo {
	return &SyncJobRepo{pool: pool}
}

// Create inserts a new sync job. The caller must set ID and CreatedAt on j.
func (r *SyncJobRepo) Create(ctx context.Context, j *models.SyncJob) error {
	_, err := r.pool.Exec(ctx, sqlSJCreate, pgx.NamedArgs{
		"id":                j.ID,
		"namespace_id":      j.NamespaceID,
		"file_id":           j.FileID,
		"operation":         j.Operation,
		"status":            j.Status,
		"retry_count":       j.RetryCount,
		"error_message":     j.ErrorMessage,
		"bytes_transferred": j.BytesTransferred,
		"started_at":        j.StartedAt,
		"completed_at":      j.CompletedAt,
		"created_at":        j.CreatedAt,
	})
	if err != nil {
		return fmt.Errorf("sync_job_repo.Create: %w", err)
	}
	return nil
}

// GetByID retrieves a sync job by primary key.
// Returns ErrNotFound if no row exists.
func (r *SyncJobRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.SyncJob, error) {
	rows, err := r.pool.Query(ctx, sqlSJGetByID, pgx.NamedArgs{"id": id})
	if err != nil {
		return nil, fmt.Errorf("sync_job_repo.GetByID: %w", err)
	}
	j, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[models.SyncJob])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("sync_job_repo.GetByID: %w", err)
	}
	return &j, nil
}

// UpdateStatus transitions the job to a new status and persists errMsg to error_message.
// Pass errMsg="" when there is no error. The timing columns started_at and completed_at
// are managed automatically by the SQL CASE expressions.
func (r *SyncJobRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status, errMsg string) error {
	_, err := r.pool.Exec(ctx, sqlSJUpdateStatus, pgx.NamedArgs{
		"id":            id,
		"status":        status,
		"error_message": errMsg,
	})
	if err != nil {
		return fmt.Errorf("sync_job_repo.UpdateStatus: %w", err)
	}
	return nil
}

// UpdateProgress records the number of bytes transferred so far for a running job.
func (r *SyncJobRepo) UpdateProgress(ctx context.Context, id uuid.UUID, bytesTransferred int64) error {
	_, err := r.pool.Exec(ctx, sqlSJUpdateProgress, pgx.NamedArgs{
		"id":                id,
		"bytes_transferred": bytesTransferred,
	})
	if err != nil {
		return fmt.Errorf("sync_job_repo.UpdateProgress: %w", err)
	}
	return nil
}

// PollPending atomically claims up to limit pending sync jobs, transitions them
// to "queued", and returns them ordered by created_at. The SELECT FOR UPDATE SKIP LOCKED
// inside the CTE guarantees concurrent-safe claiming with no duplicate assignments.
func (r *SyncJobRepo) PollPending(ctx context.Context, limit int) ([]*models.SyncJob, error) {
	rows, err := r.pool.Query(ctx, sqlSJPollPending, pgx.NamedArgs{"limit": limit})
	if err != nil {
		return nil, fmt.Errorf("sync_job_repo.PollPending: %w", err)
	}
	return collectJobRows(rows)
}

// ListByNamespace returns sync jobs for a namespace, optionally filtered by status.
// Pass status="" to return all statuses. Results are ordered newest-first.
func (r *SyncJobRepo) ListByNamespace(
	ctx context.Context,
	namespaceID uuid.UUID,
	status string,
	limit, offset int,
) ([]*models.SyncJob, error) {
	rows, err := r.pool.Query(ctx, sqlSJListByNS, pgx.NamedArgs{
		"namespace_id": namespaceID,
		"status":       status,
		"limit":        limit,
		"offset":       offset,
	})
	if err != nil {
		return nil, fmt.Errorf("sync_job_repo.ListByNamespace: %w", err)
	}
	return collectJobRows(rows)
}

// CountByStatus returns a map of status → job count for all sync jobs.
// Intended for publishing Prometheus gauges.
func (r *SyncJobRepo) CountByStatus(ctx context.Context) (map[string]int64, error) {
	rows, err := r.pool.Query(ctx, sqlSJCountByStatus)
	if err != nil {
		return nil, fmt.Errorf("sync_job_repo.CountByStatus: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("sync_job_repo.CountByStatus scan: %w", err)
		}
		counts[status] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sync_job_repo.CountByStatus: %w", err)
	}
	return counts, nil
}

// collectJobRows drains pgx.Rows into a []*models.SyncJob slice.
func collectJobRows(rows pgx.Rows) ([]*models.SyncJob, error) {
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.SyncJob])
	if err != nil {
		return nil, err
	}
	out := make([]*models.SyncJob, len(items))
	for i := range items {
		out[i] = &items[i]
	}
	return out, nil
}
