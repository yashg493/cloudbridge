package models

import (
	"time"

	"github.com/google/uuid"
)

// Protocol is the file-sharing protocol exposed by a namespace.
type Protocol string

const (
	ProtocolNFS Protocol = "nfs"
	ProtocolSMB Protocol = "smb"
)

// CloudBackend identifies the cloud object-storage backend for a namespace.
type CloudBackend string

const (
	CloudBackendS3   CloudBackend = "s3"
	CloudBackendGCS  CloudBackend = "gcs"
	CloudBackendNone CloudBackend = "none"
)

// ReplicationMode controls how CloudBridge moves bytes between source and cloud.
type ReplicationMode string

const (
	ReplicationModeSync   ReplicationMode = "sync"
	ReplicationModeAsync  ReplicationMode = "async"
	ReplicationModeTiered ReplicationMode = "tiered"
)

// NamespaceStatus reflects the operational health of a namespace.
type NamespaceStatus string

const (
	NamespaceStatusActive   NamespaceStatus = "active"
	NamespaceStatusDegraded NamespaceStatus = "degraded"
	NamespaceStatusOffline  NamespaceStatus = "offline"
)

// Namespace represents a storage share, analogous to an NFS export or SMB share.
type Namespace struct {
	ID              uuid.UUID       `json:"id"               db:"id"`
	Name            string          `json:"name"             db:"name"`
	Protocol        Protocol        `json:"protocol"         db:"protocol"`
	SourcePath      string          `json:"source_path"      db:"source_path"`
	CloudBackend    CloudBackend    `json:"cloud_backend"    db:"cloud_backend"`
	CloudBucket     string          `json:"cloud_bucket"     db:"cloud_bucket"`
	CloudPrefix     string          `json:"cloud_prefix"     db:"cloud_prefix"`
	ReplicationMode ReplicationMode `json:"replication_mode" db:"replication_mode"`
	Status          NamespaceStatus `json:"status"           db:"status"`
	NodeCount       int             `json:"node_count"       db:"node_count"`
	CreatedAt       time.Time       `json:"created_at"       db:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"       db:"updated_at"`
}
