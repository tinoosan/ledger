// Package httpapi wires the HTTP surface of the ledger service.
// It keeps handlers thin, delegating business rules to the service layer.
package httpapi

import (
    "net/http"

    chi "github.com/go-chi/chi/v5"
    chimw "github.com/go-chi/chi/v5/middleware"
    "log/slog"
    "github.com/tinoosan/ledger/internal/service/journal"
    "github.com/tinoosan/ledger/internal/service/account"
)

// Server wires handlers and middleware using Chi.
// It composes read (repo) and write (writer) dependencies through services.
type Server struct {
    svc    journal.Service
    accountSvc account.Service
    repo   Repository
    log    *slog.Logger
    rt     *chi.Mux
}

// New constructs the HTTP server with routes and middleware.
// The logger is used by basic request/response logging and panic recovery.
func New(repo Repository, writer Writer, logger *slog.Logger) *Server {
    r := chi.NewRouter()
    r.Use(chimw.RequestID)
    r.Use(requestLogger(logger))
    r.Use(recoverer(logger))

    s := &Server{svc: journal.New(repo, writer), accountSvc: account.New(repo, writer), repo: repo, rt: r, log: logger}
    s.routes()
    return s
}

// Handler exposes the configured http.Handler.
func (s *Server) Handler() http.Handler { return s.rt }

// Mux is kept for compatibility with existing main wiring.
func (s *Server) Mux() http.Handler { return s.rt }

// routes declares the public HTTP API endpoints and attaches any per-route middleware.
func (s *Server) routes() {
    // Entries
    s.rt.With(s.validatePostEntry()).Post("/entries", s.postEntry)
    s.rt.With(s.validateListEntries()).Get("/entries", s.listEntries)
    s.rt.Get("/entries/{id}", s.getEntry)
    s.rt.With(s.validateReverseEntry()).Post("/entries/reverse", s.reverseEntry)
    s.rt.With(s.validateTrialBalance()).Get("/trial-balance", s.trialBalance)
    // Accounts
    s.rt.With(s.validatePostAccount()).Post("/accounts", s.postAccount)
    s.rt.With(s.validateListAccounts()).Get("/accounts", s.listAccounts)
    s.rt.Get("/accounts/{id}/balance", s.getAccountBalance)
    s.rt.Get("/accounts/{id}/ledger", s.getAccountLedger)
    s.rt.Patch("/accounts/{id}", s.updateAccount)
    s.rt.Delete("/accounts/{id}", s.deactivateAccount)
    // Idempotency
    s.rt.Get("/idempotency/entries/{client_entry_id}", s.getEntryByClientID)
    // Health
    s.rt.Get("/healthz", s.healthz)
    s.rt.Get("/readyz", s.readyz)
}
