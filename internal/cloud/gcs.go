package cloud

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// GCSConfig holds Google Cloud Storage configuration.
type GCSConfig struct {
	ProjectID       string // GCP project ID
	CredentialsFile string // path to service-account JSON; empty = use ADC
}

// GCSProvider implements Provider for Google Cloud Storage.
// NOTE: This is a stub — all methods return ErrNotImplemented.
//
// Implementation guide:
//   - import "cloud.google.com/go/storage" (add to go.mod when implementing)
//   - NewGCSProvider: storage.NewClient(ctx, option.WithCredentialsFile(...))
//   - Upload: wc := client.Bucket(b).Object(k).NewWriter(ctx); io.Copy(wc, body); wc.Close()
//   - Download: client.Bucket(b).Object(k).NewReader(ctx)
//   - Delete: client.Bucket(b).Object(k).Delete(ctx)
//   - Exists: client.Bucket(b).Object(k).Attrs(ctx) — nil err == exists
type GCSProvider struct {
	cfg    GCSConfig
	logger *zap.Logger
}

// Compile-time assertion that GCSProvider satisfies the Provider interface.
var _ Provider = (*GCSProvider)(nil)

// NewGCSProvider creates a GCSProvider stub. Logs a warning that GCS is unimplemented.
func NewGCSProvider(_ context.Context, cfg GCSConfig, logger *zap.Logger) (*GCSProvider, error) {
	logger.Warn("GCSProvider is a stub — operations will return errors until implemented",
		zap.String("project_id", cfg.ProjectID),
	)
	return &GCSProvider{cfg: cfg, logger: logger}, nil
}

// Upload implements Provider.Upload (stub).
func (p *GCSProvider) Upload(_ context.Context, _ UploadInput) (ObjectInfo, error) {
	return ObjectInfo{}, fmt.Errorf("GCSProvider.Upload: not implemented")
}

// Download implements Provider.Download (stub).
func (p *GCSProvider) Download(_ context.Context, _, _ string) (DownloadOutput, error) {
	return DownloadOutput{}, fmt.Errorf("GCSProvider.Download: not implemented")
}

// Delete implements Provider.Delete (stub).
func (p *GCSProvider) Delete(_ context.Context, _, _ string) error {
	return fmt.Errorf("GCSProvider.Delete: not implemented")
}

// Exists implements Provider.Exists (stub).
func (p *GCSProvider) Exists(_ context.Context, _, _ string) (bool, error) {
	return false, fmt.Errorf("GCSProvider.Exists: not implemented")
}

// Name implements Provider.Name.
func (p *GCSProvider) Name() string { return "gcs" }
