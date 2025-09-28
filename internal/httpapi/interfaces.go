package httpapi

import (
    "context"

    "github.com/google/uuid"
    "github.com/tinoosan/ledger/internal/ledger"
)

// AccountReader abstracts account read operations.
type AccountReader interface {
    // AccountsByIDs returns accounts for the given user filtered by the provided ids.
    AccountsByIDs(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]ledger.Account, error)
    // AccountsByUserID returns accounts for a given user.
    AccountsByUserID(ctx context.Context, userID uuid.UUID) ([]ledger.Account, error)
    // AccountByID returns a user's account by ID.
    AccountByID(ctx context.Context, userID, accountID uuid.UUID) (ledger.Account, error)
}

// EntryReader abstracts entry read operations.
type EntryReader interface {
    // EntriesByUserID returns entries for a given user.
    EntriesByUserID(ctx context.Context, userID uuid.UUID) ([]ledger.JournalEntry, error)
    // EntryByID returns entry by id for the user
    EntryByID(ctx context.Context, userID, entryID uuid.UUID) (ledger.JournalEntry, error)
}

// IdempotencyStore abstracts idempotency key operations for entries.
type IdempotencyStore interface {
    // ResolveEntryByIdempotencyKey resolves an entry by idempotency key for the user.
    ResolveEntryByIdempotencyKey(ctx context.Context, userID uuid.UUID, key string) (ledger.JournalEntry, bool, error)
    // SaveEntryIdempotencyKey stores an idempotency key mapping for an entry.
    SaveEntryIdempotencyKey(ctx context.Context, userID uuid.UUID, key string, entryID uuid.UUID) error
}

// Repository composes the read-side operations used by the API.
// It is a convenience union satisfied by the in-memory store.
type Repository interface {
    AccountReader
    EntryReader
    IdempotencyStore
}

// Writer abstracts write-side operations needed by the API.
type Writer interface {
    // CreateJournalEntry persists the entry and its lines atomically.
    CreateJournalEntry(ctx context.Context, entry ledger.JournalEntry) (ledger.JournalEntry, error)
    // CreateAccount persists a new account.
    CreateAccount(ctx context.Context, a ledger.Account) (ledger.Account, error)
    // UpdateAccount persists account changes.
    UpdateAccount(ctx context.Context, a ledger.Account) (ledger.Account, error)
}
