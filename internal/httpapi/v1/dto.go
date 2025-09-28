package v1

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
    Metadata      map[string]string `json:"metadata,omitempty"`
    Lines         []postEntryLine `json:"lines"`
}

type postEntryLine struct {
    AccountID   uuid.UUID   `json:"account_id"`
    Side        ledger.Side `json:"side"`
    AmountMinor int64       `json:"amount_minor"`
}


type entryResponse struct {
    ID            uuid.UUID       `json:"id"`
    UserID        uuid.UUID       `json:"user_id"`
    Date          time.Time       `json:"date"`
    Currency      string          `json:"currency"`
    Memo          string          `json:"memo"`
    Category      ledger.Category `json:"category"`
    Metadata      map[string]string `json:"metadata,omitempty"`
    IsReversed    bool            `json:"is_reversed"`
    Lines         []lineResponse  `json:"lines"`
}

type lineResponse struct {
    ID          uuid.UUID   `json:"id"`
    AccountID   uuid.UUID   `json:"account_id"`
    Side        ledger.Side `json:"side"`
    AmountMinor int64       `json:"amount_minor"`
    Amount      string      `json:"amount"`
}

// listEntriesQuery holds validated query params for GET /entries.
type listEntriesQuery struct {
    UserID uuid.UUID
}

// listEntriesResponse wraps entries with cursor for pagination.
type listEntriesResponse struct {
    Items      []entryResponse `json:"items"`
    NextCursor *string         `json:"next_cursor,omitempty"`
}

// Reverse entry
type reverseEntryRequest struct {
    UserID  uuid.UUID `json:"user_id"`
    EntryID uuid.UUID `json:"entry_id"`
    // optional date; if omitted handler sets time.Now()
    Date    *time.Time `json:"date,omitempty"`
}

// Trial balance
type trialBalanceQuery struct {
    UserID uuid.UUID
    AsOf   *time.Time
}

type trialBalanceAccount struct {
    AccountID   uuid.UUID          `json:"account_id"`
    Name        string             `json:"name"`
    Path        string             `json:"path"`
    Currency    string             `json:"currency"`
    DebitMinor  int64              `json:"debit_minor"`
    CreditMinor int64              `json:"credit_minor"`
    Debit       string             `json:"debit"`
    Credit      string             `json:"credit"`
    Type        ledger.AccountType `json:"type"`
}

type trialBalanceCurrencyGroup struct {
    Currency string                 `json:"currency"`
    Accounts []trialBalanceAccount  `json:"accounts"`
}

type trialBalanceResponse struct {
    UserID uuid.UUID                  `json:"user_id"`
    AsOf   *time.Time                 `json:"as_of,omitempty"`
    Groups []trialBalanceCurrencyGroup `json:"groups"`
}

// Accounts

type postAccountRequest struct {
    UserID   uuid.UUID           `json:"user_id"`
    Name     string              `json:"name"`
    Currency string              `json:"currency"`
    Type     ledger.AccountType  `json:"type"`
    Method   string              `json:"method"`
    Vendor   string              `json:"vendor"`
    System   bool                `json:"system,omitempty"`
}

type accountResponse struct {
    ID       uuid.UUID           `json:"id"`
    UserID   uuid.UUID           `json:"user_id"`
    Name     string              `json:"name"`
    Currency string              `json:"currency"`
    Type     ledger.AccountType  `json:"type"`
    Method   string              `json:"method"`
    Vendor   string              `json:"vendor"`
    Path     string              `json:"path"`
    Metadata map[string]string   `json:"metadata,omitempty"`
    System   bool                `json:"system"`
}

type listAccountsQuery struct {
    UserID uuid.UUID
    Method string
    Vendor string
    Type   string
}
