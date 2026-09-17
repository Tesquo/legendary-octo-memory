package processing

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/Tesquo/legendary-octo-memory/internal/ffmpeg"
)

// thumbnailWidth is the width thumbnails are scaled to. Height is derived to
// preserve aspect ratio (the "-2" in the scale filter keeps it even).
const thumbnailWidth = 480

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
	args := []string{"-y"}
	args = append(args, seekArgs...)
	args = append(args,
		"-i", videoPath,
		"-an", // no audio stream in the output
		"-frames:v", "1",
		"-vf", scale,
		thumbPath,
	)

	cmd := exec.Command(ffmpeg.FFmpegPath(), args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg thumbnail: %v: %s", err, string(out))
	}
	return nil
}
