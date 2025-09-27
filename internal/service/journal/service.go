package journal

import (
    "context"
    "errors"

    "github.com/google/uuid"
    "github.com/govalues/money"

    "github.com/tinoosan/ledger/internal/ledger"
)

// Repo defines read operations needed by the service.
type Repo interface {
    AccountsByIDs(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]ledger.Account, error)
}

// Writer defines write operations needed by the service.
type Writer interface {
    CreateJournalEntry(ctx context.Context, entry ledger.JournalEntry) (ledger.JournalEntry, error)
}

// Service exposes validation and creation of journal entries.
type Service interface {
    ValidateEntryInput(ctx context.Context, in EntryInput) error
    CreateEntry(ctx context.Context, in EntryInput) (ledger.JournalEntry, error)
}

type service struct {
    repo   Repo
    writer Writer
}

func New(repo Repo, writer Writer) Service { return &service{repo: repo, writer: writer} }

// EntryInput carries all data necessary to create a journal entry.
type EntryInput struct {
    UserID        uuid.UUID
    Date          int64 // unix seconds; transport may use time.Time and convert as needed
    Currency      string
    Memo          string
    Category      ledger.Category
    ClientEntryID string
    Lines         []LineInput
}

type LineInput struct {
    AccountID   uuid.UUID
    Side        ledger.Side
    AmountMinor int64
}

func (s *service) ValidateEntryInput(ctx context.Context, in EntryInput) error {
    if in.UserID == uuid.Nil {
        return errors.New("user_id is required")
    }
    if in.Currency == "" {
        return errors.New("currency is required")
    }
    if len(in.Lines) < 2 {
        return errors.New("at least 2 lines")
    }

    ids := make([]uuid.UUID, 0, len(in.Lines))
    var sumDebits, sumCredits int64
    for i, ln := range in.Lines {
        if ln.AccountID == uuid.Nil {
            return fieldErr(i, "account_id required")
        }
        if ln.AmountMinor <= 0 {
            return fieldErr(i, "amount must be > 0")
        }
        switch ln.Side {
        case ledger.SideDebit:
            sumDebits += ln.AmountMinor
        case ledger.SideCredit:
            sumCredits += ln.AmountMinor
        default:
            return fieldErr(i, "side must be debit or credit")
        }
        ids = append(ids, ln.AccountID)
    }
    if sumDebits != sumCredits {
        return errors.New("sum(debits) must equal sum(credits)")
    }

    accMap, err := s.repo.AccountsByIDs(ctx, in.UserID, ids)
    if err != nil {
        return err
    }
    if len(accMap) != len(unique(ids)) {
        return errors.New("unknown or unauthorized accounts")
    }
    for i, ln := range in.Lines {
        acc, ok := accMap[ln.AccountID]
        if !ok {
            return fieldErr(i, "account not found for user")
        }
        if acc.UserID != in.UserID {
            return fieldErr(i, "account does not belong to user")
        }
        if acc.Currency != in.Currency {
            return fieldErr(i, "account currency mismatch")
        }
    }
    return nil
}

func (s *service) CreateEntry(ctx context.Context, in EntryInput) (ledger.JournalEntry, error) {
    // Assume ValidateEntryInput has been called; create and persist atomically.
    entryID := uuid.New()
    lines := ledger.JournalLines{ByID: make(map[uuid.UUID]*ledger.JournalLine, len(in.Lines))}
    for _, ln := range in.Lines {
        amt, _ := money.NewAmountFromMinorUnits(in.Currency, ln.AmountMinor)
        id := uuid.New()
        lines.ByID[id] = &ledger.JournalLine{
            ID:        id,
            EntryID:   entryID,
            AccountID: ln.AccountID,
            Side:      ln.Side,
            Amount:    amt,
        }
    }

    // Convert unix seconds to time.Time in the API layer instead if preferred; here we set zero time.
    // For simplicity we'll leave Date to be set by caller; handler currently passes time.Time.
    entry := ledger.JournalEntry{
        ID:            entryID,
        UserID:        in.UserID,
        Currency:      in.Currency,
        Memo:          in.Memo,
        Category:      in.Category,
        ClientEntryID: in.ClientEntryID,
        Lines:         lines,
    }
    return s.writer.CreateJournalEntry(ctx, entry)
}

func fieldErr(i int, msg string) error { return errors.New("line[" + itoa(i) + "]: " + msg) }

func unique(ids []uuid.UUID) []uuid.UUID {
    seen := make(map[uuid.UUID]struct{}, len(ids))
    out := make([]uuid.UUID, 0, len(ids))
    for _, id := range ids {
        if _, ok := seen[id]; ok {
            continue
        }
        seen[id] = struct{}{}
        out = append(out, id)
    }
    return out
}

func itoa(n int) string {
    if n == 0 { return "0" }
    neg := false
    if n < 0 { neg = true; n = -n }
    var buf [20]byte
    i := len(buf)
    for n > 0 { i--; buf[i] = byte('0' + n%10); n /= 10 }
    if neg { i--; buf[i] = '-' }
    return string(buf[i:])
}

