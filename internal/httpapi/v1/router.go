// Package httpapi wires the HTTP surface of the ledger service.
// It keeps handlers thin, delegating business rules to the service layer.
package v1

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
    accReader AccountReader
    entryReader EntryReader
    idemStore IdempotencyStore
    log    *slog.Logger
    rt     *chi.Mux
}

// New constructs the HTTP server with routes and middleware.
// The logger is used by basic request/response logging and panic recovery.
func New(accReader AccountReader, entryReader EntryReader, idem IdempotencyStore, jrepo journal.Repo, arepo account.Repo, jwriter journal.Writer, awriter account.Writer, logger *slog.Logger) *Server {
    r := chi.NewRouter()
    r.Use(chimw.RequestID)
    r.Use(requestLogger(logger))
    r.Use(recoverer(logger))

    s := &Server{
        svc:        journal.New(jrepo, jwriter),
        accountSvc: account.New(arepo, awriter),
        accReader:  accReader,
        entryReader: entryReader,
        idemStore:  idem,
        rt:         r,
        log:        logger,
    }
    s.routes()
    return s
}

// Handler exposes the configured http.Handler.
func (s *Server) Handler() http.Handler { return s.rt }

// Mux is kept for compatibility with existing main wiring.
func (s *Server) Mux() http.Handler { return s.rt }

// routes declares the public HTTP API endpoints and attaches any per-route middleware.
func (s *Server) routes() {
    // Entries (v1)
    s.rt.With(s.validatePostEntry()).Post("/v1/entries", s.postEntry)
    s.rt.With(s.validateListEntries()).Get("/v1/entries", s.listEntries)
    s.rt.Get("/v1/entries/{id}", s.getEntry)
    s.rt.With(s.validateReverseEntry()).Post("/v1/entries/reverse", s.reverseEntry)
    s.rt.Post("/v1/entries/reclassify", s.reclassifyEntry)
    s.rt.With(s.validateTrialBalance()).Get("/v1/trial-balance", s.trialBalance)
    // Accounts (v1)
    s.rt.With(s.validatePostAccount()).Post("/v1/accounts", s.postAccount)
    s.rt.Post("/v1/accounts/batch", s.postAccountsBatch)
    s.rt.With(s.validateListAccounts()).Get("/v1/accounts", s.listAccounts)
    s.rt.Get("/v1/accounts/{id}", s.getAccount)
    s.rt.Get("/v1/accounts/{id}/balance", s.getAccountBalance)
    s.rt.Get("/v1/accounts/{id}/ledger", s.getAccountLedger)
    s.rt.Get("/v1/accounts/opening-balances", s.getOpeningBalancesAccount)
    // Unversioned aliases for convenience/tests
    s.rt.Get("/accounts/{id}/balance", s.getAccountBalance)
    s.rt.Get("/accounts/{id}/ledger", s.getAccountLedger)
    s.rt.Patch("/v1/accounts/{id}", s.updateAccount)
    s.rt.Delete("/v1/accounts/{id}", s.deactivateAccount)
    // Health (unversioned)
    s.rt.Get("/healthz", s.healthz)
    s.rt.Get("/readyz", s.readyz)
    // OpenAPI spec (dev convenience)
    s.rt.Get("/v1/openapi.yaml", s.openapiSpec)
}
