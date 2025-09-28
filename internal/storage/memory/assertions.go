package memory

import (
	"github.com/tinoosan/ledger/internal/service/account"
	"github.com/tinoosan/ledger/internal/service/journal"
)

// Compile-time interface assertions documenting which interfaces Store satisfies.
var (
	// Service layer repos and writers
	_ journal.Repo   = (*Store)(nil)
	_ journal.Writer = (*Store)(nil)
	_ account.Repo   = (*Store)(nil)
	_ account.Writer = (*Store)(nil)
)
