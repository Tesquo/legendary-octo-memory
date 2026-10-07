package processing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Tesquo/legendary-octo-memory/internal/ffmpeg"
)

type Metadata struct {
	Duration   float64
	Width      int
	Height     int
	VideoCodec string
	AudioCodec string
	FormatName string
	BitRate    int64
}

// probeTimeout bounds a single ffprobe run. A malformed file can make ffprobe
// hang, and without a bound the upload handler never answers while the item's
// directory stays behind.
const probeTimeout = 30 * time.Second

// ErrToolMissing reports that ffprobe could not be started at all, as opposed to
// the file being unreadable. The API needs the difference: a missing toolchain is
// this server's problem (500), not the uploader's (400).
var ErrToolMissing = errors.New("ffprobe executable not found")

func ExtractMetadata(path string) (*Metadata, error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, ffmpeg.FFprobePath(),
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	)

	// Output rather than CombinedOutput, so stderr cannot land in the JSON. Go
	// stashes stderr in the ExitError for the error below, which keeps a failure
	// diagnosable instead of silencing it as -v quiet did.
	out, err := cmd.Output()
	if err != nil {
		switch {
		case ctx.Err() != nil:
			return nil, fmt.Errorf("ffprobe timed out after %s: %w", probeTimeout, err)
		case errors.Is(err, exec.ErrNotFound), errors.Is(err, fs.ErrNotExist):
			return nil, fmt.Errorf("%w: %v", ErrToolMissing, err)
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil, fmt.Errorf("ffprobe: %w: %s", err, strings.TrimSpace(string(exit.Stderr)))
		}
		return nil, fmt.Errorf("ffprobe: %w", err)
	}

	var probe struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
		} `json:"streams"`
		Format struct {
			Duration   string `json:"duration"`
			FormatName string `json:"format_name"`
			BitRate    string `json:"bit_rate"`
		} `json:"format"`
	}

	if err := json.Unmarshal(out, &probe); err != nil {
		return nil, fmt.Errorf("parse ffprobe output: %w", err)
	}

	md := &Metadata{
		FormatName: probe.Format.FormatName,
	}

	// Duration (ffprobe reports it as a string)
	if probe.Format.Duration != "" {
		if d, err := strconv.ParseFloat(probe.Format.Duration, 64); err == nil {
			md.Duration = d
		}
	}

	// Overall bitrate, if the container reports one.
	if probe.Format.BitRate != "" {
		if br, err := strconv.ParseInt(probe.Format.BitRate, 10, 64); err == nil {
			md.BitRate = br
		}
	}

	// Streams
	for _, s := range probe.Streams {
		if s.CodecType == "video" && md.VideoCodec == "" {
			md.VideoCodec = s.CodecName
			md.Width = s.Width
			md.Height = s.Height
		}
		if s.CodecType == "audio" && md.AudioCodec == "" {
			md.AudioCodec = s.CodecName
		}
	}

	return md, nil
}
