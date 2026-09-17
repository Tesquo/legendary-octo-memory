package processing

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		md       *Metadata
		expected Strategy
	}{
		{
			name:     "mp4 h264/aac is playable",
			path:     "movie.mp4",
			md:       &Metadata{VideoCodec: "h264", AudioCodec: "aac"},
			expected: StrategyPlayable,
		},
		{
			name:     "webm vp9/opus is playable",
			path:     "clip.webm",
			md:       &Metadata{VideoCodec: "vp9", AudioCodec: "opus"},
			expected: StrategyPlayable,
		},
		{
			name:     "mkv h264/aac only needs a remux",
			path:     "movie.mkv",
			md:       &Metadata{VideoCodec: "h264", AudioCodec: "aac"},
			expected: StrategyRemux,
		},
		{
			name:     "avi h264/mp3 only needs a remux",
			path:     "movie.avi",
			md:       &Metadata{VideoCodec: "h264", AudioCodec: "mp3"},
			expected: StrategyRemux,
		},
		{
			name:     "hevc in mp4 must be transcoded (codec)",
			path:     "movie.mp4",
			md:       &Metadata{VideoCodec: "hevc", AudioCodec: "aac"},
			expected: StrategyTranscode,
		},
		{
			name:     "h264 with incompatible audio must be transcoded",
			path:     "movie.mp4",
			md:       &Metadata{VideoCodec: "h264", AudioCodec: "ac3"},
			expected: StrategyTranscode,
		},
		{
			name:     "unknown video codec must be transcoded",
			path:     "movie.mp4",
			md:       &Metadata{VideoCodec: "mpeg2video", AudioCodec: "aac"},
			expected: StrategyTranscode,
		},
		{
			name:     "codecs compared case-insensitively",
			path:     "MOVIE.MP4",
			md:       &Metadata{VideoCodec: "H264", AudioCodec: "AAC"},
			expected: StrategyPlayable,
		},
		{
			name:     "video-only file with playable codec in mp4 is playable",
			path:     "silent.mp4",
			md:       &Metadata{VideoCodec: "h264"},
			expected: StrategyPlayable,
		},
		{
			name:     "nil metadata falls back to transcode",
			path:     "movie.mp4",
			md:       nil,
			expected: StrategyTranscode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.path, tt.md); got != tt.expected {
				t.Errorf("Classify(%q, %+v) = %s, want %s",
					tt.path, tt.md, got, tt.expected)
			}
		})
	}
}
