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
	// without a code change. Credentials stay off: the API has no cookies and no
	// auth headers, so nothing needs them
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   s.Config.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: false,
	}))

	// Health
	r.Get("/health", s.HealthHandler)

	// Library. Every route that takes an {id} goes through validMediaID, which
	// turns away anything that is not a UUID this server issued before it can
	// reach the database or the filesystem.
	r.Get("/media", s.MediaListHandler)
	r.With(validMediaID).Get("/media/{id}", s.MediaDetailHandler)
	r.With(validMediaID).Delete("/media/{id}", s.DeleteHandler)

	// Ingestion and processing
	r.Post("/upload", s.UploadHandler)
	r.With(validMediaID).Post("/reprocess/{id}", s.ReprocessHandler)

	// Playback: serve the prepared rendition with HTTP range support so the
	// <video> element can seek instantly.
	r.With(validMediaID).Get("/play/{id}", s.PlayHandler)
	r.With(validMediaID).Head("/play/{id}", s.PlayHandler)

	// Raw source file, used by the sharing flow.
	r.With(validMediaID).Get("/open/{id}", s.OpenHandler)

	// Live pipeline progress (Server-Sent Events).
	r.Get("/progress", s.ProgressHandler)

	// Static media (thumbnails and renditions). serveMediaFile owns the
	// Content-Type instead of deriving it from the file extension, refuses
	// directory requests, and keeps every request inside media/<id>/. The mount
	// path is shared with relPath so the URLs clients build always match a real
	// route.
	r.Handle("/"+MediaURLPrefix+"/*", http.StripPrefix("/"+MediaURLPrefix+"/", http.HandlerFunc(s.serveMediaFile)))

	return r
}
