package ffmpeg

import (
	"path/filepath"
	"runtime"
)

func FFprobePath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join("bin", "ffprobe.exe")
	}
	return filepath.Join("bin", "ffprobe")
}

func FFmpegPath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join("bin", "ffmpeg.exe")
	}
	return filepath.Join("bin", "ffmpeg")
}
