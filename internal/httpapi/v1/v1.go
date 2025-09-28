package v1

import (
    "log/slog"
    base "github.com/tinoosan/ledger/internal/httpapi"
    "github.com/tinoosan/ledger/internal/service/account"
    "github.com/tinoosan/ledger/internal/service/journal"
)

// Server is an alias to the base HTTP API server for v1.
type Server = base.Server

// New constructs a v1 HTTP server using the same implementation as the base package.
func New(accReader base.AccountReader, entryReader base.EntryReader, idem base.IdempotencyStore, jrepo journal.Repo, arepo account.Repo, jwriter journal.Writer, awriter account.Writer, logger *slog.Logger) *Server {
    return base.New(accReader, entryReader, idem, jrepo, arepo, jwriter, awriter, logger)
}

