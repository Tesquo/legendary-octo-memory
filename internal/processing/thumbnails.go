package processing

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Tesquo/legendary-octo-memory/internal/ffmpeg"
)

// thumbnailWidth is the width thumbnails are scaled to. Height is derived to
// preserve aspect ratio (the "-2" in the scale filter keeps it even).
const thumbnailWidth = 480

// thumbnailTimeout bounds one attempt. A thumbnail is a convenience, so a file
// that makes ffmpeg hang is abandoned rather than occupying a goroutine forever.
const thumbnailTimeout = 60 * time.Second

// GenerateThumbnail writes a JPEG preview of videoPath into outputDir and
// returns its path. It attempts a frame ~1s in (a representative moment), but
// falls back to the very first frame so very short clips still get a preview
// instead of failing silently.
func GenerateThumbnail(videoPath, outputDir string) (string, error) {
	thumbPath := filepath.Join(outputDir, "thumb.jpg")

	scale := fmt.Sprintf("scale=%d:-2", thumbnailWidth)

	// Preferred: grab a frame just after the start.
	err := runThumbnail(videoPath, thumbPath, []string{"-ss", "00:00:01"}, scale)
	if err == nil {
		return thumbPath, nil
	}

	// Fallback: clips shorter than the seek point yield no frame, so retry from
	// the very beginning.
	if err := runThumbnail(videoPath, thumbPath, nil, scale); err != nil {
		return "", err
	}

	return thumbPath, nil
}

func runThumbnail(videoPath, thumbPath string, seekArgs []string, scale string) error {
	// -v error comes first because these are global options, and it keeps
	// ffmpeg's banner and progress chatter out of the buffer below.
	args := []string{"-y", "-v", "error"}
	args = append(args, seekArgs...)
	args = append(args,
		"-i", videoPath,
		"-an", // no audio stream in the output
		"-frames:v", "1",
		"-vf", scale,
		thumbPath,
	)

	ctx, cancel := context.WithTimeout(context.Background(), thumbnailTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, ffmpeg.FFmpegPath(), args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("ffmpeg thumbnail timed out after %s", thumbnailTimeout)
		}
		return fmt.Errorf("ffmpeg thumbnail: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
