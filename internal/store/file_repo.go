package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/yashg493/cloudbridge/internal/models"
)

// FileRepository handles persistence of file metadata records.
type FileRepository struct {
	db     *DB
	logger *zap.Logger
}

// NewFileRepository creates a FileRepository backed by db.
func NewFileRepository(db *DB, logger *zap.Logger) *FileRepository {
	return &FileRepository{db: db, logger: logger}
}

// Create inserts a new file record. The caller must populate f.ID before calling.
func (r *FileRepository) Create(ctx context.Context, f *models.File) error {
	// TODO: INSERT INTO files (id, namespace_id, name, path, size_bytes, tier,
	//       status, cloud_key, checksum, created_at, updated_at, accessed_at)
	//       VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	//       using pgx named args or positional args; return pgconn.PgError on conflict
	return fmt.Errorf("not implemented")
}

// GetByID retrieves an active file by primary key. Returns ErrNotFound when absent.
func (r *FileRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.File, error) {
	// TODO: SELECT * FROM files WHERE id = $1 AND status != 'deleted'
	// TODO: pgx.CollectOneRow / pgx.RowToStructByName
	return nil, fmt.Errorf("not implemented")
}

// ListByNamespace returns files in a namespace ordered by created_at DESC with
// limit/offset pagination. Excludes soft-deleted records.
func (r *FileRepository) ListByNamespace(
	ctx context.Context,
	namespaceID uuid.UUID,
	limit, offset int,
) ([]*models.File, error) {
	// TODO: SELECT * FROM files
	//       WHERE namespace_id = $1 AND status != 'deleted'
	//       ORDER BY created_at DESC LIMIT $2 OFFSET $3
	// TODO: pgx.CollectRows / pgx.RowToStructByName
	return nil, fmt.Errorf("not implemented")
}

// Update persists changes to mutable fields on f. updated_at is set by the query.
func (r *FileRepository) Update(ctx context.Context, f *models.File) error {
	// TODO: UPDATE files SET name=$2, path=$3, size_bytes=$4, checksum=$5,
	//       cloud_key=$6, updated_at=NOW() WHERE id=$1
	return fmt.Errorf("not implemented")
}

// Delete soft-deletes a file by setting status='deleted' and updated_at=NOW().
func (r *FileRepository) Delete(ctx context.Context, id uuid.UUID) error {
	// TODO: UPDATE files SET status='deleted', updated_at=NOW() WHERE id=$1
	// TODO: return ErrNotFound if rows affected == 0
	return fmt.Errorf("not implemented")
}

// UpdateTier atomically updates the file's tier and cloud object key.
// cloudKey is nil when recalling a file back to hot tier.
func (r *FileRepository) UpdateTier(
	ctx context.Context,
	id uuid.UUID,
	tier models.TierType,
	cloudKey *string,
) error {
	// TODO: UPDATE files SET tier=$2, cloud_key=$3, status='active', updated_at=NOW()
	//       WHERE id=$1
	r.logger.Debug("UpdateTier called",
		zap.String("file_id", id.String()),
		zap.String("tier", string(tier)),
	)
	return fmt.Errorf("not implemented")
}

// TouchAccessedAt updates accessed_at for the given file without a full row update.
// Intended to be called asynchronously (fire-and-forget) on reads.
func (r *FileRepository) TouchAccessedAt(ctx context.Context, id uuid.UUID) error {
	// TODO: UPDATE files SET accessed_at=NOW() WHERE id=$1
	return fmt.Errorf("not implemented")
}
