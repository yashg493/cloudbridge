package cloud

import (
	"context"
	"fmt"

	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"
)

// S3Config holds all configuration required to build an S3 client.
type S3Config struct {
	Region          string // e.g. "us-east-1"
	Endpoint        string // override for LocalStack / MinIO; empty = AWS default
	AccessKeyID     string // leave empty to rely on the default credential chain
	SecretAccessKey string
	ForcePathStyle  bool // must be true for LocalStack
}

// S3Provider implements Provider for AWS S3.
// Uses the aws-sdk-go-v2 client with streaming multipart uploads.
type S3Provider struct {
	client *awss3.Client
	cfg    S3Config
	logger *zap.Logger
}

// Compile-time assertion that S3Provider satisfies the Provider interface.
var _ Provider = (*S3Provider)(nil)

// NewS3Provider creates an authenticated S3Provider from the given config.
func NewS3Provider(ctx context.Context, cfg S3Config, logger *zap.Logger) (*S3Provider, error) {
	// TODO: load aws.Config via config.LoadDefaultConfig(ctx, config.WithRegion(cfg.Region))
	// TODO: if cfg.AccessKeyID != "" override credentials with
	//       credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")
	// TODO: if cfg.Endpoint != "" configure a custom EndpointResolverWithOptions
	//       (required for LocalStack; set BaseEndpoint on awss3.Options)
	// TODO: awss3.NewFromConfig(awsCfg, func(o *awss3.Options) {
	//           o.UsePathStyle = cfg.ForcePathStyle
	//       })
	logger.Info("initialising S3 provider",
		zap.String("region", cfg.Region),
		zap.String("endpoint", cfg.Endpoint),
		zap.Bool("force_path_style", cfg.ForcePathStyle),
	)
	return nil, fmt.Errorf("S3Provider.New: not implemented")
}

// Upload implements Provider.Upload.
// Uses s3manager.NewUploader for automatic multipart handling.
func (p *S3Provider) Upload(ctx context.Context, input UploadInput) (ObjectInfo, error) {
	// TODO: build s3manager.UploadInput{Bucket, Key, Body, ContentType, Metadata}
	// TODO: uploader.Upload(ctx, ...) — s3manager handles part size & concurrency
	// TODO: map output.ETag to ObjectInfo
	return ObjectInfo{}, fmt.Errorf("S3Provider.Upload: not implemented")
}

// Download implements Provider.Download.
// Returns a streaming ReadCloser; the caller must close it.
func (p *S3Provider) Download(ctx context.Context, bucket, key string) (DownloadOutput, error) {
	// TODO: p.client.GetObject(ctx, &awss3.GetObjectInput{Bucket: &bucket, Key: &key})
	// TODO: map resp.Body, *resp.ContentType, *resp.ContentLength, *resp.ETag
	return DownloadOutput{}, fmt.Errorf("S3Provider.Download: not implemented")
}

// Delete implements Provider.Delete.
func (p *S3Provider) Delete(ctx context.Context, bucket, key string) error {
	// TODO: p.client.DeleteObject(ctx, &awss3.DeleteObjectInput{Bucket: &bucket, Key: &key})
	return fmt.Errorf("S3Provider.Delete: not implemented")
}

// Exists implements Provider.Exists using a lightweight HeadObject call.
func (p *S3Provider) Exists(ctx context.Context, bucket, key string) (bool, error) {
	// TODO: p.client.HeadObject(ctx, &awss3.HeadObjectInput{Bucket: &bucket, Key: &key})
	// TODO: if err is a *types.NotFound return false, nil
	// TODO: any other error: return false, err
	return false, fmt.Errorf("S3Provider.Exists: not implemented")
}

// Name implements Provider.Name.
func (p *S3Provider) Name() string { return "s3" }
