package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tesquo/legendary-octo-memory/config"
)

// guardCase describes one request the origin guard has to judge. An empty
// origin or fetchSite means the header is not sent at all, which is what a
// non-browser client such as curl looks like.
type guardCase struct {
	name      string
	method    string
	host      string
	origin    string
	fetchSite string
	want      int
}

// loopbackConfig is the default deployment: bound to loopback, with the given
// origin allow-list.
func loopbackConfig(origins ...string) config.Config {
	return config.Config{Host: "127.0.0.1", AllowedOrigins: origins}
}

// runGuardCases drives each case through the guard and asserts both the status
// and whether the wrapped handler actually ran. The second assertion is the
// point: a rejected request must not reach the handler, which is exactly what
// the CORS middleware alone fails to guarantee.
func runGuardCases(t *testing.T, cfg config.Config, cases []guardCase) {
	t.Helper()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reached := false
			s := &Server{Config: cfg}
			handler := s.guardRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reached = true
				w.WriteHeader(http.StatusOK)
			}))

			r := httptest.NewRequest(tc.method, "http://localhost:8080/upload", nil)
			r.Host = tc.host
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if tc.fetchSite != "" {
				r.Header.Set("Sec-Fetch-Site", tc.fetchSite)
			}

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)

			if w.Code != tc.want {
				t.Errorf("%s Host=%q Origin=%q Sec-Fetch-Site=%q: status = %d, want %d",
					tc.method, tc.host, tc.origin, tc.fetchSite, w.Code, tc.want)
			}
			if wantReached := tc.want == http.StatusOK; reached != wantReached {
				t.Errorf("%s Host=%q Origin=%q Sec-Fetch-Site=%q: handler reached = %v, want %v",
					tc.method, tc.host, tc.origin, tc.fetchSite, reached, wantReached)
			}
		})
	}
}

// TestIsLoopbackHost pins the door on DNS rebinding: the API only listens on
// loopback, so any other Host reached us through a name that resolves here, and
// a browser would treat that name as same-origin with the API.
func TestIsLoopbackHost(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{host: "localhost:8080", want: true},
		{host: "localhost", want: true},
		{host: "LOCALHOST:8080", want: true},
		{host: "127.0.0.1:8080", want: true},
		{host: "127.0.0.1", want: true},
		{host: "127.9.9.9:8080", want: true}, // all of 127.0.0.0/8 is loopback
		{host: "[::1]:8080", want: true},
		{host: "[::1]", want: true},
		{host: "::1", want: true},
		{host: "", want: false},
		{host: "evil.com:8080", want: false},
		{host: "evil.com", want: false},
		{host: "localhost.evil.com:8080", want: false},
		{host: "192.168.1.20:8080", want: false},
		{host: "0.0.0.0:8080", want: false},
		{host: "[fe80::1]:8080", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := isLoopbackHost(tt.host); got != tt.want {
				t.Errorf("isLoopbackHost(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

func TestGuardRequestsRejectsForeignHost(t *testing.T) {
	runGuardCases(t, loopbackConfig("https://tesquo.github.io"), []guardCase{
		{name: "public host on a read", method: http.MethodGet, host: "evil.com:8080", want: http.StatusMisdirectedRequest},
		{name: "public host on a write", method: http.MethodPost, host: "evil.com:8080", origin: "http://evil.com:8080", want: http.StatusMisdirectedRequest},
		{name: "matching origin does not help when the host is foreign", method: http.MethodPost, host: "evil.com", origin: "http://evil.com", want: http.StatusMisdirectedRequest},
		{name: "lan address", method: http.MethodPost, host: "192.168.1.20:8080", origin: "http://192.168.1.20:8080", want: http.StatusMisdirectedRequest},
		{name: "wildcard bind address is not an address clients use", method: http.MethodPost, host: "0.0.0.0:8080", origin: "http://0.0.0.0:8080", want: http.StatusMisdirectedRequest},
		{name: "missing host header", method: http.MethodPost, host: "", want: http.StatusMisdirectedRequest},
		{name: "localhost lookalike domain", method: http.MethodPost, host: "localhost.evil.com:8080", origin: "http://localhost.evil.com:8080", want: http.StatusMisdirectedRequest},
	})
}

func TestGuardRequestsRejectsCrossOriginWrites(t *testing.T) {
	runGuardCases(t, loopbackConfig("https://tesquo.github.io"), []guardCase{
		{name: "form post from another site", method: http.MethodPost, host: "localhost:8080", origin: "https://evil.com", want: http.StatusForbidden},
		{name: "delete from another site", method: http.MethodDelete, host: "localhost:8080", origin: "https://evil.com", want: http.StatusForbidden},
		{name: "reprocess from another site", method: http.MethodPost, host: "localhost:8080", origin: "https://evil.com", want: http.StatusForbidden},
		{name: "origin stripped, fetch metadata says cross-site", method: http.MethodPost, host: "localhost:8080", fetchSite: "cross-site", want: http.StatusForbidden},
		{name: "sandboxed iframe null origin", method: http.MethodPost, host: "localhost:8080", origin: "null", want: http.StatusForbidden},
		{name: "same site on a port that is not allow-listed", method: http.MethodPost, host: "localhost:8080", origin: "http://localhost:5174", want: http.StatusForbidden},
		{name: "suffix of an allowed origin", method: http.MethodPost, host: "localhost:8080", origin: "https://tesquo.github.io.evil.com", want: http.StatusForbidden},
	})
}

func TestGuardRequestsAcceptsLoopbackAndAllowedOrigins(t *testing.T) {
	runGuardCases(t, loopbackConfig("https://tesquo.github.io"), []guardCase{
		{name: "non-browser client sends no origin", method: http.MethodPost, host: "localhost:8080", want: http.StatusOK},
		{name: "ipv4 loopback", method: http.MethodPost, host: "127.0.0.1:8080", want: http.StatusOK},
		{name: "ipv6 loopback", method: http.MethodPost, host: "[::1]:8080", want: http.StatusOK},
		{name: "own origin on a non-default port", method: http.MethodPost, host: "localhost:9000", origin: "http://localhost:9000", want: http.StatusOK},
		{name: "allow-listed deployment origin", method: http.MethodPost, host: "localhost:8080", origin: "https://tesquo.github.io", want: http.StatusOK},
		{name: "allow-listed origin in different case", method: http.MethodPost, host: "localhost:8080", origin: "https://Tesquo.GitHub.io", want: http.StatusOK},
		{
			// The host's own browser loads the published UI, so its writes to
			// localhost genuinely are cross-site. A blanket cross-site rejection
			// would break the product, which is why the origin decides.
			name:      "cross-site fetch metadata with an allowed origin",
			method:    http.MethodPost,
			host:      "localhost:8080",
			origin:    "https://tesquo.github.io",
			fetchSite: "cross-site",
			want:      http.StatusOK,
		},
		{
			// Reads are left to CORS: a viewer's <video> pulls /play and
			// /media-file without an Origin header, and the rebinding hole is
			// closed by the host check instead.
			name:   "read from an arbitrary origin",
			method: http.MethodGet,
			host:   "localhost:8080",
			origin: "https://evil.com",
			want:   http.StatusOK,
		},
	})
}

func TestGuardRequestsHonoursWildcardOrigin(t *testing.T) {
	runGuardCases(t, loopbackConfig("*"), []guardCase{
		{name: "wildcard accepts any origin", method: http.MethodPost, host: "localhost:8080", origin: "https://evil.com", want: http.StatusOK},
		{name: "host check still applies under a wildcard", method: http.MethodPost, host: "evil.com:8080", origin: "https://evil.com", want: http.StatusMisdirectedRequest},
	})
}

func TestIsStateChanging(t *testing.T) {
	tests := []struct {
		method string
		want   bool
	}{
		{method: http.MethodGet, want: false},
		{method: http.MethodHead, want: false},
		{method: http.MethodOptions, want: false},
		{method: http.MethodPost, want: true},
		{method: http.MethodPut, want: true},
		{method: http.MethodDelete, want: true},
		{method: http.MethodPatch, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			if got := isStateChanging(tt.method); got != tt.want {
				t.Errorf("isStateChanging(%q) = %v, want %v", tt.method, got, tt.want)
			}
		})
	}
}

func TestSecurityHeadersSetsNosniff(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://localhost:8080/media-file/x/thumb.jpg", nil))

	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want %q", got, "nosniff")
	}
}

// TestAttachmentDispositionForcesDownload keeps /open from becoming an
// XSS foothold: the raw source is served under a filename the user chose, so it
// must never render inline in the API's origin.
func TestAttachmentDispositionForcesDownload(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     string
	}{
		{name: "plain name", filename: "movie.mp4", want: "attachment; filename=movie.mp4"},
		{name: "html is not special-cased", filename: "payload.html", want: "attachment; filename=payload.html"},
		{name: "quote is escaped rather than ending the value", filename: `we"ird.mp4`, want: `attachment; filename="we\"ird.mp4"`},
		{name: "crlf is folded so no header can be injected", filename: "bad\r\nInjected: 1", want: `attachment; filename="bad  Injected: 1"`},
		{name: "non-ascii uses the extended form", filename: "café.mp4", want: "attachment; filename*=utf-8''caf%C3%A9.mp4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := attachmentDisposition(tt.filename); got != tt.want {
				t.Errorf("attachmentDisposition(%q) = %q, want %q", tt.filename, got, tt.want)
			}
		})
	}
}

// TestRouterWiresTheGuards goes through the real router, because the unit tests
// above pass just as well if the middleware was never mounted.
func TestRouterWiresTheGuards(t *testing.T) {
	router := NewRouter(&Server{Config: loopbackConfig("http://localhost:5173")})

	t.Run("loopback host reaches the handler", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://localhost:8080/health", nil))

		if w.Code != http.StatusOK {
			t.Errorf("GET /health status = %d, want %d", w.Code, http.StatusOK)
		}
		if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("X-Content-Type-Options = %q, want %q", got, "nosniff")
		}
	})

	t.Run("foreign host is refused", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://evil.com:8080/health", nil))

		if w.Code != http.StatusMisdirectedRequest {
			t.Errorf("GET /health via evil.com = %d, want %d", w.Code, http.StatusMisdirectedRequest)
		}
		if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("rejected requests should still carry the security header, got %q", got)
		}
	})

	t.Run("cross-site upload is refused", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "http://localhost:8080/upload", nil)
		r.Header.Set("Origin", "https://evil.com")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)

		// Without the guard this reaches UploadHandler, which is the whole point:
		// CORS alone would have run it and merely hidden the answer.
		if w.Code != http.StatusForbidden {
			t.Errorf("POST /upload from evil.com = %d, want %d", w.Code, http.StatusForbidden)
		}
	})
}

// TestGuardRequestsWhenBoundToTheNetwork pins the HOST=0.0.0.0 opt-in. The Host
// header check protects the loopback-only default, and skipping it there must not
// skip the origin check too: that is what still stands between a cross-site page
// and the library once the API is on the network. The last case records what that
// mode cannot do, so nobody assumes it is covered.
func TestGuardRequestsWhenBoundToTheNetwork(t *testing.T) {
	networked := config.Config{Host: "0.0.0.0", AllowedOrigins: []string{"http://localhost:5173"}}

	runGuardCases(t, networked, []guardCase{
		{name: "lan client is not locked out", method: http.MethodGet, host: "192.168.1.20:8080", want: http.StatusOK},
		{name: "lan client can post without an origin", method: http.MethodPost, host: "192.168.1.20:8080", want: http.StatusOK},
		{name: "the origin check still applies", method: http.MethodPost, host: "192.168.1.20:8080", origin: "https://evil.com", want: http.StatusForbidden},
		{name: "allow-listed origin still works", method: http.MethodPost, host: "192.168.1.20:8080", origin: "http://localhost:5173", want: http.StatusOK},
		{name: "a page on another origin is still refused", method: http.MethodPost, host: "evil.com:8080", origin: "http://evil.com", want: http.StatusForbidden},
		{
			// A pinned limitation, not an endorsement. Rebound, the attacker's page
			// sees its own Origin and Host agree, which is indistinguishable from a
			// legitimate LAN client, so the allow-list cannot judge it. Loopback
			// mode is protected by the host check; this mode needs a per-request
			// secret instead of another header rule.
			name:   "sameness cannot be judged once the API is on the network",
			method: http.MethodPost,
			host:   "evil.com:8080",
			origin: "http://evil.com:8080",
			want:   http.StatusOK,
		},
	})
}
