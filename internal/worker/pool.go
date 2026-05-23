package worker

import (
	"context"
	"errors"
	"sync"

	"go.uber.org/zap"
)

// ErrQueueFull is returned by Submit when the job queue has reached capacity.
var ErrQueueFull = errors.New("worker: job queue is full")

// Job is the unit of work executed by the pool.
// Implementations must be idempotent where possible (worker may retry on transient errors).
type Job interface {
	// Execute performs the work. Receives the pool's context; honour cancellation.
	Execute(ctx context.Context) error
	// ID returns a stable unique identifier used for logging and deduplication.
	ID() string
	// Type returns a short human-readable label (e.g. "tier_up", "tier_down").
	Type() string
}

// Pool is a fixed-size goroutine pool that processes Jobs from a bounded channel.
// Callers submit jobs via Submit; the pool fans them out across worker goroutines.
// All workers share the same context; cancelling it triggers a graceful drain.
type Pool struct {
	workers    int
	jobQueue   chan Job
	wg         sync.WaitGroup
	logger     *zap.Logger
	ctx        context.Context
	cancelFunc context.CancelFunc
}

// NewPool creates a Pool with the given number of goroutines and queue depth.
// Call Start to launch the workers; call Stop to drain and shut down.
func NewPool(ctx context.Context, workers, queueSize int, logger *zap.Logger) *Pool {
	poolCtx, cancel := context.WithCancel(ctx)
	return &Pool{
		workers:    workers,
		jobQueue:   make(chan Job, queueSize),
		logger:     logger,
		ctx:        poolCtx,
		cancelFunc: cancel,
	}
}

// Start spawns all worker goroutines. Must be called exactly once before Submit.
func (p *Pool) Start() {
	for i := range p.workers {
		p.wg.Add(1)
		go p.runWorker(i)
	}
	p.logger.Info("worker pool started", zap.Int("workers", p.workers), zap.Int("queue_cap", cap(p.jobQueue)))
}

// Submit enqueues a job for execution. Returns ErrQueueFull immediately if the
// channel is at capacity (non-blocking to avoid back-pressure on HTTP handlers).
func (p *Pool) Submit(job Job) error {
	select {
	case p.jobQueue <- job:
		// TODO: increment WorkerQueueDepth metric
		p.logger.Debug("job enqueued", zap.String("job_id", job.ID()), zap.String("type", job.Type()))
		return nil
	default:
		p.logger.Warn("job queue full, dropping job",
			zap.String("job_id", job.ID()),
			zap.String("type", job.Type()),
		)
		// TODO: increment dropped_jobs_total metric
		return ErrQueueFull
	}
}

// Stop signals the workers to stop, drains all queued jobs, and waits for
// in-flight jobs to finish. Safe to call multiple times.
func (p *Pool) Stop() {
	p.cancelFunc()      // signal workers to stop accepting new jobs
	close(p.jobQueue)   // drain: workers finish remaining jobs then exit
	p.wg.Wait()
	p.logger.Info("worker pool stopped")
}

// runWorker is the per-goroutine event loop. It processes jobs until the channel
// is closed or the pool context is cancelled.
func (p *Pool) runWorker(id int) {
	defer p.wg.Done()
	p.logger.Debug("worker started", zap.Int("worker_id", id))

	for job := range p.jobQueue {
		// TODO: decrement WorkerQueueDepth metric
		p.logger.Debug("executing job",
			zap.Int("worker_id", id),
			zap.String("job_id", job.ID()),
			zap.String("type", job.Type()),
		)

		// TODO: wrap Execute with a per-job timeout derived from p.ctx
		// TODO: record WorkerJobDuration histogram
		if err := job.Execute(p.ctx); err != nil {
			p.logger.Error("job execution failed",
				zap.Int("worker_id", id),
				zap.String("job_id", job.ID()),
				zap.String("type", job.Type()),
				zap.Error(err),
			)
			// TODO: increment WorkerJobsTotal{type, "failed"}
		} else {
			// TODO: increment WorkerJobsTotal{type, "completed"}
		}
	}

	p.logger.Debug("worker stopped", zap.Int("worker_id", id))
}
