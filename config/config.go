package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds runtime settings. Values are loaded once at startup from the
// environment, falling back to sensible local-development defaults. New settings
// (including future user-facing preferences) should be added here so there is a
// single, discoverable place for configuration.
type Config struct {
	// Port is the TCP port the HTTP server listens on. Env: PORT.
	Port string

	// WorkerCount is the number of concurrent ffmpeg preparation jobs.
	// Env: WORKER_COUNT.
	WorkerCount int

	// MaxUploadBytes caps the size of a single uploaded file. Env: MAX_UPLOAD_BYTES.
	MaxUploadBytes int64

	// AllowedOrigins are the CORS origins permitted to call the API.
	// Env: ALLOWED_ORIGINS (comma-separated).
	AllowedOrigins []string
}

// Load reads configuration from the environment, applying defaults for any
// value that is unset or unparseable.
func Load() Config {
	return Config{
		Port:           envString("PORT", "8080"),
		WorkerCount:    envInt("WORKER_COUNT", 2),
		MaxUploadBytes: envInt64("MAX_UPLOAD_BYTES", 8<<30), // 8 GiB
		AllowedOrigins: envStringSlice("ALLOWED_ORIGINS", []string{"http://localhost:5173"}),
	}
}

func envString(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

func envStringSlice(key string, fallback []string) []string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}
