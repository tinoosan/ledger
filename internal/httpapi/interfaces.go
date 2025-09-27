package httpapi

import (
    "context"

    "github.com/google/uuid"
    "github.com/tinoosan/ledger/internal/ledger"
)

// Repository abstracts read-side operations needed by the API.
type Repository interface {
    // AccountsByIDs returns accounts for the given user filtered by the provided ids.
    AccountsByIDs(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]ledger.Account, error)
    // EntriesByUserID returns entries for a given user.
    EntriesByUserID(ctx context.Context, userID uuid.UUID) ([]ledger.JournalEntry, error)
    // AccountsByUserID returns accounts for a given user.
    AccountsByUserID(ctx context.Context, userID uuid.UUID) ([]ledger.Account, error)
    // EntryByID returns entry by id for the user
    EntryByID(ctx context.Context, userID, entryID uuid.UUID) (ledger.JournalEntry, error)
    AccountByID(ctx context.Context, userID, accountID uuid.UUID) (ledger.Account, error)
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
