package processing

import (
	"context"
	"testing"
	"time"
)

// newTestPipeline builds a pipeline whose runner stands in for ffmpeg: these
// tests are about the pipeline's bookkeeping, not about encoding, and spawning
// real transcodes would make them slow and platform-dependent.
func newTestPipeline(t *testing.T, runner jobRunner) (*Pipeline, chan ProgressEvent) {
	t.Helper()

	events := make(chan ProgressEvent, 64)
	p := NewPipeline(1, func(ev ProgressEvent) { events <- ev })
	p.runner = runner
	t.Cleanup(p.Close)

	return p, events
}

// blockRunner announces the job it was given and then blocks until its context
// is cancelled, the way a running encode does. Tests name jobs by source path.
func blockRunner(started chan<- string) jobRunner {
	return func(ctx context.Context, job Job, _ ProgressFunc) error {
		started <- job.SourcePath
		<-ctx.Done()
		return ctx.Err()
	}
}

// holdRunner announces the job it was given and then blocks until the test lets
// it go, so the single worker stays busy and other jobs stay queued.
func holdRunner(started chan<- string, release <-chan struct{}) jobRunner {
	return func(ctx context.Context, job Job, _ ProgressFunc) error {
		started <- job.SourcePath

		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// hasStage drains the events reported so far and reports whether one of them was
// the wanted stage.
func hasStage(events <-chan ProgressEvent, want Stage) bool {
	for {
		select {
		case ev := <-events:
			if ev.Stage == want {
				return true
			}
		default:
			return false
		}
	}
}

func TestSubmitRefusesSecondJobForTheSameItem(t *testing.T) {
	started := make(chan string, 4)
	p, _ := newTestPipeline(t, blockRunner(started))

	if !p.Submit(Job{MediaID: "a", SourcePath: "first", Strategy: StrategyRemux}) {
		t.Fatal("Submit refused the first job")
	}
	if got := <-started; got != "first" {
		t.Fatalf("job %q started, want %q", got, "first")
	}

	// Without this the item would get two ffmpeg processes writing the same
	// output file.
	if p.Submit(Job{MediaID: "a", SourcePath: "second", Strategy: StrategyRemux}) {
		t.Error("Submit accepted a second job for an item that already has one in flight")
	}
}

func TestCancelAndWaitStopsRunningJobAndSilencesIt(t *testing.T) {
	started := make(chan string, 4)
	p, events := newTestPipeline(t, blockRunner(started))

	p.Submit(Job{MediaID: "a", SourcePath: "job", Strategy: StrategyRemux})
	<-started

	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		p.CancelAndWait("a")
	}()

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("CancelAndWait did not return for a cancelled job")
	}

	// The item is free again: that is what lets a caller re-process without
	// racing the job it just stopped.
	if !p.Submit(Job{MediaID: "a", SourcePath: "replacement", Strategy: StrategyRemux}) {
		t.Error("Submit refused a replacement job after CancelAndWait")
	}

	if hasStage(events, StageCanceled) {
		t.Error("a caller-initiated cancel reported a canceled event; the caller decides the item's outcome")
	}
}

func TestCancelDropsJobThatIsStillQueued(t *testing.T) {
	started := make(chan string, 4)
	release := make(chan struct{})
	p, _ := newTestPipeline(t, holdRunner(started, release))

	if !p.Submit(Job{MediaID: "a", SourcePath: "holder", Strategy: StrategyRemux}) {
		t.Fatal("Submit refused the holding job")
	}
	<-started // the only worker is busy, so the next jobs stay in the queue

	if !p.Submit(Job{MediaID: "b", SourcePath: "stale", Strategy: StrategyRemux}) {
		t.Fatal("Submit refused the queued job")
	}

	p.CancelAndWait("b")

	if !p.Submit(Job{MediaID: "b", SourcePath: "replacement", Strategy: StrategyRemux}) {
		t.Fatal("Submit refused the replacement job for a cancelled queued item")
	}

	close(release)

	// The queue is drained in order: the cancelled job is reached first, and it
	// must be dropped rather than started.
	select {
	case got := <-started:
		if got != "replacement" {
			t.Errorf("job %q ran; a job cancelled while queued must be dropped instead", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the replacement job never started")
	}
}

func TestShutdownReportsCanceledJob(t *testing.T) {
	started := make(chan string, 4)
	p, events := newTestPipeline(t, blockRunner(started))

	p.Submit(Job{MediaID: "a", SourcePath: "job", Strategy: StrategyRemux})
	<-started

	p.Close()

	// Nobody asked for this cancel, so the job still reports: an item cut short
	// by shutdown is worth seeing as failed rather than silently in progress.
	if !hasStage(events, StageCanceled) {
		t.Error("a job cut short by shutdown did not report a canceled event")
	}
}
