package worker

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/yashg493/cloudbridge/internal/models"
	"github.com/yashg493/cloudbridge/internal/store"
)

const (
	schedulerInterval  = 2 * time.Second
	schedulerPollLimit = 50
	schedulerPollTimeout = 10 * time.Second
)

// Scheduler polls the database for pending sync jobs every 2 seconds and
// submits them to the WorkerPool. It is the bridge between the durable DB
// queue and the in-process goroutine pool.
//
// Concurrency safety:
//   - PollPending uses SELECT FOR UPDATE SKIP LOCKED, so multiple Scheduler
//     instances (one per pod) can run concurrently without double-processing.
//   - On ErrPoolFull the scheduler resets unsubmitted jobs to "pending" so
//     the next poll cycle can reclaim them.
type Scheduler struct {
	pool    *WorkerPool
	jobRepo *store.SyncJobRepo
	logger  *zap.Logger
	done    chan struct{}
	once    sync.Once
}

// NewScheduler creates a Scheduler. Call Start() to begin polling.
func NewScheduler(pool *WorkerPool, jobRepo *store.SyncJobRepo, logger *zap.Logger) *Scheduler {
	return &Scheduler{
		pool:    pool,
		jobRepo: jobRepo,
		logger:  logger,
		done:    make(chan struct{}),
	}
}

// Start launches the polling goroutine. Non-blocking.
func (s *Scheduler) Start() {
	go s.run()
	s.logger.Info("scheduler started",
		zap.Duration("interval", schedulerInterval),
		zap.Int("poll_limit", schedulerPollLimit),
	)
}

// Stop signals the polling goroutine to exit. Idempotent and safe to call
// multiple times or from multiple goroutines.
func (s *Scheduler) Stop() {
	s.once.Do(func() {
		close(s.done)
		s.logger.Info("scheduler stopped")
	})
}

// run is the main polling loop.
func (s *Scheduler) run() {
	ticker := time.NewTicker(schedulerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.poll()
		}
	}
}

// poll fetches a batch of pending jobs from the DB, submits them to the pool,
// and resets any that could not be submitted back to "pending".
func (s *Scheduler) poll() {
	ctx, cancel := context.WithTimeout(context.Background(), schedulerPollTimeout)
	defer cancel()

	jobs, err := s.jobRepo.PollPending(ctx, schedulerPollLimit)
	if err != nil {
		s.logger.Error("scheduler: poll pending failed", zap.Error(err))
		return
	}
	if len(jobs) == 0 {
		return
	}

	var submitted int
	for i, job := range jobs {
		if err := s.pool.Submit(job); err != nil {
			if errors.Is(err, ErrPoolFull) {
				// Pool is saturated. Reset this job and all remaining ones
				// to "pending" so the next poll cycle can reclaim them.
				remaining := jobs[i:]
				s.logger.Warn("pool full; requeueing jobs to pending",
					zap.Int("count", len(remaining)),
				)
				for _, r := range remaining {
					if rstErr := s.jobRepo.UpdateStatus(
						ctx, r.ID,
						string(models.SyncStatusPending), ""); rstErr != nil {
						s.logger.Error("failed to reset job to pending",
							zap.String("job_id", r.ID.String()),
							zap.Error(rstErr),
						)
					}
				}
				break
			}
			// Unexpected submit error (pool shutting down); log and stop polling.
			s.logger.Error("scheduler: submit error",
				zap.String("job_id", job.ID.String()),
				zap.Error(err),
			)
			break
		}
		submitted++
	}

	if submitted > 0 {
		s.logger.Info("scheduler: jobs submitted",
			zap.Int("submitted", submitted),
			zap.Int("polled", len(jobs)),
		)
	}
}
