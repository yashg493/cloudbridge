package worker

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/yashg493/cloudbridge/internal/models"
)

// ProcessJob is the core sync engine. It implements a state machine for a single
// *models.SyncJob:
//  1. Mark the job as "running" in the database.
//  2. Dispatch to the correct operation handler (upload/download/tier_move/delete).
//  3. On success: record bytes transferred and mark "completed".
//  4. On transient error (RetryCount < 3): sleep with exponential back-off (1s/2s/4s)
//     and reset the job to "pending" so the Scheduler can re-claim it.
//  5. On permanent failure (RetryCount >= 3): mark "failed" and return the error
//     so the WorkerPool can emit the appropriate metric.
func ProcessJob(ctx context.Context, job *models.SyncJob, deps Deps) error {
	log := deps.Logger.With(
		zap.String("job_id", job.ID.String()),
		zap.String("file_id", job.FileID.String()),
		zap.String("namespace_id", job.NamespaceID.String()),
		zap.String("operation", string(job.Operation)),
		zap.Int("retry_count", job.RetryCount),
	)

	// ── Step 1: mark running ──────────────────────────────────────────────────
	if err := deps.SyncJobRepo.UpdateStatus(
		ctx, job.ID, string(models.SyncStatusRunning), ""); err != nil {
		// If we can't mark the job running we still attempt the operation;
		// the status will be corrected when we mark completed/failed below.
		log.Warn("failed to mark job as running", zap.Error(err))
	}

	log.Info("sync job started")

	// ── Step 2: dispatch to operation handler ─────────────────────────────────
	var bytesTransferred int64
	var opErr error

	switch job.Operation {
	case models.SyncOperationUpload:
		bytesTransferred, opErr = execUpload(ctx, job, deps, log)
	case models.SyncOperationDownload:
		bytesTransferred, opErr = execDownload(ctx, job, deps, log)
	case models.SyncOperationTierMove:
		opErr = execTierMove(ctx, job, deps, log)
	case models.SyncOperationDelete:
		opErr = execDelete(ctx, job, deps, log)
	default:
		opErr = fmt.Errorf("unknown operation %q", job.Operation)
	}

	// ── Step 3: success path ──────────────────────────────────────────────────
	if opErr == nil {
		if bytesTransferred > 0 {
			if err := deps.SyncJobRepo.UpdateProgress(ctx, job.ID, bytesTransferred); err != nil {
				log.Warn("failed to update bytes_transferred", zap.Error(err))
			}
		}
		if err := deps.SyncJobRepo.UpdateStatus(
			ctx, job.ID, string(models.SyncStatusCompleted), ""); err != nil {
			log.Warn("failed to mark job completed", zap.Error(err))
		}
		if deps.Metrics != nil {
			deps.Metrics.SyncJobsTotal.
				WithLabelValues(string(job.Operation), "completed").Inc()
			if bytesTransferred > 0 && deps.Provider != nil {
				deps.Metrics.BytesTransferredTotal.
					WithLabelValues(string(job.Operation), deps.Provider.Name()).
					Add(float64(bytesTransferred))
			}
		}
		log.Info("sync job completed", zap.Int64("bytes_transferred", bytesTransferred))
		return nil
	}

	// ── Step 4: transient failure — retry with exponential back-off ───────────
	log.Warn("sync job failed", zap.Error(opErr))
	if deps.Metrics != nil {
		deps.Metrics.SyncJobsTotal.
			WithLabelValues(string(job.Operation), "error").Inc()
	}

	if job.RetryCount < 3 {
		// Backoff schedule: retry 0→1s, 1→2s, 2→4s
		backoff := time.Duration(1<<uint(job.RetryCount)) * time.Second
		log.Info("requeueing job with backoff",
			zap.Duration("backoff", backoff),
			zap.Int("next_retry_count", job.RetryCount+1),
		)

		// Sleep while honouring context cancellation (e.g. pool shutdown).
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return ctx.Err()
		}

		// Increment retry_count and reset to pending so the Scheduler picks it up.
		if err := deps.SyncJobRepo.RequeueWithRetry(ctx, job.ID, opErr.Error()); err != nil {
			log.Error("failed to requeue job", zap.Error(err))
		}
		return nil // error is handled by requeueing — do not propagate
	}

	// ── Step 5: permanent failure ─────────────────────────────────────────────
	if err := deps.SyncJobRepo.UpdateStatus(
		ctx, job.ID, string(models.SyncStatusFailed), opErr.Error()); err != nil {
		log.Error("failed to mark job as failed", zap.Error(err))
	}
	log.Error("sync job permanently failed",
		zap.Int("retry_count", job.RetryCount),
		zap.Error(opErr),
	)
	return opErr // returned so the WorkerPool increments the failed metric
}

// ── Operation handlers ────────────────────────────────────────────────────────

// execUpload reads file bytes from the NFS mount path and streams them to the
// configured cloud provider, then marks the file as cloud-synced in the DB.
func execUpload(ctx context.Context, job *models.SyncJob, deps Deps, log *zap.Logger) (int64, error) {
	file, err := deps.FileRepo.GetByID(ctx, job.FileID)
	if err != nil {
		return 0, fmt.Errorf("upload: get file metadata: %w", err)
	}

	ns, err := deps.NSRepo.GetByID(ctx, job.NamespaceID)
	if err != nil {
		return 0, fmt.Errorf("upload: get namespace: %w", err)
	}
	if ns.CloudBackend == models.CloudBackendNone {
		return 0, fmt.Errorf("upload: namespace %q has no cloud backend configured", ns.Name)
	}

	cloudKey := buildCloudKey(ns.CloudPrefix, job.NamespaceID.String(), file.Path)

	// TODO: replace with os.Open(filepath.Join(nfsMountPath, file.Path)) when NFS
	//       mount points are wired. For now simulate an empty body so the cloud
	//       provider call is exercised (it will fail with "not implemented" until
	//       the S3 provider is complete).
	body := strings.NewReader("")

	log.Info("uploading file to cloud",
		zap.String("path", file.Path),
		zap.String("bucket", ns.CloudBucket),
		zap.String("cloud_key", cloudKey),
		zap.Int64("size_bytes", file.SizeBytes),
	)

	if err := deps.Provider.Upload(ctx, cloudKey, body, file.SizeBytes); err != nil {
		return 0, fmt.Errorf("upload: cloud provider upload: %w", err)
	}

	if err := deps.FileRepo.UpdateCloudSync(ctx, job.FileID, cloudKey); err != nil {
		return 0, fmt.Errorf("upload: persist cloud key: %w", err)
	}

	log.Info("file uploaded", zap.String("cloud_key", cloudKey))
	return file.SizeBytes, nil
}

// execDownload fetches file bytes from cloud storage and writes them to the NFS
// mount path, then updates the file's tier to hot in the DB.
func execDownload(ctx context.Context, job *models.SyncJob, deps Deps, log *zap.Logger) (int64, error) {
	file, err := deps.FileRepo.GetByID(ctx, job.FileID)
	if err != nil {
		return 0, fmt.Errorf("download: get file metadata: %w", err)
	}
	if !file.CloudSynced || file.CloudKey == "" {
		return 0, fmt.Errorf("download: file %s has no cloud object to recall", file.ID)
	}

	log.Info("downloading file from cloud",
		zap.String("cloud_key", file.CloudKey),
	)

	body, err := deps.Provider.Download(ctx, file.CloudKey)
	if err != nil {
		return 0, fmt.Errorf("download: cloud provider download: %w", err)
	}
	defer body.Close()

	// TODO: replace io.Discard with nfs.Simulator.WriteFile(file.Path, data)
	//       once the NFS simulator is wired into the Deps struct.
	n, err := io.Copy(io.Discard, body)
	if err != nil {
		return 0, fmt.Errorf("download: drain response body: %w", err)
	}

	if err := deps.FileRepo.UpdateTier(ctx, job.FileID, string(models.TierHot)); err != nil {
		return 0, fmt.Errorf("download: update tier to hot: %w", err)
	}

	log.Info("file recalled to hot tier", zap.Int64("bytes", n))
	return n, nil
}

// execTierMove changes a file's storage class within cloud (e.g. Standard → Glacier)
// and updates the tier field in the DB. The progression is always hot→warm or warm→cold.
func execTierMove(ctx context.Context, job *models.SyncJob, deps Deps, log *zap.Logger) error {
	file, err := deps.FileRepo.GetByID(ctx, job.FileID)
	if err != nil {
		return fmt.Errorf("tier_move: get file: %w", err)
	}

	var targetTier models.TierType
	switch file.Tier {
	case models.TierHot:
		targetTier = models.TierWarm
	case models.TierWarm:
		targetTier = models.TierCold
	default:
		return fmt.Errorf("tier_move: file is already at terminal tier %q", file.Tier)
	}

	log.Info("moving file to colder tier",
		zap.String("from", string(file.Tier)),
		zap.String("to", string(targetTier)),
	)

	// TODO: issue a cloud-side CopyObject with the new storage class
	// (S3: StorageClass=STANDARD_IA for warm, GLACIER for cold) before updating DB.
	// This requires a Provider.CopyObject method — defer to Phase 2.

	if err := deps.FileRepo.UpdateTier(ctx, job.FileID, string(targetTier)); err != nil {
		return fmt.Errorf("tier_move: update tier in DB: %w", err)
	}
	return nil
}

// execDelete removes the file's cloud object and clears the cloud_synced flag in the DB.
func execDelete(ctx context.Context, job *models.SyncJob, deps Deps, log *zap.Logger) error {
	file, err := deps.FileRepo.GetByID(ctx, job.FileID)
	if err != nil {
		return fmt.Errorf("delete: get file: %w", err)
	}

	if !file.CloudSynced || file.CloudKey == "" {
		log.Info("delete: file has no cloud object, skipping provider call")
		return nil
	}

	log.Info("deleting cloud object",
		zap.String("cloud_key", file.CloudKey),
	)

	if err := deps.Provider.Delete(ctx, file.CloudKey); err != nil {
		return fmt.Errorf("delete: cloud provider delete: %w", err)
	}

	if err := deps.FileRepo.UpdateCloudSync(ctx, job.FileID, ""); err != nil {
		return fmt.Errorf("delete: clear cloud_synced flag: %w", err)
	}

	log.Info("cloud object deleted")
	return nil
}

// buildCloudKey constructs the object key for a file in cloud storage.
// Format: [prefix/]<namespaceID>/<filePath>
func buildCloudKey(prefix, namespaceID, filePath string) string {
	if prefix == "" {
		return fmt.Sprintf("%s/%s", namespaceID, filePath)
	}
	return fmt.Sprintf("%s/%s/%s", strings.TrimRight(prefix, "/"), namespaceID, filePath)
}
