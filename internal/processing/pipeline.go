package processing

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Stage describes where a job is in its lifecycle.
type Stage string

const (
	StageQueued Stage = "queued"
	// StageThumbnail is emitted when a preview image becomes available. It is
	// not an ffmpeg pipeline job, but it travels over the same progress channel
	// so clients can refresh as soon as the thumbnail exists.
	StageThumbnail  Stage = "thumbnail"
	StageProcessing Stage = "processing"
	StageDone       Stage = "done"
	StageFailed     Stage = "failed"
	StageCanceled   Stage = "canceled"
)

// ProgressEvent is emitted as a job moves through the pipeline. It serialises
// straight to the frontend (JSON field names match the client).
type ProgressEvent struct {
	MediaID  string  `json:"media_id"`
	Stage    Stage   `json:"stage"`
	Strategy string  `json:"strategy"`
	Percent  float64 `json:"percent"`
	Message  string  `json:"message,omitempty"`
}

// ProgressFunc receives every progress event. It must not block; the websocket
// hub is expected to buffer and fan out asynchronously.
type ProgressFunc func(ProgressEvent)

// Job is a single unit of work: prepare one source file for browser playback.
type Job struct {
	MediaID    string
	SourcePath string
	OutputPath string
	Strategy   Strategy
	Duration   float64

	// token identifies the reservation Submit made for this job. Submit sets it,
	// and it is what tells a job that still owns its slot from one that was
	// cancelled or replaced while it waited in the queue.
	token uint64
}

// jobRunner executes one job to completion. It is the seam between the
// pipeline's bookkeeping — queueing, cancelling, reporting — and the way a job
// is actually run: production uses ffmpegRunner, while tests substitute a runner
// that blocks until its context is cancelled instead of spawning a process.
type jobRunner func(ctx context.Context, job Job, onProgress ProgressFunc) error

// Pipeline runs preparation jobs on a bounded pool of goroutines. Each job can
// be cancelled individually, and progress is reported through a callback so the
// API layer can forward it over a websocket.
//
// At most one job per media item is ever in flight; see jobRegistry.
type Pipeline struct {
	jobs       chan Job
	wg         sync.WaitGroup
	onProgress ProgressFunc
	runner     jobRunner

	// active maps a media item to the job currently holding it.
	active *jobRegistry

	ctx    context.Context
	cancel context.CancelFunc
}

// NewPipeline starts `workers` goroutines consuming the job queue.
func NewPipeline(workers int, onProgress ProgressFunc) *Pipeline {
	if workers < 1 {
		workers = 1
	}
	if onProgress == nil {
		onProgress = func(ProgressEvent) {}
	}

	ctx, cancel := context.WithCancel(context.Background())
	p := &Pipeline{
		jobs:       make(chan Job, 64),
		onProgress: onProgress,
		runner:     ffmpegRunner,
		active:     newJobRegistry(),
		ctx:        ctx,
		cancel:     cancel,
	}

	for i := 0; i < workers; i++ {
		p.wg.Add(1)
		go p.worker()
	}
	return p
}

// Submit queues a job for processing. It reports whether the job was accepted:
// an item that already has work in flight is refused, because two jobs for one
// item would run two ffmpeg processes over the same output file.
func (p *Pipeline) Submit(job Job) bool {
	token, ok := p.active.reserve(job.MediaID)
	if !ok {
		return false
	}
	job.token = token

	p.onProgress(ProgressEvent{
		MediaID:  job.MediaID,
		Stage:    StageQueued,
		Strategy: job.Strategy.String(),
		Percent:  0,
		Message:  "queued",
	})

	select {
	case p.jobs <- job:
		return true
	case <-p.ctx.Done():
		// Shutting down: nothing will consume the queue, so drop the job's
		// reservation with it.
		p.active.release(job.MediaID, token)
		return false
	}
}

// CancelAndWait stops the job in flight for a media item and waits until it has
// stopped. A job still waiting in the queue is dropped before it can start.
//
// Callers that delete an item or replace its job must wait: an ffmpeg process
// keeps writing into the item's directory until it dies, so anything done to
// those files — or the status set for the item — races the job otherwise.
func (p *Pipeline) CancelAndWait(mediaID string) {
	if done := p.active.cancel(mediaID); done != nil {
		<-done
	}
}

// Close stops accepting work and waits for in-flight jobs to finish.
func (p *Pipeline) Close() {
	p.cancel()
	p.wg.Wait()
}

func (p *Pipeline) worker() {
	defer p.wg.Done()
	for {
		select {
		case <-p.ctx.Done():
			return
		case job, ok := <-p.jobs:
			if !ok {
				return
			}
			p.run(job)
		}
	}
}

func (p *Pipeline) run(job Job) {
	ctx, cancel := context.WithCancel(p.ctx)

	// Claim the reservation Submit made for this job. A job whose item was
	// deleted or re-queued while it waited must not start now: it would recreate
	// the directory a delete just removed, or race the job that replaced it.
	if !p.active.claim(job.MediaID, job.token, cancel) {
		cancel()
		return
	}
	defer p.active.release(job.MediaID, job.token)
	defer cancel()

	err := p.runner(ctx, job, p.onProgress)

	switch {
	case errors.Is(err, context.Canceled):
		// A cancel from a caller (delete, re-process) supersedes the job: the
		// caller is about to drop the row or start a replacement, so reporting a
		// status here would only flash one nobody is waiting for. Shutdown
		// cancels through the pipeline's own context, has nobody waiting to move
		// the item on, and still reports.
		if p.active.canceledByCaller(job.MediaID, job.token) {
			return
		}
		p.onProgress(ProgressEvent{
			MediaID:  job.MediaID,
			Stage:    StageCanceled,
			Strategy: job.Strategy.String(),
			Message:  "canceled",
		})
	case err != nil:
		p.fail(job, err.Error())
	default:
		p.onProgress(ProgressEvent{
			MediaID:  job.MediaID,
			Stage:    StageDone,
			Strategy: job.Strategy.String(),
			Percent:  100,
			Message:  "ready",
		})
	}
}

func (p *Pipeline) fail(job Job, msg string) {
	fmt.Printf("pipeline job %s failed: %s\n", job.MediaID, msg)
	p.onProgress(ProgressEvent{
		MediaID:  job.MediaID,
		Stage:    StageFailed,
		Strategy: job.Strategy.String(),
		Message:  msg,
	})
}

