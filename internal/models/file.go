package models

import (
	"time"

	"github.com/google/uuid"
)

// TierType is the storage tier a file currently resides in.
type TierType string

const (
	TierHot  TierType = "hot"
	TierWarm TierType = "warm"
	TierCold TierType = "cold"
)

// FileMetadata tracks every file in the CloudBridge namespace index.
type FileMetadata struct {
	ID             uuid.UUID `json:"id"               db:"id"`
	NamespaceID    uuid.UUID `json:"namespace_id"     db:"namespace_id"`
	Path           string    `json:"path"             db:"path"`
	SizeBytes      int64     `json:"size_bytes"       db:"size_bytes"`
	Checksum       string    `json:"checksum"         db:"checksum"`
	Tier           TierType  `json:"tier"             db:"tier"`
	AccessCount    int64     `json:"access_count"     db:"access_count"`
	LastAccessedAt time.Time `json:"last_accessed_at" db:"last_accessed_at"`
	CloudSynced    bool      `json:"cloud_synced"     db:"cloud_synced"`
	CloudKey       string    `json:"cloud_key"        db:"cloud_key"`
	CreatedAt      time.Time `json:"created_at"       db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"       db:"updated_at"`
}

// File is kept as a package-level alias while the scaffolded repository code is
// incrementally migrated to the FileMetadata name.
type File = FileMetadata
