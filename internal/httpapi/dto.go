package httpapi

import (
    "time"

    "github.com/google/uuid"
    "github.com/tinoosan/ledger/internal/ledger"
)

type postEntryRequest struct {
    UserID        uuid.UUID       `json:"user_id"`
    Date          time.Time       `json:"date"`
    Currency      string          `json:"currency"`
    Memo          string          `json:"memo"`
    Category      ledger.Category `json:"category"`
    ClientEntryID string          `json:"client_entry_id"`
    Lines         []postEntryLine `json:"lines"`
}

type postEntryLine struct {
    AccountID   uuid.UUID   `json:"account_id"`
    Side        ledger.Side `json:"side"`
    AmountMinor int64       `json:"amount_minor"`
}

type errorResponse struct {
    Error string `json:"error"`
}

type entryResponse struct {
    ID            uuid.UUID       `json:"id"`
    UserID        uuid.UUID       `json:"user_id"`
    Date          time.Time       `json:"date"`
    Currency      string          `json:"currency"`
    Memo          string          `json:"memo"`
    Category      ledger.Category `json:"category"`
    ClientEntryID string          `json:"client_entry_id"`
    Lines         []lineResponse  `json:"lines"`
}

type lineResponse struct {
    ID          uuid.UUID   `json:"id"`
    AccountID   uuid.UUID   `json:"account_id"`
    Side        ledger.Side `json:"side"`
    AmountMinor int64       `json:"amount_minor"`
}

// listEntriesQuery holds validated query params for GET /entries.
type listEntriesQuery struct {
    UserID uuid.UUID
}

// Accounts

type postAccountRequest struct {
    UserID   uuid.UUID           `json:"user_id"`
    Name     string              `json:"name"`
    Currency string              `json:"currency"`
    Type     ledger.AccountType  `json:"type"`
}

type accountResponse struct {
    ID       uuid.UUID           `json:"id"`
    UserID   uuid.UUID           `json:"user_id"`
    Name     string              `json:"name"`
    Currency string              `json:"currency"`
    Type     ledger.AccountType  `json:"type"`
}

type listAccountsQuery struct {
    UserID uuid.UUID
}
