package models

import (
	"time"

	"github.com/google/uuid"
)

// TierType represents the storage tier a file currently resides in.
type TierType string

const (
	TierHot  TierType = "hot"  // local NFS, immediate access
	TierWarm TierType = "warm" // object storage, infrequent access (>7 days)
	TierCold TierType = "cold" // archival storage, rare access (>30 days)
)

// FileStatus represents the lifecycle state of a file record.
type FileStatus string

const (
	FileStatusActive  FileStatus = "active"
	FileStatusDeleted FileStatus = "deleted"
	FileStatusTiering FileStatus = "tiering" // transition in progress
)

// File represents the metadata record for a file managed by CloudBridge.
// Actual byte content lives either on local NFS (hot) or cloud object storage (warm/cold).
type File struct {
	ID          uuid.UUID  `json:"id"                    db:"id"`
	NamespaceID uuid.UUID  `json:"namespace_id"          db:"namespace_id"`
	Name        string     `json:"name"                  db:"name"`
	Path        string     `json:"path"                  db:"path"`
	SizeBytes   int64      `json:"size_bytes"            db:"size_bytes"`
	Tier        TierType   `json:"tier"                  db:"tier"`
	Status      FileStatus `json:"status"                db:"status"`
	CloudKey    *string    `json:"cloud_key,omitempty"   db:"cloud_key"` // set when tier != hot
	Checksum    string     `json:"checksum"              db:"checksum"`  // SHA-256 hex
	CreatedAt   time.Time  `json:"created_at"            db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"            db:"updated_at"`
	AccessedAt  time.Time  `json:"accessed_at"           db:"accessed_at"`
}
