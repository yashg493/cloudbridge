package cloud

// TODO: implement using cloud.google.com/go/storage
// Implementation guide:
//   - storage.NewClient(ctx, option.WithCredentialsFile(credentialsFile))
//   - Upload:   wc := client.Bucket(bucket).Object(key).NewWriter(ctx); io.Copy(wc, data); wc.Close()
//   - Download: r, _ := client.Bucket(bucket).Object(key).NewReader(ctx)
//   - Delete:   client.Bucket(bucket).Object(key).Delete(ctx)
//   - Exists:   _, err := client.Bucket(bucket).Object(key).Attrs(ctx); err == nil → exists
//   - GetURL:   client.Bucket(bucket).Object(key).SignedURL(storage.SignedURLOptions{Expires: ...})

import (
	"context"
	"io"
	"time"
)

// GCSProvider is a stub implementation of CloudProvider for Google Cloud Storage.
// All methods return ErrNotImplemented until the GCS client is wired in.
type GCSProvider struct{}

// Compile-time assertion that GCSProvider satisfies CloudProvider.
var _ CloudProvider = (*GCSProvider)(nil)

func (g *GCSProvider) Upload(_ context.Context, _ string, _ io.Reader, _ int64) error {
	return ErrNotImplemented
}
func (g *GCSProvider) Download(_ context.Context, _ string) (io.ReadCloser, error) {
	return nil, ErrNotImplemented
}
func (g *GCSProvider) Delete(_ context.Context, _ string) error       { return ErrNotImplemented }
func (g *GCSProvider) Exists(_ context.Context, _ string) (bool, error) {
	return false, ErrNotImplemented
}
func (g *GCSProvider) GetURL(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", ErrNotImplemented
}
func (g *GCSProvider) Name() string { return "gcs" }
