package api

import (
	"mime"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// securityHeaders applies response headers that should hold for every route.
//
// Nothing this API serves is a document, and these say so. A Content-Security-
// Policy of default-src 'none' plus sandbox means that even if a stored file is
// rendered as a document anyway — the case nosniff alone cannot rule out, since
// a file whose extension already claims text/html needs no sniffing — it can run
// no script, load no subresource, and is treated as a unique origin, which is
// what keeps an uploaded page from calling this API as a same-origin client.
// frame-ancestors (with the older X-Frame-Options spelling for good measure)
// stops another site framing the API, and no-referrer keeps library URLs out of a
// third party's logs.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; sandbox")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// isStateChanging reports whether a method can mutate server state. Only those
// requests need the origin check below: a safe method cannot be turned into a
// write by a cross-site page, and CORS already stops that page from *reading*
// the response.
func isStateChanging(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

// isLoopbackHost reports whether an HTTP Host header names the loopback
// interface. The API only ever listens on loopback (see config.Host), so a
// request that arrives under any other name reached us through a hostname that
// resolves here — which is precisely the DNS-rebinding case, where the browser
// considers the attacker's page and this API to be the same origin and so skips
// CORS on the request entirely.
func isLoopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	// SplitHostPort strips the brackets from an IPv6 literal, but a bare "[::1]"
	// with no port is not split at all, so trim them by hand too.
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")

	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// originAllowed mirrors the CORS allow-list, "*" escape hatch included. The
// literal "null" origin (a sandboxed iframe, or a page opened from file://) is
// never a match.
func (s *Server) originAllowed(origin string) bool {
	if origin == "null" {
		return false
	}
	for _, allowed := range s.Config.AllowedOrigins {
		if allowed == "*" || strings.EqualFold(allowed, origin) {
			return true
		}
	}
	return false
}

// isSameOrigin reports whether an Origin equals the address the request was sent
// to. Same-origin requests need no allow-list entry, which keeps a UI served by
// the binary itself — or opened directly on the API's own port — working
// whatever PORT is configured. It cannot smuggle a cross-site request: by the
// time this is called, guardRequests has already established that Host names
// loopback, so matching means the caller is already running on this machine.
func isSameOrigin(r *http.Request, origin string) bool {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return strings.EqualFold(origin, scheme+"://"+r.Host)
}

// guardsHost reports whether the Host header check applies. It protects the
// default deployment, where the API is bound to loopback and the operator is
// relying on no other machine reaching it: rebinding is the way around that, and
// it works because the browser then treats this API as same-origin with a page
// on a domain that re-resolves to 127.0.0.1, which is exactly what the check
// refuses.
//
// Setting HOST to a routable address is a deliberate opt-in to serving the
// network, and there the Host header legitimately names this machine, so the
// check is skipped instead of locking every client out. The origin check below
// still applies in both modes, but note what it cannot do once the API is on the
// network: a rebound page's Origin and Host agree with each other, so it looks
// exactly like a legitimate client and only a per-request secret can separate
// them.
func (s *Server) guardsHost() bool {
	return isLoopbackHost(s.Config.Host)
}

// guardRequests is the server-side companion to the CORS middleware, which is
// only a browser-side hint: except for a preflight it forwards every request to
// the handler and merely withholds the Access-Control-Allow-* headers, so a
// cross-site form post to /upload would still create a file and start ffmpeg.
// The two checks here close that gap and the rebinding hole behind it:
//
//  1. Host must name loopback (see guardsHost).
//  2. A state-changing request that carries an Origin must come from an allowed
//     origin, or from this server's own origin.
//
// A state-changing request with no Origin at all is passed through: a visited
// page cannot forge that, and rejecting it would break non-browser callers like
// curl. Fetch Metadata is the backstop for the case where a browser sent no
// Origin, since a request made by a page on another site always says so.
func (s *Server) guardRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.guardsHost() && !isLoopbackHost(r.Host) {
			http.Error(w, "unexpected host", http.StatusMisdirectedRequest)
			return
		}

		if isStateChanging(r.Method) {
			if origin := r.Header.Get("Origin"); origin != "" {
				if !s.originAllowed(origin) && !isSameOrigin(r, origin) {
					http.Error(w, "cross-origin request refused", http.StatusForbidden)
					return
				}
			} else if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				http.Error(w, "cross-site request refused", http.StatusForbidden)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// validID reports whether id is an id this server could have issued. uuid.Parse
// accepts a few other spellings (braces, a URN prefix, no dashes); those are
// canonicalised away here, so what arrives must be exactly the form uuid.New
// produced. Ids are used to build filesystem paths, so a value that is not one of
// ours is refused rather than passed on to be looked up or joined.
func validID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed.String() == id
}

// validMediaID turns away a request whose {id} is not a UUID before any handler
// sees it. A malformed id is answered 404 rather than 400: from outside, "no such
// item" and "not an id at all" should be indistinguishable.
func validMediaID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validID(chi.URLParam(r, "id")) {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// attachmentDisposition builds a Content-Disposition value that forces a
// download. /open hands back the raw source under its original, user-supplied
// filename and extension, so rendering it inline would let an uploaded .html
// file run as a same-origin script. mime.FormatMediaType takes care of quoting
// and of non-ASCII filenames.
func attachmentDisposition(filename string) string {
	// The filename is client-supplied, so a CR or LF in it would otherwise land
	// inside a header value. Fold them away rather than relying on the transport
	// to refuse the value, which keeps this function's output safe on its own.
	filename = strings.NewReplacer("\r", " ", "\n", " ").Replace(filename)
	return mime.FormatMediaType("attachment", map[string]string{"filename": filename})
}
