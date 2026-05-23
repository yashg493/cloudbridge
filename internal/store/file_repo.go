package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yashg493/cloudbridge/internal/models"
)

// SQL constants for FileRepo. All queries use pgx named-arg (@name) syntax.
const (
	sqlFileCols = `id, namespace_id, path, size_bytes, checksum, tier,
	               access_count, last_accessed_at, cloud_synced, cloud_key, created_at, updated_at`

	// Upsert inserts a new record or, on conflict on (namespace_id, path), updates
	// all mutable fields. updated_at is managed by the database trigger on the UPDATE path.
	sqlFileUpsert = `
		INSERT INTO files
			(id, namespace_id, path, size_bytes, checksum, tier,
			 access_count, last_accessed_at, cloud_synced, cloud_key, created_at, updated_at)
		VALUES
			(@id, @namespace_id, @path, @size_bytes, @checksum, @tier,
			 @access_count, @last_accessed_at, @cloud_synced, @cloud_key, @created_at, @updated_at)
		ON CONFLICT (namespace_id, path) DO UPDATE SET
			size_bytes       = EXCLUDED.size_bytes,
			checksum         = EXCLUDED.checksum,
			tier             = EXCLUDED.tier,
			access_count     = EXCLUDED.access_count,
			last_accessed_at = EXCLUDED.last_accessed_at,
			cloud_synced     = EXCLUDED.cloud_synced,
			cloud_key        = EXCLUDED.cloud_key`

	sqlFileGetByID = `SELECT ` + sqlFileCols + ` FROM files WHERE id = @id`

	sqlFileGetByPath = `
		SELECT ` + sqlFileCols + `
		FROM   files
		WHERE  namespace_id = @namespace_id AND path = @path`

	sqlFileListByNS = `
		SELECT ` + sqlFileCols + `
		FROM   files
		WHERE  namespace_id = @namespace_id
		ORDER  BY path ASC
		LIMIT  @limit OFFSET @offset`

	sqlFileListByTier = `
		SELECT ` + sqlFileCols + `
		FROM   files
		WHERE  tier = @tier
		ORDER  BY last_accessed_at ASC
		LIMIT  @limit`

	// IncrementAccess is fire-and-forget safe; call from a background goroutine on reads.
	sqlFileIncrAccess = `
		UPDATE files
		SET    access_count     = access_count + 1,
		       last_accessed_at = NOW()
		WHERE  id = @file_id`

	sqlFileUpdateTier = `UPDATE files SET tier = @tier WHERE id = @file_id`

	sqlFileUpdateCloudSync = `
		UPDATE files
		SET    cloud_synced = TRUE,
		       cloud_key   = @cloud_key
		WHERE  id = @file_id`

	// GetColdFiles returns hot/warm candidates for cold-tier promotion.
	// cutoff is computed in Go to avoid PostgreSQL interval type mapping.
	sqlFileGetColdCandidates = `
		SELECT ` + sqlFileCols + `
		FROM   files
		WHERE  last_accessed_at < @cutoff
		  AND  tier != 'cold'
		ORDER  BY last_accessed_at ASC
		LIMIT  @limit`

	sqlFileCountByNS = `SELECT COUNT(*) FROM files WHERE namespace_id = @namespace_id`

	sqlFileTotalSizeByNS = `
		SELECT COALESCE(SUM(size_bytes), 0) FROM files WHERE namespace_id = @namespace_id`
)

// FileRepo handles persistence of FileMetadata records.
type FileRepo struct {
	pool *pgxpool.Pool
}

// NewFileRepo creates a FileRepo backed by pool.
func NewFileRepo(pool *pgxpool.Pool) *FileRepo {
	return &FileRepo{pool: pool}
}

// Upsert inserts a file record or, on conflict on (namespace_id, path), updates
// all mutable fields. The caller must set ID and timestamps on f.
func (r *FileRepo) Upsert(ctx context.Context, f *models.FileMetadata) error {
	_, err := r.pool.Exec(ctx, sqlFileUpsert, pgx.NamedArgs{
		"id":               f.ID,
		"namespace_id":     f.NamespaceID,
		"path":             f.Path,
		"size_bytes":       f.SizeBytes,
		"checksum":         f.Checksum,
		"tier":             f.Tier,
		"access_count":     f.AccessCount,
		"last_accessed_at": f.LastAccessedAt,
		"cloud_synced":     f.CloudSynced,
		"cloud_key":        f.CloudKey,
		"created_at":       f.CreatedAt,
		"updated_at":       f.UpdatedAt,
	})
	if err != nil {
		return fmt.Errorf("file_repo.Upsert: %w", err)
	}
	return nil
}

// GetByID retrieves a file by primary key.
// Returns ErrNotFound if no row exists.
func (r *FileRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.FileMetadata, error) {
	rows, err := r.pool.Query(ctx, sqlFileGetByID, pgx.NamedArgs{"id": id})
	if err != nil {
		return nil, fmt.Errorf("file_repo.GetByID: %w", err)
	}
	f, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[models.FileMetadata])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("file_repo.GetByID: %w", err)
	}
	return &f, nil
}

// GetByPath retrieves a file by namespace + relative path.
// Returns ErrNotFound if no row exists.
func (r *FileRepo) GetByPath(ctx context.Context, namespaceID uuid.UUID, path string) (*models.FileMetadata, error) {
	rows, err := r.pool.Query(ctx, sqlFileGetByPath, pgx.NamedArgs{
		"namespace_id": namespaceID,
		"path":         path,
	})
	if err != nil {
		return nil, fmt.Errorf("file_repo.GetByPath: %w", err)
	}
	f, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[models.FileMetadata])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("file_repo.GetByPath: %w", err)
	}
	return &f, nil
}

// ListByNamespace returns files in a namespace ordered by path with pagination.
func (r *FileRepo) ListByNamespace(ctx context.Context, namespaceID uuid.UUID, limit, offset int) ([]*models.FileMetadata, error) {
	rows, err := r.pool.Query(ctx, sqlFileListByNS, pgx.NamedArgs{
		"namespace_id": namespaceID,
		"limit":        limit,
		"offset":       offset,
	})
	if err != nil {
		return nil, fmt.Errorf("file_repo.ListByNamespace: %w", err)
	}
	return collectFileRows(rows)
}

// ListByTier returns the oldest-accessed files in a given tier, up to limit rows.
func (r *FileRepo) ListByTier(ctx context.Context, tier string, limit int) ([]*models.FileMetadata, error) {
	rows, err := r.pool.Query(ctx, sqlFileListByTier, pgx.NamedArgs{
		"tier":  tier,
		"limit": limit,
	})
	if err != nil {
		return nil, fmt.Errorf("file_repo.ListByTier: %w", err)
	}
	return collectFileRows(rows)
}

// IncrementAccess atomically increments access_count and sets last_accessed_at = NOW().
// Safe to call as fire-and-forget from a background goroutine.
func (r *FileRepo) IncrementAccess(ctx context.Context, fileID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, sqlFileIncrAccess, pgx.NamedArgs{"file_id": fileID})
	if err != nil {
		return fmt.Errorf("file_repo.IncrementAccess: %w", err)
	}
	return nil
}

// UpdateTier sets the storage tier for a file.
func (r *FileRepo) UpdateTier(ctx context.Context, fileID uuid.UUID, tier string) error {
	_, err := r.pool.Exec(ctx, sqlFileUpdateTier, pgx.NamedArgs{
		"file_id": fileID,
		"tier":    tier,
	})
	if err != nil {
		return fmt.Errorf("file_repo.UpdateTier: %w", err)
	}
	return nil
}

// UpdateCloudSync marks a file as synced and records its cloud object key.
func (r *FileRepo) UpdateCloudSync(ctx context.Context, fileID uuid.UUID, cloudKey string) error {
	_, err := r.pool.Exec(ctx, sqlFileUpdateCloudSync, pgx.NamedArgs{
		"file_id":   fileID,
		"cloud_key": cloudKey,
	})
	if err != nil {
		return fmt.Errorf("file_repo.UpdateCloudSync: %w", err)
	}
	return nil
}

// GetColdFiles returns hot/warm files not accessed since olderThan ago,
// ordered by last_accessed_at ascending (coldest candidates first).
// Used by the tiering scheduler to find promotion candidates.
func (r *FileRepo) GetColdFiles(ctx context.Context, olderThan time.Duration, limit int) ([]*models.FileMetadata, error) {
	cutoff := time.Now().Add(-olderThan)
	rows, err := r.pool.Query(ctx, sqlFileGetColdCandidates, pgx.NamedArgs{
		"cutoff": cutoff,
		"limit":  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("file_repo.GetColdFiles: %w", err)
	}
	return collectFileRows(rows)
}

// CountByNamespace returns the total number of files in a namespace.
func (r *FileRepo) CountByNamespace(ctx context.Context, namespaceID uuid.UUID) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, sqlFileCountByNS,
		pgx.NamedArgs{"namespace_id": namespaceID}).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("file_repo.CountByNamespace: %w", err)
	}
	return n, nil
}

// TotalSizeByNamespace returns the sum of size_bytes for all files in a namespace.
// Returns 0 when the namespace contains no files.
func (r *FileRepo) TotalSizeByNamespace(ctx context.Context, namespaceID uuid.UUID) (int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx, sqlFileTotalSizeByNS,
		pgx.NamedArgs{"namespace_id": namespaceID}).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("file_repo.TotalSizeByNamespace: %w", err)
	}
	return total, nil
}

// collectFileRows drains pgx.Rows into a []*models.FileMetadata slice.
func collectFileRows(rows pgx.Rows) ([]*models.FileMetadata, error) {
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.FileMetadata])
	if err != nil {
		return nil, err
	}
	out := make([]*models.FileMetadata, len(items))
	for i := range items {
		out[i] = &items[i]
	}
	return out, nil
}
