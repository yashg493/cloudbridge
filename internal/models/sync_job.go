package models

import (
	"time"

	"github.com/google/uuid"
)

// SyncOperation classifies the cloud/file operation performed by a sync job.
type SyncOperation string

const (
	SyncOperationUpload   SyncOperation = "upload"
	SyncOperationDownload SyncOperation = "download"
	SyncOperationDelete   SyncOperation = "delete"
	SyncOperationTierMove SyncOperation = "tier_move"
)

// SyncStatus tracks the execution lifecycle of a sync job.
type SyncStatus string

const (
	SyncStatusPending   SyncStatus = "pending"
	SyncStatusQueued    SyncStatus = "queued"
	SyncStatusRunning   SyncStatus = "running"
	SyncStatusCompleted SyncStatus = "completed"
	SyncStatusFailed    SyncStatus = "failed"
	SyncStatusCancelled SyncStatus = "cancelled"
)

// SyncJob tracks file replication and tiering operations.
type SyncJob struct {
	ID               uuid.UUID     `json:"id"                db:"id"`
	NamespaceID      uuid.UUID     `json:"namespace_id"      db:"namespace_id"`
	FileID           uuid.UUID     `json:"file_id"           db:"file_id"`
	Operation        SyncOperation `json:"operation"         db:"operation"`
	Status           SyncStatus    `json:"status"            db:"status"`
	RetryCount       int           `json:"retry_count"       db:"retry_count"`
	ErrorMessage     string        `json:"error_message"     db:"error_message"`
	BytesTransferred int64         `json:"bytes_transferred" db:"bytes_transferred"`
	StartedAt        *time.Time    `json:"started_at"        db:"started_at"`
	CompletedAt      *time.Time    `json:"completed_at"      db:"completed_at"`
	CreatedAt        time.Time     `json:"created_at"        db:"created_at"`
}
