package cloud

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"go.uber.org/zap"
)

// ── S3Provider ────────────────────────────────────────────────────────────────

// S3Provider implements CloudProvider for AWS S3.
// The target bucket is fixed at construction time; callers only supply the key.
type S3Provider struct {
	client *s3.Client
	bucket string
	logger *zap.Logger
}

// Compile-time assertion.
var _ CloudProvider = (*S3Provider)(nil)

// s3Config bundles construction-time settings for S3Provider.
type s3Config struct {
	region          string
	bucket          string
	accessKeyID     string
	secretAccessKey string
	endpoint        string // custom endpoint for LocalStack / MinIO; empty = AWS default
}

// newS3Provider builds an authenticated S3Provider from cfg.
func newS3Provider(ctx context.Context, cfg s3Config, logger *zap.Logger) (*S3Provider, error) {
	loadOpts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.region),
	}
	if cfg.accessKeyID != "" {
		// Override the default credential chain with static credentials.
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(
			aws.CredentialsProviderFunc(func(_ context.Context) (aws.Credentials, error) {
				return aws.Credentials{
					AccessKeyID:     cfg.accessKeyID,
					SecretAccessKey: cfg.secretAccessKey,
				}, nil
			}),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("s3: load AWS config: %w", err)
	}

	s3Opts := []func(*s3.Options){}
	if cfg.endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.endpoint)
			o.UsePathStyle = true // required for LocalStack / MinIO
		})
	}

	return &S3Provider{
		client: s3.NewFromConfig(awsCfg, s3Opts...),
		bucket: cfg.bucket,
		logger: logger,
	}, nil
}

// NewProvider returns a CloudProvider backed by real S3 when AWS_REGION and
// AWS_BUCKET environment variables are set. Otherwise it falls back to the
// in-memory MockS3Provider so the system works without AWS credentials.
func NewProvider(ctx context.Context, logger *zap.Logger) CloudProvider {
	region := os.Getenv("AWS_REGION")
	bucket := os.Getenv("AWS_BUCKET")

	if region == "" || bucket == "" {
		logger.Info("AWS_REGION or AWS_BUCKET not set — using in-memory MockS3Provider")
		return NewMockS3Provider(logger)
	}

	cfg := s3Config{
		region:          region,
		bucket:          bucket,
		accessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
		secretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
		endpoint:        os.Getenv("AWS_ENDPOINT_URL"), // set to http://localstack:4566 for LocalStack
	}

	provider, err := newS3Provider(ctx, cfg, logger)
	if err != nil {
		logger.Error("failed to create S3 provider — falling back to MockS3Provider",
			zap.Error(err))
		return NewMockS3Provider(logger)
	}

	logger.Info("using AWS S3 provider",
		zap.String("region", region),
		zap.String("bucket", bucket),
		zap.String("endpoint", cfg.endpoint),
	)
	return provider
}

// Upload implements CloudProvider.Upload via PutObject.
func (p *S3Provider) Upload(ctx context.Context, key string, data io.Reader, sizeBytes int64) error {
	start := time.Now()

	input := &s3.PutObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(key),
		Body:   data,
	}
	if sizeBytes >= 0 {
		input.ContentLength = aws.Int64(sizeBytes)
	}

	_, err := p.client.PutObject(ctx, input)
	if err != nil {
		return fmt.Errorf("s3: upload %q: %w", key, err)
	}
	p.logger.Debug("s3: Upload",
		zap.String("key", key),
		zap.Int64("size_bytes", sizeBytes),
		zap.Duration("duration", time.Since(start)),
	)
	return nil
}

// Download implements CloudProvider.Download via GetObject.
// The caller must close the returned ReadCloser.
func (p *S3Provider) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	start := time.Now()

	result, err := p.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("s3: download %q: %w", key, err)
	}
	p.logger.Debug("s3: Download",
		zap.String("key", key),
		zap.Duration("duration", time.Since(start)),
	)
	return result.Body, nil
}

// Delete implements CloudProvider.Delete via DeleteObject.
func (p *S3Provider) Delete(ctx context.Context, key string) error {
	start := time.Now()

	_, err := p.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("s3: delete %q: %w", key, err)
	}
	p.logger.Debug("s3: Delete",
		zap.String("key", key),
		zap.Duration("duration", time.Since(start)),
	)
	return nil
}

// Exists implements CloudProvider.Exists via HeadObject.
func (p *S3Provider) Exists(ctx context.Context, key string) (bool, error) {
	_, err := p.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var notFound *s3types.NotFound
		var noSuchKey *s3types.NoSuchKey
		if errors.As(err, &notFound) || errors.As(err, &noSuchKey) {
			return false, nil
		}
		return false, fmt.Errorf("s3: exists %q: %w", key, err)
	}
	return true, nil
}

// GetURL generates a pre-signed GET URL valid for expires duration.
func (p *S3Provider) GetURL(ctx context.Context, key string, expires time.Duration) (string, error) {
	presignClient := s3.NewPresignClient(p.client)
	req, err := presignClient.PresignGetObject(ctx,
		&s3.GetObjectInput{
			Bucket: aws.String(p.bucket),
			Key:    aws.String(key),
		},
		s3.WithPresignExpires(expires),
	)
	if err != nil {
		return "", fmt.Errorf("s3: presign %q: %w", key, err)
	}
	return req.URL, nil
}

// Name implements CloudProvider.Name.
func (p *S3Provider) Name() string { return "s3" }

// ── MockS3Provider ────────────────────────────────────────────────────────────

// MockS3Provider is a thread-safe in-memory CloudProvider for testing and
// local development when real AWS credentials are not available.
// Data is stored in a map[string][]byte; it is not persisted across restarts.
type MockS3Provider struct {
	mu      sync.RWMutex
	objects map[string][]byte
	logger  *zap.Logger
}

// Compile-time assertion.
var _ CloudProvider = (*MockS3Provider)(nil)

// NewMockS3Provider creates a MockS3Provider and logs a warning.
func NewMockS3Provider(logger *zap.Logger) *MockS3Provider {
	logger.Warn("MockS3Provider active — objects are stored in memory and not persisted")
	return &MockS3Provider{
		objects: make(map[string][]byte),
		logger:  logger,
	}
}

// Upload implements CloudProvider.Upload (in-memory).
func (m *MockS3Provider) Upload(_ context.Context, key string, data io.Reader, _ int64) error {
	start := time.Now()

	b, err := io.ReadAll(data)
	if err != nil {
		return fmt.Errorf("mock_s3: read data for %q: %w", key, err)
	}

	m.mu.Lock()
	m.objects[key] = b
	m.mu.Unlock()

	m.logger.Debug("mock_s3: Upload",
		zap.String("key", key),
		zap.Int("size_bytes", len(b)),
		zap.Duration("duration", time.Since(start)),
	)
	return nil
}

// Download implements CloudProvider.Download (in-memory).
func (m *MockS3Provider) Download(_ context.Context, key string) (io.ReadCloser, error) {
	start := time.Now()

	m.mu.RLock()
	data, ok := m.objects[key]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("mock_s3: key %q not found", key)
	}

	m.logger.Debug("mock_s3: Download",
		zap.String("key", key),
		zap.Int("size_bytes", len(data)),
		zap.Duration("duration", time.Since(start)),
	)
	return io.NopCloser(bytes.NewReader(data)), nil
}

// Delete implements CloudProvider.Delete (in-memory).
func (m *MockS3Provider) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	delete(m.objects, key)
	m.mu.Unlock()
	m.logger.Debug("mock_s3: Delete", zap.String("key", key))
	return nil
}

// Exists implements CloudProvider.Exists (in-memory).
func (m *MockS3Provider) Exists(_ context.Context, key string) (bool, error) {
	m.mu.RLock()
	_, ok := m.objects[key]
	m.mu.RUnlock()
	return ok, nil
}

// GetURL implements CloudProvider.GetURL (returns a mock URL).
func (m *MockS3Provider) GetURL(_ context.Context, key string, expires time.Duration) (string, error) {
	return fmt.Sprintf("mock://cloudbridge/%s?expires=%d", key, int(expires.Seconds())), nil
}

// Name implements CloudProvider.Name.
func (m *MockS3Provider) Name() string { return "mock-s3" }
