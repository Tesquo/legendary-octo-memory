package processing

import (
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/Tesquo/legendary-octo-memory/internal/ffmpeg"
)

type Metadata struct {
	Duration   float64
	Width      int
	Height     int
	VideoCodec string
	AudioCodec string
}

func ExtractMetadata(path string) (*Metadata, error) {
	// path = filepath.ToSlash(path)
	// fmt.Println("Running ffprobe on:", path)

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
			Duration string `json:"duration"`
		} `json:"format"`
	}

	json.Unmarshal(out, &probe)

	md := &Metadata{}

	// Duration
	if probe.Format.Duration != "" {
		// convert string to float
		// (add strconv.ParseFloat here)
	}

	// Streams
	for _, s := range probe.Streams {
		if s.CodecType == "video" {
			md.VideoCodec = s.CodecName
			md.Width = s.Width
			md.Height = s.Height
		}
		if s.CodecType == "audio" {
			md.AudioCodec = s.CodecName
		}
	}

	return md, nil
}
