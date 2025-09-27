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
}

// Writer abstracts write-side operations needed by the API.
type Writer interface {
    // CreateJournalEntry persists the entry and its lines atomically.
    CreateJournalEntry(ctx context.Context, entry ledger.JournalEntry) (ledger.JournalEntry, error)
}

