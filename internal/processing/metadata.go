package processing

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"

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

func ExtractMetadata(path string) (*Metadata, error) {
	cmd := exec.Command(ffmpeg.FFprobePath(),
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("ffprobe error:", string(out))
		return nil, err
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
