package processing

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/Tesquo/legendary-octo-memory/internal/ffmpeg"
)

func GenerateThumbnail(videoPath, outputDir string) (string, error) {
	thumbPath := filepath.Join(outputDir, "thumb.jpg")

	cmd := exec.Command(
		ffmpeg.FFmpegPath(),
		"-i", videoPath,
		"-ss", "00:00:01",
		"-vframes", "1",
		thumbPath,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("ffmpeg thumbnail error:", string(out))
		return "", err
	}

	return thumbPath, nil
}
