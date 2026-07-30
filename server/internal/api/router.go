package api

import (
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Router builds the chi mux for the whole server: the JSON API under
// /api/v1, and the embedded static frontend under /. The Recoverer
// middleware is the last-resort panic guard (see error-handling spec).
func Router(h *Handlers, webFS fs.FS) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/report", h.Report)
		r.Get("/snapshot", h.Snapshot)
		r.Get("/stream", h.Stream)
		r.Get("/usage", h.Usage)
	})

	// Static frontend. Serve from the embedded filesystem at the root.
	// The API routes above take precedence.
	if webFS != nil {
		fileServer := http.FileServer(http.FS(webFS))
		r.Get("/*", fileServer.ServeHTTP)
	}

	return r
}
