package processing

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/Tesquo/legendary-octo-memory/internal/ffmpeg"
)

// errUnsupportedStrategy is returned for a job whose strategy asks for no work.
var errUnsupportedStrategy = errors.New("unsupported strategy")

// ffmpegRunner is the production jobRunner: it prepares the output directory,
// spawns ffmpeg, and translates its stderr into progress events. A cancelled
// context (Pipeline.CancelAndWait, or shutdown) kills the process through
// exec.CommandContext and is reported as the cancellation it is.
func ffmpegRunner(ctx context.Context, job Job, onProgress ProgressFunc) error {
	if err := os.MkdirAll(filepath.Dir(job.OutputPath), 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	args := buildArgs(job)
	if args == nil {
		return errUnsupportedStrategy
	}

	onProgress(ProgressEvent{
		MediaID:  job.MediaID,
		Stage:    StageProcessing,
		Strategy: job.Strategy.String(),
		Percent:  0,
		Message:  job.Strategy.String(),
	})

	cmd := exec.CommandContext(ctx, ffmpeg.FFmpegPath(), args...)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		// A cancel landing between the steps above and this start surfaces here
		// as a start failure; report the cancellation it really is.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("start ffmpeg: %w", err)
	}

	// Tail ffmpeg's stderr for `time=HH:MM:SS.xx` and translate it into a
	// percentage of the known duration.
	go scanProgress(stderr, job, onProgress)

	waitErr := cmd.Wait()

	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}

	if waitErr != nil {
		return fmt.Errorf("ffmpeg: %w", waitErr)
	}

	return nil
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
