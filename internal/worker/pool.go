package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/yashg493/cloudbridge/internal/cloud"
	"github.com/yashg493/cloudbridge/internal/metrics"
	"github.com/yashg493/cloudbridge/internal/models"
	"github.com/yashg493/cloudbridge/internal/store"
)

// ErrPoolFull is returned by Submit when the job channel is at capacity.
var ErrPoolFull = errors.New("worker: pool job queue is full")

// Deps bundles every external dependency required by ProcessJob.
// Construct one value and pass it to NewWorkerPool; it is propagated to
// every job execution without further allocation.
type Deps struct {
	FileRepo    *store.FileRepo
	NSRepo      *store.NamespaceRepo
	SyncJobRepo *store.SyncJobRepo
	Provider    cloud.CloudProvider
	Metrics     *metrics.Registry
	Logger      *zap.Logger
}

// WorkerPool is a bounded, concurrent pool that processes *models.SyncJob records
// pulled from a buffered channel. Jobs are sourced by the Scheduler from the
// database and handed to the pool via Submit.
type WorkerPool struct {
	size          int
	jobChan       chan *models.SyncJob
	wg            sync.WaitGroup
	activeWorkers atomic.Int32
	ctx           context.Context
	cancel        context.CancelFunc
	deps          Deps
	mu            sync.Mutex // protects closed
	closed        bool
}

// NewWorkerPool creates a WorkerPool with size goroutines.
// The internal channel is buffered at size×10 to absorb submission bursts.
// Call Start() to launch workers and Submit() to enqueue jobs.
func NewWorkerPool(ctx context.Context, size int, deps Deps) *WorkerPool {
	poolCtx, cancel := context.WithCancel(ctx)
	return &WorkerPool{
		size:    size,
		jobChan: make(chan *models.SyncJob, size*10),
		ctx:     poolCtx,
		cancel:  cancel,
		deps:    deps,
	}
}

// Start spawns size worker goroutines. Must be called exactly once before Submit.
func (wp *WorkerPool) Start() {
	for i := range wp.size {
		wp.wg.Add(1)
		go wp.workerLoop(i)
	}
	wp.deps.Logger.Info("worker pool started",
		zap.Int("size", wp.size),
		zap.Int("queue_cap", cap(wp.jobChan)),
	)
}

// Submit enqueues job for asynchronous processing.
// Non-blocking: returns ErrPoolFull immediately if the channel is at capacity
// so HTTP handlers are never stalled. Returns an error if the pool is shutting down.
func (wp *WorkerPool) Submit(job *models.SyncJob) error {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if wp.closed {
		return fmt.Errorf("worker: pool is shutting down")
	}

	select {
	case wp.jobChan <- job:
		if wp.deps.Metrics != nil {
			wp.deps.Metrics.WorkerQueueDepth.Set(float64(len(wp.jobChan)))
		}
		return nil
	default:
		if wp.deps.Metrics != nil {
			wp.deps.Metrics.WorkerJobsTotal.WithLabelValues(string(job.Operation), "dropped").Inc()
		}
		return ErrPoolFull
	}
}

// Shutdown gracefully stops the pool:
//  1. Closes the job channel (no new jobs accepted; Submit returns an error).
//  2. Lets in-flight and remaining-queued jobs complete.
//  3. Returns ctx.Err() if the provided context expires before workers finish.
func (wp *WorkerPool) Shutdown(ctx context.Context) error {
	wp.mu.Lock()
	if wp.closed {
		wp.mu.Unlock()
		return nil
	}
	wp.closed = true
	close(wp.jobChan) // workers range over the channel and drain remaining jobs
	wp.mu.Unlock()

	wp.cancel() // cancel pool context so in-flight ops can detect shutdown

	done := make(chan struct{})
	go func() { wp.wg.Wait(); close(done) }()

	select {
	case <-done:
		wp.deps.Logger.Info("worker pool stopped cleanly")
		return nil
	case <-ctx.Done():
		return fmt.Errorf("worker pool shutdown timed out: %w", ctx.Err())
	}
}

// ActiveWorkers returns the number of goroutines currently executing a job.
func (wp *WorkerPool) ActiveWorkers() int32 {
	return wp.activeWorkers.Load()
}

// workerLoop is the per-goroutine event loop. It ranges over jobChan until
// the channel is closed by Shutdown, then exits and signals wg.Done.
func (wp *WorkerPool) workerLoop(workerID int) {
	defer wp.wg.Done()
	wp.activeWorkers.Add(1)
	defer wp.activeWorkers.Add(-1)

	wp.deps.Logger.Debug("worker started", zap.Int("worker_id", workerID))

	for job := range wp.jobChan {
		if wp.deps.Metrics != nil {
			wp.deps.Metrics.WorkerQueueDepth.Set(float64(len(wp.jobChan)))
		}

		start := time.Now()
		log := wp.deps.Logger.With(
			zap.Int("worker_id", workerID),
			zap.String("job_id", job.ID.String()),
			zap.String("operation", string(job.Operation)),
		)
		log.Debug("processing job")

		err := ProcessJob(wp.ctx, job, wp.deps)

		elapsed := time.Since(start)
		if wp.deps.Metrics != nil {
			wp.deps.Metrics.WorkerJobDuration.
				WithLabelValues(string(job.Operation)).
				Observe(elapsed.Seconds())
		}

		if err != nil {
			log.Error("job permanently failed",
				zap.Duration("elapsed", elapsed),
				zap.Error(err),
			)
			if wp.deps.Metrics != nil {
				wp.deps.Metrics.WorkerJobsTotal.
					WithLabelValues(string(job.Operation), "failed").Inc()
			}
		} else {
			log.Debug("job completed", zap.Duration("elapsed", elapsed))
			if wp.deps.Metrics != nil {
				wp.deps.Metrics.WorkerJobsTotal.
					WithLabelValues(string(job.Operation), "completed").Inc()
			}
		}
	}

	wp.deps.Logger.Debug("worker stopped", zap.Int("worker_id", workerID))
}
