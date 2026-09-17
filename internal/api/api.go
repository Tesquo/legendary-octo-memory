package api

import (
	"database/sql"

	"github.com/Tesquo/legendary-octo-memory/config"
	"github.com/Tesquo/legendary-octo-memory/internal/processing"
	"github.com/Tesquo/legendary-octo-memory/internal/storage"
	"github.com/Tesquo/legendary-octo-memory/internal/ws"
)

// MediaDir is where imported media and derived renditions live on disk.
const MediaDir = "media"

// MediaURLPrefix is the HTTP path under which MediaDir is served. It is shared
// by the router mount and relPath so the URL clients build and the route the
// server serves can never drift apart.
const MediaURLPrefix = "media-file"

// Server bundles the HTTP handlers' dependencies. Keeping them in one struct
// (instead of package globals) makes the API testable and explicit.
type Server struct {
	Config   config.Config
	Store    *storage.MediaStore
	Hub      *ws.Hub
	Pipeline *processing.Pipeline
}

// NewServer wires the store, the SSE progress hub, and the ffmpeg pipeline
// together. Pipeline progress is fed straight back into handleProgress.
func NewServer(db *sql.DB, cfg config.Config) *Server {
	s := &Server{
		Config: cfg,
		Store:  storage.NewMediaStore(db),
		Hub:    ws.NewHub(),
	}
	s.Pipeline = processing.NewPipeline(cfg.WorkerCount, s.handleProgress)
	return s
}

// handleProgress persists terminal job states and forwards every event to
// connected browsers. Non-terminal events skip the database to avoid a write on
// every ffmpeg progress tick.
func (s *Server) handleProgress(ev processing.ProgressEvent) {
	switch ev.Stage {
	case processing.StageDone:
		s.Store.SetPlayable(ev.MediaID, playablePath(ev.MediaID), storage.StatusReady)
	case processing.StageFailed, processing.StageCanceled:
		s.Store.SetStatus(ev.MediaID, storage.StatusFailed)
	}

	s.Hub.Publish(ev.MediaID, ev)
}
