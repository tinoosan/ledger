package httpapi

import "github.com/tinoosan/ledger/internal/storage/memory"

// Compile-time interface assertions for the in-memory Store against HTTP API interfaces.
var (
    _ AccountReader     = (*memory.Store)(nil)
    _ EntryReader       = (*memory.Store)(nil)
    _ IdempotencyStore  = (*memory.Store)(nil)
)

