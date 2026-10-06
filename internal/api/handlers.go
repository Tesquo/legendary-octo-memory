package api

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
)

func (s *Server) HealthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) MediaListHandler(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.List()
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	// Resolve stored on-disk paths into URL-friendly paths for the client.
	for i := range items {
		items[i].Thumbnail = relPath(items[i].Thumbnail)
		items[i].PlayablePath = relPath(items[i].PlayablePath)
	}

	writeJSON(w, http.StatusOK, items)
}

func (s *Server) MediaDetailHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	m, err := s.Store.Get(id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	m.Thumbnail = relPath(m.Thumbnail)
	m.PlayablePath = relPath(m.PlayablePath)

	writeJSON(w, http.StatusOK, m)
}

// ProgressHandler streams pipeline progress over Server-Sent Events.
func (s *Server) ProgressHandler(w http.ResponseWriter, r *http.Request) {
	s.Hub.ServeSSE(w, r)
}

// ReprocessHandler re-runs the preparation pipeline for a media item, e.g.
// after a failed job or if the user wants to switch from remux to transcode.
func (s *Server) ReprocessHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	m, err := s.Store.Get(id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	src := sourcePath(m.ID, m.Filename)
	if _, err := os.Stat(src); err != nil {
		http.Error(w, "source file missing", http.StatusGone)
		return
	}

	// The pipeline keeps one job per item: a job already running (or still
	// queued) is stopped and waited for before the replacement is submitted, so
	// two ffmpeg processes never write the same output file.
	s.Pipeline.CancelAndWait(m.ID)

	s.enqueueJob(m.ID, src, m.Duration)
	writeJSON(w, http.StatusAccepted, map[string]string{
		"id":     m.ID,
		"status": "queued",
	})
}

// OpenHandler hands the raw source file back to the browser. Nothing in the UI
// calls it today: the player streams the prepared rendition via /play, and the
// share flow captures the host's <video> element rather than fetching a file.
// It is kept as the escape hatch for reading the (possibly incompatible)
// original when no playable rendition exists.
func (s *Server) OpenHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	m, err := s.Store.Get(id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Force a download. The escape hatch hands back the original file under its
	// own name and extension, and a filename a user chose may well end in .html
	// or .svg — rendering that inline would execute it in the API's origin.
	if disposition := attachmentDisposition(m.Filename); disposition != "" {
		w.Header().Set("Content-Disposition", disposition)
	}

	http.ServeFile(w, r, sourcePath(m.ID, m.Filename))
}

// DeleteHandler removes a media item from the library and deletes its files.
func (s *Server) DeleteHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if _, err := s.Store.Get(id); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if err := s.Store.Delete(id); err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	// Then stop the item's job and wait for it. The row goes first so any
	// progress event the job still emits lands on nothing, but the files can only
	// be removed once ffmpeg has exited: a running job keeps writing into
	// media/<id>/ and would recreate the directory RemoveAll is about to delete.
	s.Pipeline.CancelAndWait(id)

	// Best-effort cleanup of artefacts. A failure here leaves orphaned files
	// but the item is already gone from the user's library, so it is not fatal.
	if err := os.RemoveAll(mediaDir(id)); err != nil {
		http.Error(w, "delete files: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}
