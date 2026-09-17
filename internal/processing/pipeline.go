package processing

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"

	"github.com/Tesquo/legendary-octo-memory/internal/ffmpeg"
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
}

// Pipeline runs ffmpeg jobs on a bounded pool of goroutines. Each job can be
// cancelled individually, and progress is reported through a callback so the
// API layer can forward it over a websocket.
type Pipeline struct {
	jobs       chan Job
	wg         sync.WaitGroup
	onProgress ProgressFunc

	mu      sync.Mutex
	cancels map[string]context.CancelFunc

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
		cancels:    make(map[string]context.CancelFunc),
		ctx:        ctx,
		cancel:     cancel,
	}

	for i := 0; i < workers; i++ {
		p.wg.Add(1)
		go p.worker()
	}
	return p
}

// Submit queues a job for processing.
func (p *Pipeline) Submit(job Job) {
	p.onProgress(ProgressEvent{
		MediaID:  job.MediaID,
		Stage:    StageQueued,
		Strategy: job.Strategy.String(),
		Percent:  0,
		Message:  "queued",
	})

	select {
	case p.jobs <- job:
	case <-p.ctx.Done():
	}
}

// Cancel stops the running job for a given media ID, if any.
func (p *Pipeline) Cancel(mediaID string) {
	p.mu.Lock()
	cancel, ok := p.cancels[mediaID]
	p.mu.Unlock()
	if ok {
		cancel()
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
	// Register a per-job cancel so it can be stopped independently.
	ctx, cancel := context.WithCancel(p.ctx)
	p.mu.Lock()
	p.cancels[job.MediaID] = cancel
	p.mu.Unlock()

	defer func() {
		cancel()
		p.mu.Lock()
		delete(p.cancels, job.MediaID)
		p.mu.Unlock()
	}()

	if err := os.MkdirAll(filepath.Dir(job.OutputPath), 0755); err != nil {
		p.fail(job, fmt.Sprintf("create output dir: %v", err))
		return
	}

	args := buildArgs(job)
	if args == nil {
		p.fail(job, "unsupported strategy")
		return
	}

	p.onProgress(ProgressEvent{
		MediaID:  job.MediaID,
		Stage:    StageProcessing,
		Strategy: job.Strategy.String(),
		Percent:  0,
		Message:  job.Strategy.String(),
	})

	cmd := exec.CommandContext(ctx, ffmpeg.FFmpegPath(), args...)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		p.fail(job, fmt.Sprintf("stderr pipe: %v", err))
		return
	}

	if err := cmd.Start(); err != nil {
		p.fail(job, fmt.Sprintf("start ffmpeg: %v", err))
		return
	}

	// Tail ffmpeg's stderr for `time=HH:MM:SS.xx` and translate it into a
	// percentage of the known duration.
	go scanProgress(stderr, job, p.onProgress)

	waitErr := cmd.Wait()

	if ctx.Err() == context.Canceled {
		p.onProgress(ProgressEvent{
			MediaID:  job.MediaID,
			Stage:    StageCanceled,
			Strategy: job.Strategy.String(),
			Message:  "canceled",
		})
		return
	}

	if waitErr != nil {
		p.fail(job, fmt.Sprintf("ffmpeg: %v", waitErr))
		return
	}

	p.onProgress(ProgressEvent{
		MediaID:  job.MediaID,
		Stage:    StageDone,
		Strategy: job.Strategy.String(),
		Percent:  100,
		Message:  "ready",
	})
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

// buildArgs returns the ffmpeg arguments for a job, or nil if the strategy
// needs no work.
func buildArgs(job Job) []string {
	switch job.Strategy {
	case StrategyRemux:
		// Copy streams unchanged into a browser-friendly fragmented MP4.
		return []string{
			"-y",
			"-i", job.SourcePath,
			"-c", "copy",
			"-movflags", "+faststart",
			job.OutputPath,
		}
	case StrategyTranscode:
		// Re-encode to H.264/AAC, capping width at 1920 without upscaling.
		return []string{
			"-y",
			"-i", job.SourcePath,
			"-c:v", "libx264", "-preset", "veryfast", "-crf", "23",
			"-vf", "scale='min(1920,iw)':-2",
			"-c:a", "aac", "-b:a", "128k",
			"-movflags", "+faststart",
			job.OutputPath,
		}
	default:
		return nil
	}
}

var timeRe = regexp.MustCompile(`time=(\d+):(\d+):(\d+(?:\.\d+)?)`)

// scanProgress reads ffmpeg's stderr word by word and emits a percentage once
// per parsed timestamp. ffmpeg emits progress on stderr, not stdout.
func scanProgress(r io.Reader, job Job, onProgress ProgressFunc) {
	scanner := bufio.NewScanner(r)
	scanner.Split(bufio.ScanWords)

	for scanner.Scan() {
		m := timeRe.FindStringSubmatch(scanner.Text())
		if m == nil {
			continue
		}

		hours, _ := strconv.Atoi(m[1])
		mins, _ := strconv.Atoi(m[2])
		secs, _ := strconv.ParseFloat(m[3], 64)

		elapsed := float64(hours*3600+mins*60) + secs
		percent := 0.0
		if job.Duration > 0 {
			percent = (elapsed / job.Duration) * 100
		}
		if percent > 99 {
			percent = 99
		}
		if percent < 0 {
			percent = 0
		}

		onProgress(ProgressEvent{
			MediaID:  job.MediaID,
			Stage:    StageProcessing,
			Strategy: job.Strategy.String(),
			Percent:  percent,
			Message:  "processing",
		})
	}
}
