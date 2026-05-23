package cloud

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrNotImplemented is returned by cloud provider methods that are not yet built.
var ErrNotImplemented = errors.New("cloud: operation not implemented")

// CloudProvider is the abstraction over cloud object-storage backends.
// The storage bucket is configured at provider construction time, so callers
// only pass the object key. All implementations must be safe for concurrent use.
type CloudProvider interface {
	// Upload streams data to the cloud backend under key.
	// sizeBytes is a hint for Content-Length; pass -1 if the size is unknown.
	Upload(ctx context.Context, key string, data io.Reader, sizeBytes int64) error

	// Download opens a streaming read of the object at key.
	// The caller must close the returned ReadCloser after consuming it.
	Download(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete permanently removes the object at key.
	// Returns nil if the key does not exist.
	Delete(ctx context.Context, key string) error

	// Exists reports whether an object with the given key is present.
	Exists(ctx context.Context, key string) (bool, error)

	// GetURL generates a pre-signed URL granting time-limited read access to key.
	GetURL(ctx context.Context, key string, expires time.Duration) (string, error)

	// Name returns the provider identifier (e.g. "s3", "gcs", "mock-s3").
	Name() string
}
