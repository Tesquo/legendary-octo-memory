package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
)

func NewRouter(s *Server) http.Handler {
	r := chi.NewRouter()

	// Security headers on every response, then the origin guard, then CORS. The
	// guard runs ahead of CORS on purpose: CORS only withholds response headers
	// from a disallowed origin, it does not reject the request, so on its own it
	// would still let a cross-site page post /upload or delete a media item.
	r.Use(securityHeaders)
	r.Use(s.guardRequests)

	// CORS so the Vite dev server (and any static host) can talk to the API.
	// Origins are configuration-driven so a deployed frontend can be added
	// without a code change.
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   s.Config.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
	}))

	// Health
	r.Get("/health", s.HealthHandler)

	// Library
	r.Get("/media", s.MediaListHandler)
	r.Get("/media/{id}", s.MediaDetailHandler)
	r.Delete("/media/{id}", s.DeleteHandler)

	// Ingestion and processing
	r.Post("/upload", s.UploadHandler)
	r.Post("/reprocess/{id}", s.ReprocessHandler)

	// Playback: serve the prepared rendition with HTTP range support so the
	// <video> element can seek instantly.
	r.Get("/play/{id}", s.PlayHandler)
	r.Head("/play/{id}", s.PlayHandler)

	// Raw source file, used by the sharing flow.
	r.Get("/open/{id}", s.OpenHandler)

	// Live pipeline progress (Server-Sent Events).
	r.Get("/progress", s.ProgressHandler)

	// Static media (thumbnails and renditions) via http.FileServer, which also
	// supports HTTP range requests. The mount path is shared with relPath so the
	// URLs clients build always match a real route.
	r.Handle("/"+MediaURLPrefix+"/*", http.StripPrefix("/"+MediaURLPrefix+"/", http.FileServer(http.Dir(MediaDir))))

	return r
}
