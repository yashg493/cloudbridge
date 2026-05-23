package worker

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/yashg493/cloudbridge/internal/cloud"
	"github.com/yashg493/cloudbridge/internal/models"
	"github.com/yashg493/cloudbridge/internal/store"
)

// SyncJob implements Job for file tiering and cloud synchronisation operations.
// It is the primary way file data moves between hot (NFS) and warm/cold (cloud) tiers.
type SyncJob struct {
	id         string
	jobType    string // matches models.SyncOperation values
	fileID     uuid.UUID
	targetTier models.TierType
	fileRepo   *store.FileRepo
	provider   cloud.Provider
	logger     *zap.Logger
}

// Compile-time assertion that SyncJob satisfies the Job interface.
var _ Job = (*SyncJob)(nil)

// NewSyncJob constructs a SyncJob. jobType should be one of the SyncOperation constants.
func NewSyncJob(
	fileID uuid.UUID,
	targetTier models.TierType,
	jobType string,
	fileRepo *store.FileRepo,
	provider cloud.Provider,
	logger *zap.Logger,
) *SyncJob {
	return &SyncJob{
		id:         uuid.New().String(),
		jobType:    jobType,
		fileID:     fileID,
		targetTier: targetTier,
		fileRepo:   fileRepo,
		provider:   provider,
		logger:     logger,
	}
}

// ID implements Job.ID.
func (j *SyncJob) ID() string { return j.id }

// Type implements Job.Type.
func (j *SyncJob) Type() string { return j.jobType }

// Execute performs the tier transition for j.fileID.
//
// Tier-up (hot → warm/cold):
//   - fetch file metadata
//   - open local byte stream (TODO: NFS mount path)
//   - upload to cloud via j.provider.Upload
//   - update tier + cloud_key in DB via j.fileRepo.UpdateTier
//   - mark sync_job as completed
//
// Tier-down (warm/cold → hot):
//   - fetch file metadata
//   - download from cloud via j.provider.Download
//   - write to local NFS path (TODO)
//   - update tier (cloud_key stays for reference) in DB
//   - mark sync_job as completed
func (j *SyncJob) Execute(ctx context.Context) error {
	log := j.logger.With(
		zap.String("job_id", j.id),
		zap.String("file_id", j.fileID.String()),
		zap.String("target_tier", string(j.targetTier)),
		zap.String("job_type", j.jobType),
	)

	log.Info("sync job started")

	// TODO: fetch file record — return wrapped error on not-found
	file, err := j.fileRepo.GetByID(ctx, j.fileID)
	if err != nil {
		return fmt.Errorf("sync_job %s: get file: %w", j.id, err)
	}

	switch j.targetTier {
	case models.TierWarm, models.TierCold:
		// TODO: tier-up — upload local file bytes to cloud
		//   1. open file at NFS mount path (file.Path)
		//   2. j.provider.Upload(ctx, cloud.UploadInput{...})
		//   3. j.fileRepo.UpdateTier(ctx, file.ID, j.targetTier, &cloudKey)
		log.Info("tier-up: uploading to cloud", zap.String("path", file.Path))
		return fmt.Errorf("sync_job %s: tier-up not implemented", j.id)

	case models.TierHot:
		// TODO: tier-down / recall — download from cloud to NFS
		//   1. j.provider.Download(ctx, bucket, *file.CloudKey)
		//   2. write bytes to NFS path
		//   3. j.fileRepo.UpdateTier(ctx, file.ID, models.TierHot, nil)
		log.Info("tier-down: recalling from cloud", zap.String("cloud_key", file.CloudKey))
		return fmt.Errorf("sync_job %s: tier-down not implemented", j.id)

	default:
		return fmt.Errorf("sync_job %s: unknown target tier %q", j.id, j.targetTier)
	}
}
