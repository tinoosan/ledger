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
    Type        ledger.AccountType `json:"type"`
}

type trialBalanceResponse struct {
    UserID   uuid.UUID             `json:"user_id"`
    AsOf     *time.Time            `json:"as_of,omitempty"`
    Accounts []trialBalanceAccount `json:"accounts"`
}

// Accounts

type postAccountRequest struct {
    UserID   uuid.UUID           `json:"user_id"`
    Name     string              `json:"name"`
    Currency string              `json:"currency"`
    Type     ledger.AccountType  `json:"type"`
    Method   string              `json:"method"`
    Vendor   string              `json:"vendor"`
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
}

type listAccountsQuery struct {
    UserID uuid.UUID
    Method string
    Vendor string
    Type   string
}
