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

// SQL constants for NamespaceRepo. All queries use pgx named-arg (@name) syntax.
const (
	sqlNSCols = `id, name, protocol, source_path, cloud_backend, cloud_bucket,
	              cloud_prefix, replication_mode, status, node_count, created_at, updated_at`

	sqlNSCreate = `
		INSERT INTO namespaces
			(id, name, protocol, source_path, cloud_backend, cloud_bucket,
			 cloud_prefix, replication_mode, status, node_count, created_at, updated_at)
		VALUES
			(@id, @name, @protocol, @source_path, @cloud_backend, @cloud_bucket,
			 @cloud_prefix, @replication_mode, @status, @node_count, @created_at, @updated_at)`

	sqlNSGetByID = `SELECT ` + sqlNSCols + ` FROM namespaces WHERE id = @id`

	sqlNSGetByName = `SELECT ` + sqlNSCols + ` FROM namespaces WHERE name = @name`

	// Optional status filter: pass status="" to return all namespaces.
	sqlNSList = `
		SELECT ` + sqlNSCols + `
		FROM   namespaces
		WHERE  @status = '' OR status = @status
		ORDER  BY created_at DESC
		LIMIT  @limit OFFSET @offset`

	sqlNSCount = `
		SELECT COUNT(*) FROM namespaces
		WHERE  @status = '' OR status = @status`

	sqlNSUpdateStatus = `UPDATE namespaces SET status = @status WHERE id = @id`

	sqlNSIncrNodeCount = `
		UPDATE namespaces SET node_count = node_count + @delta WHERE id = @id`

	sqlNSDelete = `DELETE FROM namespaces WHERE id = @id`
)

// NamespaceRepo handles persistence of Namespace records.
type NamespaceRepo struct {
	pool *pgxpool.Pool
}

// NewNamespaceRepo creates a NamespaceRepo backed by pool.
func NewNamespaceRepo(pool *pgxpool.Pool) *NamespaceRepo {
	return &NamespaceRepo{pool: pool}
}

// Create inserts a new namespace. The caller must set ID and timestamps on ns.
func (r *NamespaceRepo) Create(ctx context.Context, ns *models.Namespace) error {
	_, err := r.pool.Exec(ctx, sqlNSCreate, pgx.NamedArgs{
		"id":               ns.ID,
		"name":             ns.Name,
		"protocol":         ns.Protocol,
		"source_path":      ns.SourcePath,
		"cloud_backend":    ns.CloudBackend,
		"cloud_bucket":     ns.CloudBucket,
		"cloud_prefix":     ns.CloudPrefix,
		"replication_mode": ns.ReplicationMode,
		"status":           ns.Status,
		"node_count":       ns.NodeCount,
		"created_at":       ns.CreatedAt,
		"updated_at":       ns.UpdatedAt,
	})
	if err != nil {
		return fmt.Errorf("namespace_repo.Create: %w", err)
	}
	return nil
}

// GetByID retrieves a namespace by primary key.
// Returns ErrNotFound if no row exists.
func (r *NamespaceRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Namespace, error) {
	rows, err := r.pool.Query(ctx, sqlNSGetByID, pgx.NamedArgs{"id": id})
	if err != nil {
		return nil, fmt.Errorf("namespace_repo.GetByID: %w", err)
	}
	ns, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[models.Namespace])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("namespace_repo.GetByID: %w", err)
	}
	return &ns, nil
}

// GetByName retrieves a namespace by its unique name.
// Returns ErrNotFound if no row exists.
func (r *NamespaceRepo) GetByName(ctx context.Context, name string) (*models.Namespace, error) {
	rows, err := r.pool.Query(ctx, sqlNSGetByName, pgx.NamedArgs{"name": name})
	if err != nil {
		return nil, fmt.Errorf("namespace_repo.GetByName: %w", err)
	}
	ns, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[models.Namespace])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("namespace_repo.GetByName: %w", err)
	}
	return &ns, nil
}

// List returns namespaces filtered by status with limit/offset pagination.
// Pass status="" to return all statuses.
// Returns the page slice and the total matching count (for pagination metadata).
func (r *NamespaceRepo) List(ctx context.Context, status string, limit, offset int) ([]*models.Namespace, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, sqlNSCount,
		pgx.NamedArgs{"status": status}).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("namespace_repo.List count: %w", err)
	}

	rows, err := r.pool.Query(ctx, sqlNSList, pgx.NamedArgs{
		"status": status,
		"limit":  limit,
		"offset": offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("namespace_repo.List: %w", err)
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.Namespace])
	if err != nil {
		return nil, 0, fmt.Errorf("namespace_repo.List: %w", err)
	}
	out := make([]*models.Namespace, len(items))
	for i := range items {
		out[i] = &items[i]
	}
	return out, total, nil
}

// UpdateStatus sets the status field for the given namespace.
// The updated_at column is managed by the database trigger.
func (r *NamespaceRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	_, err := r.pool.Exec(ctx, sqlNSUpdateStatus, pgx.NamedArgs{
		"id":     id,
		"status": status,
	})
	if err != nil {
		return fmt.Errorf("namespace_repo.UpdateStatus: %w", err)
	}
	return nil
}

// IncrementNodeCount atomically adjusts node_count by delta (may be negative).
func (r *NamespaceRepo) IncrementNodeCount(ctx context.Context, id uuid.UUID, delta int) error {
	_, err := r.pool.Exec(ctx, sqlNSIncrNodeCount, pgx.NamedArgs{
		"id":    id,
		"delta": delta,
	})
	if err != nil {
		return fmt.Errorf("namespace_repo.IncrementNodeCount: %w", err)
	}
	return nil
}

// Delete permanently removes a namespace record.
func (r *NamespaceRepo) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, sqlNSDelete, pgx.NamedArgs{"id": id})
	if err != nil {
		return fmt.Errorf("namespace_repo.Delete: %w", err)
	}
	return nil
}
