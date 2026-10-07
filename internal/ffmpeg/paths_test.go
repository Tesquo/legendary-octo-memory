package ffmpeg

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// findCase describes one layout for find to resolve. The filesystem and PATH are
// faked so every combination can be stated without creating files.
type findCase struct {
	name     string
	env      string
	present  []string
	onPath   string
	wantPath string
	wantOK   bool
}

// runFind drives one case for the given tool and reports what find resolved.
// Candidates are built from binaryName, so the expectations hold on every
// platform.
func runFind(t *testing.T, base string, tc findCase) (string, bool) {
	t.Helper()

	present := make(map[string]bool, len(tc.present))
	for _, p := range tc.present {
		present[p] = true
	}

	return find(base, search{
		env:      tc.env,
		exeDir:   filepath.Join(string(filepath.Separator)+"opt", "app", "bin"),
		cwd:      filepath.Join(string(filepath.Separator)+"home", "dev"),
		exists:   func(p string) bool { return present[p] },
		lookPath: func(string) (string, error) {
			if tc.onPath == "" {
				return "", errors.New("executable file not found in %PATH%")
			}
			return tc.onPath, nil
		},
	})
}

func runFindCases(t *testing.T, cases []findCase) {
	t.Helper()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotPath, gotOK := runFind(t, "ffmpeg", tc)
			if gotPath != tc.wantPath || gotOK != tc.wantOK {
				t.Errorf("find() = (%q, %v), want (%q, %v)", gotPath, gotOK, tc.wantPath, tc.wantOK)
			}
		})
	}
}

// TestFindPrefersBesideTheExecutable pins the search order, because it is what
// makes the shipped layout work (bin/server.exe beside bin/ffmpeg.exe) and what
// keeps the toolchain from being resolved against whatever directory the server
// happened to be started in.
func TestFindPrefersBesideTheExecutable(t *testing.T) {
	name := binaryName("ffmpeg")
	exeDir := filepath.Join(string(filepath.Separator)+"opt", "app", "bin")
	cwd := filepath.Join(string(filepath.Separator)+"home", "dev")
	usrBin := filepath.Join(string(filepath.Separator), "usr", "bin", name)

	runFindCases(t, []findCase{
		{
			name: "beside the executable beats every other location",
			present: []string{
				filepath.Join(exeDir, name),
				filepath.Join(exeDir, "bin", name),
				filepath.Join(cwd, "bin", name),
			},
			onPath:   usrBin,
			wantPath: filepath.Join(exeDir, name),
			wantOK:   true,
		},
		{
			name: "then bin/ under the executable",
			present: []string{
				filepath.Join(exeDir, "bin", name),
				filepath.Join(cwd, "bin", name),
			},
			onPath:   usrBin,
			wantPath: filepath.Join(exeDir, "bin", name),
			wantOK:   true,
		},
		{
			// The documented `go run ./cmd/server` layout: the executable is a
			// temp file, so only the working directory has the tools.
			name:     "then bin/ under the working directory",
			present:  []string{filepath.Join(cwd, "bin", name)},
			onPath:   usrBin,
			wantPath: filepath.Join(cwd, "bin", name),
			wantOK:   true,
		},
		{
			name:     "then PATH",
			onPath:   usrBin,
			wantPath: usrBin,
			wantOK:   true,
		},
		{
			// Not found is not fatal here: the caller reports it. What comes back
			// is the documented location, so the warning names somewhere the
			// operator can put the tool.
			name:     "nothing found names the documented location",
			wantPath: filepath.Join(cwd, "bin", name),
			wantOK:   false,
		},
	})
}

// TestFindHonoursTheEnvironmentOverride covers the escape hatch: an override wins
// outright so a pinned path is never silently replaced, and it is resolved rather
// than taken on faith so a typo is reported at startup instead of being misread as
// a bad upload later.
func TestFindHonoursTheEnvironmentOverride(t *testing.T) {
	name := binaryName("ffmpeg")
	exeDir := filepath.Join(string(filepath.Separator)+"opt", "app", "bin")
	cwd := filepath.Join(string(filepath.Separator)+"home", "dev")
	usrBin := filepath.Join(string(filepath.Separator), "usr", "bin", name)
	pinned := filepath.Join(string(filepath.Separator), "tools", name)

	runFindCases(t, []findCase{
		{
			name:     "a pinned path wins over everything else",
			env:      pinned,
			present:  []string{pinned, filepath.Join(exeDir, name), filepath.Join(cwd, "bin", name)},
			onPath:   usrBin,
			wantPath: pinned,
			wantOK:   true,
		},
		{
			name:     "a pinned path that is missing is reported, not trusted",
			env:      pinned,
			present:  []string{filepath.Join(exeDir, name)},
			wantPath: pinned,
			wantOK:   false,
		},
		{
			// A bare name is how exec.Command would look it up, so an override is
			// allowed to name a tool on PATH.
			name:     "a bare name is looked up on PATH",
			env:      "ffmpeg",
			onPath:   usrBin,
			wantPath: "ffmpeg",
			wantOK:   true,
		},
		{
			name:     "a bare name PATH does not know is reported",
			env:      "ffmpeg",
			wantPath: "ffmpeg",
			wantOK:   false,
		},
	})
}

// TestMissingToolErrorIsActionable keeps the startup warning useful: it has to
// name the tool, the search that failed, and the override that short-circuits it.
func TestMissingToolErrorIsActionable(t *testing.T) {
	tried := filepath.Join("cwd", "bin", binaryName("ffprobe"))
	msg := missingToolError("ffprobe", ffprobeEnv, tried).Error()

	for _, want := range []string{"ffprobe", ffprobeEnv, tried} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not mention %q", msg, want)
		}
	}
}
