package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/yashg493/cloudbridge/internal/models"
)

// NamespaceRepository handles persistence of namespace records.
type NamespaceRepository struct {
	db     *DB
	logger *zap.Logger
}

// NewNamespaceRepository creates a NamespaceRepository backed by db.
func NewNamespaceRepository(db *DB, logger *zap.Logger) *NamespaceRepository {
	return &NamespaceRepository{db: db, logger: logger}
}

// Create inserts a new namespace record. The caller must set ns.ID before calling.
func (r *NamespaceRepository) Create(ctx context.Context, ns *models.Namespace) error {
	// TODO: INSERT INTO namespaces (id, name, description, mount_path, status,
	//       quota_bytes, created_at, updated_at) VALUES (...)
	// TODO: return wrapped pgconn.PgError on unique constraint violation (name, mount_path)
	return fmt.Errorf("not implemented")
}

// GetByID retrieves a namespace by primary key regardless of status.
func (r *NamespaceRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Namespace, error) {
	// TODO: SELECT * FROM namespaces WHERE id = $1
	// TODO: pgx.CollectOneRow / pgx.RowToStructByName
	r.logger.Debug("GetByID called", zap.String("namespace_id", id.String()))
	return nil, fmt.Errorf("not implemented")
}

// List returns all active namespaces ordered by created_at DESC.
func (r *NamespaceRepository) List(ctx context.Context) ([]*models.Namespace, error) {
	// TODO: SELECT * FROM namespaces WHERE status = 'active' ORDER BY created_at DESC
	// TODO: pgx.CollectRows / pgx.RowToStructByName
	return nil, fmt.Errorf("not implemented")
}

// Update persists changes to mutable namespace fields. updated_at is set by the query.
func (r *NamespaceRepository) Update(ctx context.Context, ns *models.Namespace) error {
	// TODO: UPDATE namespaces SET name=$2, description=$3, mount_path=$4,
	//       quota_bytes=$5, updated_at=NOW() WHERE id=$1
	return fmt.Errorf("not implemented")
}

// Delete soft-deletes a namespace by setting status='inactive'.
// Returns an error if active files still reference this namespace.
func (r *NamespaceRepository) Delete(ctx context.Context, id uuid.UUID) error {
	// TODO: check for active files: SELECT COUNT(*) FROM files
	//       WHERE namespace_id=$1 AND status='active'
	// TODO: if count > 0 return ErrNamespaceNotEmpty sentinel
	// TODO: UPDATE namespaces SET status='inactive', updated_at=NOW() WHERE id=$1
	return fmt.Errorf("not implemented")
}
