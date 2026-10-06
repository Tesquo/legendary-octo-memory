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
	// Host is the interface the HTTP server binds to. It defaults to loopback
	// because the API is unauthenticated and only the host's own browser is
	// meant to reach it: a viewer must never call it (see the sharing design).
	// Set HOST=0.0.0.0 to serve the LAN deliberately; the request guard can no
	// longer verify the Host header in that mode (see internal/api/guard.go).
	// Env: HOST.
	Host string

	// Port is the TCP port the HTTP server listens on. Env: PORT.
	Port string

	// WorkerCount is the number of concurrent ffmpeg preparation jobs.
	// Env: WORKER_COUNT.
	WorkerCount int

	// MaxUploadBytes caps the size of a single uploaded file. Env: MAX_UPLOAD_BYTES.
	MaxUploadBytes int64

	// AllowedOrigins are the origins permitted to make state-changing calls to
	// the API. They feed both the CORS response headers and the server-side
	// origin guard, which is what actually rejects a disallowed cross-site
	// request (see internal/api/guard.go). Origin only, no path.
	// Env: ALLOWED_ORIGINS (comma-separated).
	AllowedOrigins []string
}

// Load reads configuration from the environment, applying defaults for any
// value that is unset or unparseable.
func Load() Config {
	return Config{
		Host:           envString("HOST", "127.0.0.1"),
		Port:           envString("PORT", "8080"),
		WorkerCount:    envInt("WORKER_COUNT", 2),
		MaxUploadBytes: envInt64("MAX_UPLOAD_BYTES", 8<<30), // 8 GiB
		// Defaults cover the Vite dev server and the published origin the host's
		// browser loads when it runs the static UI. A UI served by this binary
		// itself needs no entry here: a same-origin request is always accepted.
		// Override with your own deployment's origin (origin only, no path).
		AllowedOrigins: envStringSlice("ALLOWED_ORIGINS", []string{
			"http://localhost:5173",
			"https://tesquo.github.io",
		}),
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
