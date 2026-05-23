package models

import (
	"time"

	"github.com/google/uuid"
)

// NamespaceStatus represents the lifecycle state of a namespace.
type NamespaceStatus string

const (
	NamespaceStatusActive   NamespaceStatus = "active"
	NamespaceStatusInactive NamespaceStatus = "inactive"
)

// Namespace represents a file share / mount point managed by CloudBridge.
// It acts as the top-level grouping for files, analogous to an NFS export or SMB share.
type Namespace struct {
	ID          uuid.UUID       `json:"id"                   db:"id"`
	Name        string          `json:"name"                 db:"name"`
	Description string          `json:"description"          db:"description"`
	MountPath   string          `json:"mount_path"           db:"mount_path"`
	Status      NamespaceStatus `json:"status"               db:"status"`
	QuotaBytes  *int64          `json:"quota_bytes,omitempty" db:"quota_bytes"` // nil = unlimited
	CreatedAt   time.Time       `json:"created_at"           db:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"           db:"updated_at"`
}
