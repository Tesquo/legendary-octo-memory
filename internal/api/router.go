package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
)

func NewRouter() http.Handler {
	r := chi.NewRouter()

	// Cors (For React)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
	}))

	// Basic routes
	r.Get("/health", HealthHandler)

	r.Get("/media", MediaListHandler)
	r.Get("/media/{id}", MediaDetailHandler)
	r.Post("/upload", UploadHandler)

	r.Handle("/media/*", http.StripPrefix("/media/", http.FileServer(http.Dir("media"))))

	return r
}
