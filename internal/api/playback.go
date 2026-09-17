package api

import (
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
)

// PlayHandler serves the browser-playable rendition for a media item. It falls
// back to the original source file if no rendition has been produced yet (for
// example when the source was already browser-playable).
//
// http.ServeFile handles HTTP range requests, which is what allows the <video>
// element to seek without downloading the whole file.
func (s *Server) PlayHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	m, err := s.Store.Get(id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	switch {
	case m.PlayablePath != "" && fileExists(m.PlayablePath):
		http.ServeFile(w, r, m.PlayablePath)
	case fileExists(sourcePath(m.ID, m.Filename)):
		http.ServeFile(w, r, sourcePath(m.ID, m.Filename))
	default:
		// Not prepared yet: tell the client to retry after processing.
		w.Header().Set("Retry-After", "2")
		http.Error(w, "not ready", http.StatusConflict)
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
