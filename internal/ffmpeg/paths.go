// Package ffmpeg locates the ffmpeg and ffprobe executables this server runs.
package ffmpeg

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Environment overrides. Setting one selects that binary and skips the search;
// it still has to resolve, so a typo is reported at startup like any other
// missing tool.
const (
	ffmpegEnv  = "FFMPEG_PATH"
	ffprobeEnv = "FFPROBE_PATH"
)

// search describes everywhere a binary may be looked for. Passing it in rather
// than reading the environment and the filesystem inside the search is what
// makes the order below testable.
type search struct {
	env      string // explicit override, empty when unset
	exeDir   string // directory holding the running executable
	cwd      string // process working directory
	exists   func(string) bool
	lookPath func(string) (string, error)
}

// binaryName is this platform's name for a tool.
func binaryName(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

// find resolves one binary. In order:
//
//  1. the environment override, and it wins outright rather than being one more
//     candidate, so a pinned path never falls through to a different binary,
//  2. beside the running executable (the layout this repo ships: bin/server.exe
//     next to bin/ffmpeg.exe),
//  3. bin/ under the executable,
//  4. bin/ under the working directory (the documented `go run ./cmd/server`
//     development layout),
//  5. PATH.
//
// The middle three matter because exec.Command only searches PATH for a name
// containing no separator: a bare "bin/ffmpeg.exe" is resolved against whatever
// directory the server happened to be started in. Started anywhere else, every
// upload failed for no visible reason; started with an attacker-writable working
// directory, the toolchain itself became replaceable.
func find(base string, s search) (string, bool) {
	// An explicit override is authoritative: it is not one more candidate, so a
	// deployment that pins a path never silently ends up running a different
	// binary. It is still resolved before being trusted — a bare name is looked
	// up on PATH, anything with a separator has to exist.
	if s.env != "" {
		return s.env, resolves(s.env, s)
	}

	name := binaryName(base)

	var candidates []string
	if s.exeDir != "" {
		candidates = append(candidates,
			filepath.Join(s.exeDir, name),
			filepath.Join(s.exeDir, "bin", name),
		)
	}
	if s.cwd != "" {
		candidates = append(candidates, filepath.Join(s.cwd, "bin", name))
	}
	for _, candidate := range candidates {
		if s.exists(candidate) {
			return candidate, true
		}
	}

	if s.lookPath != nil {
		if found, err := s.lookPath(base); err == nil {
			return found, true
		}
	}

	// Not found. Hand back the documented location so the failure names
	// something the operator can act on.
	return filepath.Join(s.cwd, "bin", name), false
}

// resolves reports whether an explicit override names something runnable. A
// value with no separator is a name for PATH to find — that is how exec.Command
// would treat it — and anything else is a path that has to exist.
func resolves(override string, s search) bool {
	if !strings.ContainsRune(override, filepath.Separator) && !strings.ContainsRune(override, '/') {
		if s.lookPath == nil {
			return false
		}
		_, err := s.lookPath(override)
		return err == nil
	}
	return s.exists(override)
}

// Resolution is memoised: neither the environment nor the layout can change
// while the server is running, and every job would otherwise re-stat the tree.
var (
	ffmpegOnce  sync.Once
	ffmpegPath  string
	ffmpegFound bool

	ffprobeOnce  sync.Once
	ffprobePath  string
	ffprobeFound bool
)

// FFmpegPath returns the ffmpeg executable this process will run.
func FFmpegPath() string {
	ffmpegOnce.Do(func() { ffmpegPath, ffmpegFound = resolve("ffmpeg", ffmpegEnv) })
	return ffmpegPath
}

// FFprobePath returns the ffprobe executable this process will run.
func FFprobePath() string {
	ffprobeOnce.Do(func() { ffprobePath, ffprobeFound = resolve("ffprobe", ffprobeEnv) })
	return ffprobePath
}

// resolve runs find against the real environment and filesystem.
func resolve(base, envKey string) (string, bool) {
	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}
	cwd, _ := os.Getwd()
	return find(base, search{
		env:      os.Getenv(envKey),
		exeDir:   exeDir,
		cwd:      cwd,
		exists:   fileExists,
		lookPath: exec.LookPath,
	})
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// Verify reports whether both tools were found. main calls it at startup so a
// missing toolchain is stated once, up front, instead of surfacing as an opaque
// failure on every upload.
func Verify() error {
	for _, tool := range []struct {
		base  string
		env   string
		path  string
		found bool
	}{
		{"ffmpeg", ffmpegEnv, FFmpegPath(), ffmpegFound},
		{"ffprobe", ffprobeEnv, FFprobePath(), ffprobeFound},
	} {
		if !tool.found {
			return missingToolError(tool.base, tool.env, tool.path)
		}
	}
	return nil
}

// missingToolError names both the search that failed and the override that
// sidesteps it.
func missingToolError(base, envKey, tried string) error {
	return fmt.Errorf("%s not found (looked beside the executable, in ./bin, and on PATH): set %s, or install it at %s", base, envKey, tried)
}
