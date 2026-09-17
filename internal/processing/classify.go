package processing

import (
	"path/filepath"
	"strings"
)

// Strategy describes how a source file must be prepared before a browser can
// play it via a <video> element.
type Strategy int

const (
	// StrategyPlayable means the browser can play the file as-is.
	StrategyPlayable Strategy = iota
	// StrategyRemux means the codecs are browser-compatible but the container
	// is not, so we copy the streams into a fragmented MP4 (fast, lossless).
	StrategyRemux
	// StrategyTranscode means at least one codec is incompatible, so we must
	// re-encode to H.264/AAC (slower, lossy).
	StrategyTranscode
)

func (s Strategy) String() string {
	switch s {
	case StrategyPlayable:
		return "playable"
	case StrategyRemux:
		return "remux"
	case StrategyTranscode:
		return "transcode"
	default:
		return "unknown"
	}
}

// playableVideoCodecs are the video codecs that modern browsers can decode.
var playableVideoCodecs = map[string]bool{
	"h264": true,
	"vp8":  true,
	"vp9":  true,
	"av1":  true,
}

// playableAudioCodecs are the audio codecs that modern browsers can decode.
var playableAudioCodecs = map[string]bool{
	"aac":    true,
	"mp3":    true,
	"opus":   true,
	"vorbis": true,
}

// playableContainers are the container extensions browsers handle natively.
var playableContainers = map[string]bool{
	".mp4":  true,
	".m4v":  true,
	".mov":  true,
	".webm": true,
	".ogv":  true,
	".ogg":  true,
}

// Classify inspects a file's metadata and extension to decide how it must be
// prepared for browser playback.
//
// A file is "playable" only when both its container AND all of its codecs are
// browser-friendly. Note we deliberately do not trust ffprobe's format_name
// alone: a Matroska file can report "matroska,webm" while still containing
// incompatible streams, so we gate on the file extension instead.
func Classify(path string, md *Metadata) Strategy {
	if md == nil {
		return StrategyTranscode
	}

	codecsOK := playableVideoCodecs[strings.ToLower(md.VideoCodec)] &&
		(md.AudioCodec == "" || playableAudioCodecs[strings.ToLower(md.AudioCodec)])

	if !codecsOK {
		// Incompatible codec(s): only a full re-encode will help.
		return StrategyTranscode
	}

	ext := strings.ToLower(filepath.Ext(path))
	if playableContainers[ext] {
		return StrategyPlayable
	}

	// Codecs are fine, but the container is not (e.g. MKV): remux only.
	return StrategyRemux
}
