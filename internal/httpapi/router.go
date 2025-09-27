package httpapi

import (
    "net/http"

    chi "github.com/go-chi/chi/v5"
    chimw "github.com/go-chi/chi/v5/middleware"
    "github.com/tinoosan/ledger/internal/service/journal"
)

// Server wires handlers and middleware using Chi.
type Server struct {
    svc    journal.Service
    rt     *chi.Mux
}

// New constructs the HTTP server with routes and middleware.
func New(repo Repository, writer Writer) *Server {
    r := chi.NewRouter()
    r.Use(chimw.Recoverer)

    s := &Server{svc: journal.New(repo, writer), rt: r}
    s.routes()
    return s
}

// Handler exposes the configured http.Handler.
func (s *Server) Handler() http.Handler { return s.rt }

// Mux is kept for compatibility with existing main wiring.
func (s *Server) Mux() http.Handler { return s.rt }

func (s *Server) routes() {
    // POST /entries with validation middleware
    s.rt.With(s.validatePostEntry()).Post("/entries", s.postEntry)
}
