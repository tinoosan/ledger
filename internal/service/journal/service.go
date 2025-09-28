package journal

import (
    "context"
    "errors"
    "time"

    "github.com/google/uuid"
    "github.com/govalues/money"

    "github.com/tinoosan/ledger/internal/ledger"
    "github.com/tinoosan/ledger/internal/errs"
)

// Repo defines read operations needed by the service.
type Repo interface {
    AccountsByIDs(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]ledger.Account, error)
    EntriesByUserID(ctx context.Context, userID uuid.UUID) ([]ledger.JournalEntry, error)
    EntryByID(ctx context.Context, userID, entryID uuid.UUID) (ledger.JournalEntry, error)
}

// Writer defines write operations needed by the service.
type Writer interface {
    CreateJournalEntry(ctx context.Context, entry ledger.JournalEntry) (ledger.JournalEntry, error)
}

// Service exposes validation and creation of journal entries and reporting helpers.
type Service interface {
    ValidateEntry(ctx context.Context, e ledger.JournalEntry) error
    CreateEntry(ctx context.Context, e ledger.JournalEntry) (ledger.JournalEntry, error)
    ListEntries(ctx context.Context, userID uuid.UUID) ([]ledger.JournalEntry, error)
    ReverseEntry(ctx context.Context, userID, entryID uuid.UUID, date time.Time) (ledger.JournalEntry, error)
    Reclassify(ctx context.Context, userID, entryID uuid.UUID, date time.Time, memo string, category ledger.Category, newLines []ledger.JournalLine) (ledger.JournalEntry, error)
    TrialBalance(ctx context.Context, userID uuid.UUID, asOf *time.Time) (map[uuid.UUID]money.Amount, error)
    AccountBalance(ctx context.Context, userID, accountID uuid.UUID, asOf *time.Time) (money.Amount, error)
}

type service struct {
    repo   Repo
    writer Writer
}

func New(repo Repo, writer Writer) Service { return &service{repo: repo, writer: writer} }

func (s *service) ValidateEntry(ctx context.Context, entry ledger.JournalEntry) error {
    if entry.UserID == uuid.Nil {
        return errs.ErrInvalid
    }
    if entry.Currency == "" {
        return errs.ErrInvalid
    }
    if len(entry.Lines.ByID) < 2 {
        return errors.New("at least 2 lines")
    }

    ids := make([]uuid.UUID, 0, len(entry.Lines.ByID))
    var sumDebits, sumCredits int64
    i := 0
    for _, line := range entry.Lines.ByID {
        if line.AccountID == uuid.Nil {
            return fieldErr(i, "account_id required")
        }
        units, _ := line.Amount.MinorUnits()
        if units <= 0 {
            return fieldErr(i, "amount must be > 0")
        }
        switch line.Side {
        case ledger.SideDebit:
            sumDebits += units
        case ledger.SideCredit:
            sumCredits += units
        default:
            return fieldErr(i, "side must be debit or credit")
        }
        ids = append(ids, line.AccountID)
        i++
    }
    if sumDebits != sumCredits {
        return errors.New("sum(debits) must equal sum(credits)")
    }

    accMap, err := s.repo.AccountsByIDs(ctx, entry.UserID, ids)
    if err != nil {
        return err
    }
    if len(accMap) != len(unique(ids)) {
        return errors.New("unknown or unauthorized accounts")
    }
    i = 0
    for _, line := range entry.Lines.ByID {
        acc, ok := accMap[line.AccountID]
        if !ok {
            return fieldErr(i, "account not found for user")
        }
        if acc.UserID != entry.UserID {
            return fieldErr(i, "account does not belong to user")
        }
        if acc.Currency != entry.Currency {
            return fieldErr(i, "account currency mismatch")
        }
        i++
    }
    return nil
}

func (s *service) CreateEntry(ctx context.Context, entry ledger.JournalEntry) (ledger.JournalEntry, error) {
    // Assume ValidateEntry has been called; create and persist atomically.
    entryID := uuid.New()
    lines := ledger.JournalLines{ByID: make(map[uuid.UUID]*ledger.JournalLine, len(entry.Lines.ByID))}
    for _, ln := range entry.Lines.ByID {
        id := uuid.New()
        nl := *ln
        nl.ID = id
        nl.EntryID = entryID
        lines.ByID[id] = &nl
    }

    entry = ledger.JournalEntry{
        ID:            entryID,
        UserID:        entry.UserID,
        Date:          entry.Date,
        Currency:      entry.Currency,
        Memo:          entry.Memo,
        Category:      entry.Category,
        Lines:         lines,
    }
    return s.writer.CreateJournalEntry(ctx, entry)
}

func (s *service) ListEntries(ctx context.Context, userID uuid.UUID) ([]ledger.JournalEntry, error) {
    if userID == uuid.Nil {
        return nil, errs.ErrInvalid
    }
    return s.repo.EntriesByUserID(ctx, userID)
}

// ReverseEntry flips all lines of a prior entry and posts a new balancing entry.
func (s *service) ReverseEntry(ctx context.Context, userID, entryID uuid.UUID, date time.Time) (ledger.JournalEntry, error) {
    if userID == uuid.Nil || entryID == uuid.Nil {
        return ledger.JournalEntry{}, errs.ErrInvalid
    }
    orig, err := s.repo.EntryByID(ctx, userID, entryID)
    if err != nil {
        return ledger.JournalEntry{}, err
    }
    if orig.UserID != userID {
        return ledger.JournalEntry{}, errs.ErrForbidden
    }
    rid := uuid.New()
    lines := ledger.JournalLines{ByID: make(map[uuid.UUID]*ledger.JournalLine, len(orig.Lines.ByID))}
    for _, ln := range orig.Lines.ByID {
        nl := *ln
        nl.ID = uuid.New()
        nl.EntryID = rid
        if ln.Side == ledger.SideDebit { nl.Side = ledger.SideCredit } else { nl.Side = ledger.SideDebit }
        lines.ByID[nl.ID] = &nl
    }
    e := ledger.JournalEntry{
        ID:       rid,
        UserID:   userID,
        Date:     date,
        Currency: orig.Currency,
        Memo:     "reversal of " + orig.ID.String() + ": " + orig.Memo,
        Category: orig.Category,
        Lines:    lines,
    }
    return s.writer.CreateJournalEntry(ctx, e)
}

// Reclassify posts a reversing entry for the original, then a correcting entry with provided lines.
// Returns the correcting entry.
func (s *service) Reclassify(ctx context.Context, userID, entryID uuid.UUID, date time.Time, memo string, category ledger.Category, newLines []ledger.JournalLine) (ledger.JournalEntry, error) {
    if userID == uuid.Nil || entryID == uuid.Nil {
        return ledger.JournalEntry{}, errs.ErrInvalid
    }
    orig, err := s.repo.EntryByID(ctx, userID, entryID)
    if err != nil { return ledger.JournalEntry{}, err }
    if orig.UserID != userID { return ledger.JournalEntry{}, errs.ErrForbidden }

    // 1) reversing entry
    if _, err := s.ReverseEntry(ctx, userID, entryID, date); err != nil { return ledger.JournalEntry{}, err }

    // 2) correcting entry
    if memo == "" { memo = "reclassify of " + orig.ID.String() }
    lines := ledger.JournalLines{ByID: make(map[uuid.UUID]*ledger.JournalLine, len(newLines))}
    for i := range newLines {
        ln := newLines[i]
        id := uuid.New()
        ln.ID = id
        ln.EntryID = uuid.Nil
        lines.ByID[id] = &ln
    }
    e := ledger.JournalEntry{UserID: userID, Date: date, Currency: orig.Currency, Memo: memo, Category: category, Lines: lines}
    if err := s.ValidateEntry(ctx, e); err != nil { return ledger.JournalEntry{}, err }
    return s.CreateEntry(ctx, e)
}

// TrialBalance returns net amounts per account (debits - credits) up to asOf (inclusive).
// TrialBalance returns net amounts (debits - credits) per account up to asOf.
func (s *service) TrialBalance(ctx context.Context, userID uuid.UUID, asOf *time.Time) (map[uuid.UUID]money.Amount, error) {
    if userID == uuid.Nil {
        return nil, errors.New("user_id is required")
    }
    entries, err := s.repo.EntriesByUserID(ctx, userID)
    if err != nil { return nil, err }
    out := make(map[uuid.UUID]money.Amount)
    for _, e := range entries {
        if asOf != nil && e.Date.After(*asOf) {
            continue
        }
        for _, ln := range e.Lines.ByID {
            curr := ln.Amount.Curr().Code()
            // initialize zero amount for currency if needed
            if _, ok := out[ln.AccountID]; !ok {
                out[ln.AccountID], _ = money.NewAmountFromMinorUnits(curr, 0)
            }
            switch ln.Side {
            case ledger.SideDebit:
                if v, err := out[ln.AccountID].Add(ln.Amount); err == nil { out[ln.AccountID] = v }
            case ledger.SideCredit:
                if v, err := out[ln.AccountID].Sub(ln.Amount); err == nil { out[ln.AccountID] = v }
            }
        }
    }
    return out, nil
}

// AccountBalance returns net amount for a single account up to asOf.
func (s *service) AccountBalance(ctx context.Context, userID, accountID uuid.UUID, asOf *time.Time) (money.Amount, error) {
    if userID == uuid.Nil || accountID == uuid.Nil { return money.MustNewAmount("USD", 0, 0), errors.New("user_id and account_id are required") }
    entries, err := s.repo.EntriesByUserID(ctx, userID)
    if err != nil { return money.MustNewAmount("USD", 0, 0), err }
    // Determine currency from first matching line or default to USD
    var curr string
    for _, e := range entries {
        if asOf != nil && e.Date.After(*asOf) { continue }
        for _, ln := range e.Lines.ByID { if ln.AccountID == accountID { curr = ln.Amount.Curr().Code(); break } }
        if curr != "" { break }
    }
    if curr == "" { curr = "USD" }
    net, _ := money.NewAmountFromMinorUnits(curr, 0)
    for _, e := range entries {
        if asOf != nil && e.Date.After(*asOf) { continue }
        for _, ln := range e.Lines.ByID {
            if ln.AccountID != accountID { continue }
            switch ln.Side {
            case ledger.SideDebit:
                if v, err := net.Add(ln.Amount); err == nil { net = v }
            case ledger.SideCredit:
                if v, err := net.Sub(ln.Amount); err == nil { net = v }
            }
        }
    }
    return net, nil
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
