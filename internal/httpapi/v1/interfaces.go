package v1

import (
    "context"

    "github.com/google/uuid"
    "github.com/tinoosan/ledger/internal/ledger"
)

// AccountReader abstracts account read operations.
type AccountReader interface {
    // FetchAccounts returns accounts for the given user filtered by the provided ids.
    FetchAccounts(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]ledger.Account, error)
    // ListAccounts returns all accounts for a given user.
    ListAccounts(ctx context.Context, userID uuid.UUID) ([]ledger.Account, error)
    // GetAccount returns a user's account by ID.
    GetAccount(ctx context.Context, userID, accountID uuid.UUID) (ledger.Account, error)
}

// EntryReader abstracts entry read operations.
type EntryReader interface {
    // ListEntries returns entries for a given user.
    ListEntries(ctx context.Context, userID uuid.UUID) ([]ledger.JournalEntry, error)
    // GetEntry returns an entry by id for the user.
    GetEntry(ctx context.Context, userID, entryID uuid.UUID) (ledger.JournalEntry, error)
}

// IdempotencyStore abstracts idempotency key operations for entries.
type IdempotencyStore interface {
    // GetEntryByIdempotencyKey resolves an entry by idempotency key for the user.
    GetEntryByIdempotencyKey(ctx context.Context, userID uuid.UUID, key string) (ledger.JournalEntry, bool, error)
    // SaveIdempotencyKey stores an idempotency key mapping for an entry.
    SaveIdempotencyKey(ctx context.Context, userID uuid.UUID, key string, entryID uuid.UUID) error
}

// ReadyChecker is optionally implemented by stores to indicate readiness.
type ReadyChecker interface {
    Ready(ctx context.Context) error
}

// Repository composes the read-side operations used by the API.
// It is a convenience union satisfied by the in-memory store.
type Repository interface {
    AccountReader
    EntryReader
    IdempotencyStore
}

// Writer interfaces are provided by services directly (journal.Writer, account.Writer).
