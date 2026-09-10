package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func NewRouter() http.Handler {
	r := chi.NewRouter()

	// Basic routes
	r.Get("/health", HealthHandler)
	r.Get("/media", MediaListHandler)

	return r
}
