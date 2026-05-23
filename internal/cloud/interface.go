package cloud

import (
	"context"
	"io"
)

// UploadInput holds all parameters required to store an object in cloud storage.
type UploadInput struct {
	Bucket      string
	Key         string            // object key / path within the bucket
	Body        io.Reader         // byte stream; must be closed by the caller after Upload returns
	ContentType string            // MIME type, e.g. "application/octet-stream"
	SizeBytes   int64             // total content length; -1 if unknown (disables multipart size hint)
	Metadata    map[string]string // provider-level object metadata (e.g. original file path)
}

// DownloadOutput contains the result of a successful cloud object download.
// The caller is responsible for closing Body.
type DownloadOutput struct {
	Body        io.ReadCloser
	ContentType string
	SizeBytes   int64
	ETag        string // provider-assigned content hash
}

// ObjectInfo is returned on successful uploads and Exists checks.
type ObjectInfo struct {
	Key       string
	Bucket    string
	SizeBytes int64
	ETag      string
}

// Provider is the abstraction layer over cloud object storage backends.
// All implementations must be safe for concurrent use from multiple goroutines.
type Provider interface {
	// Upload streams object data to the cloud backend.
	// For large objects, implementations should use multipart / resumable upload.
	Upload(ctx context.Context, input UploadInput) (ObjectInfo, error)

	// Download opens a streaming download of the specified object.
	// The caller must close DownloadOutput.Body when done reading.
	Download(ctx context.Context, bucket, key string) (DownloadOutput, error)

	// Delete permanently removes an object. Returns nil if the object did not exist.
	Delete(ctx context.Context, bucket, key string) error

	// Exists checks whether an object is present without downloading it.
	Exists(ctx context.Context, bucket, key string) (bool, error)

	// Name returns the human-readable provider identifier (e.g. "s3", "gcs").
	Name() string
}
