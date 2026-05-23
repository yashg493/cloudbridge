package models

import (
	"time"

	"github.com/google/uuid"
)

// SyncJobType classifies the background work a sync job performs.
type SyncJobType string

const (
	SyncJobTypeTierUp    SyncJobType = "tier_up"    // move file hot→warm or warm→cold
	SyncJobTypeTierDown  SyncJobType = "tier_down"  // recall file cold/warm→hot
	SyncJobTypeReplicate SyncJobType = "replicate"  // cross-region replication
	SyncJobTypeDelete    SyncJobType = "delete"     // remove cloud object after soft-delete
)

// SyncJobStatus tracks the execution state of a sync job.
type SyncJobStatus string

const (
	SyncJobStatusPending   SyncJobStatus = "pending"
	SyncJobStatusRunning   SyncJobStatus = "running"
	SyncJobStatusCompleted SyncJobStatus = "completed"
	SyncJobStatusFailed    SyncJobStatus = "failed"
)

// SyncJob represents a single unit of background work queued for the worker pool.
type SyncJob struct {
	ID          uuid.UUID     `json:"id"                    db:"id"`
	FileID      uuid.UUID     `json:"file_id"               db:"file_id"`
	Type        SyncJobType   `json:"type"                  db:"type"`
	Status      SyncJobStatus `json:"status"                db:"status"`
	Attempts    int           `json:"attempts"              db:"attempts"`
	ErrorMsg    *string       `json:"error_msg,omitempty"   db:"error_msg"`
	ScheduledAt time.Time     `json:"scheduled_at"          db:"scheduled_at"`
	StartedAt   *time.Time    `json:"started_at,omitempty"  db:"started_at"`
	CompletedAt *time.Time    `json:"completed_at,omitempty" db:"completed_at"`
	CreatedAt   time.Time     `json:"created_at"            db:"created_at"`
}
