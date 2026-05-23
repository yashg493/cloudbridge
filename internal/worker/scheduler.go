package worker

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/yashg493/cloudbridge/internal/cloud"
	"github.com/yashg493/cloudbridge/internal/models"
	"github.com/yashg493/cloudbridge/internal/store"
)

// Scheduler periodically scans for files that have crossed an inactivity threshold
// and enqueues tiering jobs to move them to cheaper storage.
//
// Policy (configurable via constructor options in a future phase):
//   - hot → warm  after hotThreshold  of no access  (default 7 days)
//   - warm → cold after warmThreshold of no access  (default 30 days)
type Scheduler struct {
	pool         *Pool
	fileRepo     *store.FileRepo
	provider     cloud.Provider
	logger       *zap.Logger
	interval     time.Duration // how often to run the scan
	hotThreshold time.Duration // inactivity before hot→warm promotion
	warmThreshold time.Duration // inactivity before warm→cold promotion
}

// NewScheduler creates a Scheduler with sensible default tier thresholds.
func NewScheduler(
	pool *Pool,
	fileRepo *store.FileRepo,
	provider cloud.Provider,
	logger *zap.Logger,
	interval time.Duration,
) *Scheduler {
	return &Scheduler{
		pool:          pool,
		fileRepo:      fileRepo,
		provider:      provider,
		logger:        logger,
		interval:      interval,
		hotThreshold:  7 * 24 * time.Hour,
		warmThreshold: 30 * 24 * time.Hour,
	}
}

// Run starts the scheduling loop and blocks until ctx is cancelled.
// Designed to run as a background goroutine: go scheduler.Run(ctx).
func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.logger.Info("tiering scheduler started",
		zap.Duration("interval", s.interval),
		zap.Duration("hot_threshold", s.hotThreshold),
		zap.Duration("warm_threshold", s.warmThreshold),
	)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("tiering scheduler stopped")
			return
		case t := <-ticker.C:
			s.logger.Debug("tiering scan tick", zap.Time("tick", t))
			s.scanAndEnqueue(ctx)
		}
	}
}

// scanAndEnqueue queries for files that have breached their tier threshold and
// submits SyncJobs to the pool. This is a best-effort operation — failures are
// logged but not fatal; the next tick will retry any missed files.
func (s *Scheduler) scanAndEnqueue(ctx context.Context) {
	// TODO: query DB for hot files with accessed_at < NOW() - s.hotThreshold
	//       SELECT id FROM files WHERE tier='hot' AND status='active'
	//       AND accessed_at < NOW() - INTERVAL '7 days'
	//       LIMIT 100 (batch to avoid lock contention)
	// TODO: for each file, create NewSyncJob(..., models.TierWarm, ...) and s.pool.Submit(job)

	// TODO: query DB for warm files with accessed_at < NOW() - s.warmThreshold
	//       SELECT id FROM files WHERE tier='warm' AND status='active'
	//       AND accessed_at < NOW() - INTERVAL '30 days'
	// TODO: for each file, create NewSyncJob(..., models.TierCold, ...) and s.pool.Submit(job)

	// TODO: log enqueued counts per tier transition

	// Ensure models package stays imported until queries are implemented.
	_ = models.TierWarm
	_ = models.TierCold

	s.logger.Debug("tiering scan complete (not yet implemented)")
}
