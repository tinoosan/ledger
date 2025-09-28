package postgres

// Package postgres provides a pgx-backed storage implementation that satisfies
// the repository and writer interfaces used by the HTTP/API and services.
//
// It is intentionally small and explicit. Migrations that create the expected
// schema live under db/migrations. This package focuses on mapping between the
// domain entities and SQL rows and running the necessary statements/transactions.

import (
    "context"
    "errors"
    "fmt"
    "strings"

    "github.com/google/uuid"
    "github.com/govalues/money"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"

    "github.com/tinoosan/ledger/internal/errs"
    "github.com/tinoosan/ledger/internal/ledger"
    "github.com/tinoosan/ledger/internal/meta"
)

// Store holds a pgx connection pool and implements the read/write interfaces
// used across the service layer. All methods are safe for concurrent use.
type Store struct {
    pool *pgxpool.Pool
}

// Open establishes a pgx pool using the provided connection string.
func Open(ctx context.Context, dsn string) (*Store, error) {
    cfg, err := pgxpool.ParseConfig(dsn)
    if err != nil { return nil, err }
    pool, err := pgxpool.NewWithConfig(ctx, cfg)
    if err != nil { return nil, err }
    // Verify connection
    if err := pool.Ping(ctx); err != nil { pool.Close(); return nil, err }
    return &Store{pool: pool}, nil
}

// Close releases the underlying pool.
func (s *Store) Close() { if s.pool != nil { s.pool.Close() } }

// Ready pings the pool to verify connectivity.
func (s *Store) Ready(ctx context.Context) error { return s.pool.Ping(ctx) }

// SeedDev inserts a single user and three accounts (Opening Balances, Cash, Income)
// for quick local testing. It is idempotent per run due to fresh UUIDs.
func (s *Store) SeedDev(ctx context.Context) (ledger.User, []ledger.Account, error) {
    tx, err := s.pool.Begin(ctx)
    if err != nil { return ledger.User{}, nil, err }
    defer func() { _ = tx.Rollback(ctx) }()
    user := ledger.User{ID: uuid.New()}
    if _, err := tx.Exec(ctx, `insert into users (id, email) values ($1, null)`, user.ID); err != nil { return ledger.User{}, nil, err }
    // Opening balances (system)
    opening := ledger.Account{ID: uuid.New(), UserID: user.ID, Name: "Opening Balances", Currency: "GBP", Type: ledger.AccountTypeEquity, Group: "opening_balances", Vendor: "System", System: true, Active: true}
    cash := ledger.Account{ID: uuid.New(), UserID: user.ID, Name: "Cash", Currency: "GBP", Type: ledger.AccountTypeAsset, Group: "cash", Vendor: "Wallet", Active: true}
    income := ledger.Account{ID: uuid.New(), UserID: user.ID, Name: "Income", Currency: "GBP", Type: ledger.AccountTypeRevenue, Group: "salary", Vendor: "Employer", Active: true}
    accs := []ledger.Account{opening, cash, income}
    for _, a := range accs {
        md, _ := a.Metadata.MarshalStableJSON()
        if _, err := tx.Exec(ctx, `
            insert into accounts (id, user_id, name, currency, type, "group", vendor, metadata, system, active)
            values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
        `, a.ID, a.UserID, a.Name, strings.ToUpper(a.Currency), a.Type, strings.ToLower(a.Group), a.Vendor, md, a.System, a.Active); err != nil {
            return ledger.User{}, nil, err
        }
    }
    if err := tx.Commit(ctx); err != nil { return ledger.User{}, nil, err }
    return user, accs, nil
}

// --- Account reads ---

// FetchAccounts returns accounts for a user filtered by IDs.
func (s *Store) FetchAccounts(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]ledger.Account, error) {
    if len(ids) == 0 { return map[uuid.UUID]ledger.Account{}, nil }
    rows, err := s.pool.Query(ctx, `
        select id, user_id, name, currency, type, "group", vendor, metadata, system, active
        from accounts
        where user_id = $1 and id = any($2)
    `, userID, ids)
    if err != nil { return nil, err }
    defer rows.Close()
    out := make(map[uuid.UUID]ledger.Account)
    for rows.Next() {
        var a ledger.Account
        var mdBytes []byte
        if err := rows.Scan(&a.ID, &a.UserID, &a.Name, &a.Currency, &a.Type, &a.Group, &a.Vendor, &mdBytes, &a.System, &a.Active); err != nil { return nil, err }
        if len(mdBytes) > 0 {
            var m meta.Metadata
            if err := m.UnmarshalJSON(mdBytes); err == nil { a.Metadata = m }
        }
        out[a.ID] = a
    }
    return out, rows.Err()
}

// ListAccounts returns all accounts for a user.
func (s *Store) ListAccounts(ctx context.Context, userID uuid.UUID) ([]ledger.Account, error) {
    rows, err := s.pool.Query(ctx, `
        select id, user_id, name, currency, type, "group", vendor, metadata, system, active
        from accounts
        where user_id = $1
        order by type, "group", vendor, name
    `, userID)
    if err != nil { return nil, err }
    defer rows.Close()
    out := make([]ledger.Account, 0)
    for rows.Next() {
        var a ledger.Account
        var mdBytes []byte
        if err := rows.Scan(&a.ID, &a.UserID, &a.Name, &a.Currency, &a.Type, &a.Group, &a.Vendor, &mdBytes, &a.System, &a.Active); err != nil { return nil, err }
        if len(mdBytes) > 0 {
            var m meta.Metadata
            if err := m.UnmarshalJSON(mdBytes); err == nil { a.Metadata = m }
        }
        out = append(out, a)
    }
    return out, rows.Err()
}

// GetAccount fetches a single account by id for a user.
func (s *Store) GetAccount(ctx context.Context, userID, accountID uuid.UUID) (ledger.Account, error) {
    var a ledger.Account
    var mdBytes []byte
    err := s.pool.QueryRow(ctx, `
        select id, user_id, name, currency, type, "group", vendor, metadata, system, active
        from accounts
        where id = $1 and user_id = $2
    `, accountID, userID).Scan(&a.ID, &a.UserID, &a.Name, &a.Currency, &a.Type, &a.Group, &a.Vendor, &mdBytes, &a.System, &a.Active)
    if errors.Is(err, pgx.ErrNoRows) { return ledger.Account{}, errs.ErrNotFound }
    if err != nil { return ledger.Account{}, err }
    if len(mdBytes) > 0 {
        var m meta.Metadata
        if err := m.UnmarshalJSON(mdBytes); err == nil { a.Metadata = m }
    }
    return a, nil
}

// --- Account writes ---

// CreateAccount inserts an account row.
func (s *Store) CreateAccount(ctx context.Context, a ledger.Account) (ledger.Account, error) {
    if err := a.Metadata.Validate(); err != nil { return ledger.Account{}, err }
    md, _ := a.Metadata.MarshalStableJSON()
    _, err := s.pool.Exec(ctx, `
        insert into accounts (id, user_id, name, currency, type, "group", vendor, metadata, system, active)
        values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
    `, a.ID, a.UserID, a.Name, strings.ToUpper(a.Currency), a.Type, strings.ToLower(a.Group), a.Vendor, md, a.System, a.Active)
    if err != nil { return ledger.Account{}, err }
    return a, nil
}

// UpdateAccount updates mutable fields (name, group, vendor, metadata, active).
func (s *Store) UpdateAccount(ctx context.Context, a ledger.Account) (ledger.Account, error) {
    if err := a.Metadata.Validate(); err != nil { return ledger.Account{}, err }
    md, _ := a.Metadata.MarshalStableJSON()
    ct, err := s.pool.Exec(ctx, `
        update accounts
        set name=$1, "group"=$2, vendor=$3, metadata=$4, active=$5
        where id=$6 and user_id=$7
    `, a.Name, strings.ToLower(a.Group), a.Vendor, md, a.Active, a.ID, a.UserID)
    if err != nil { return ledger.Account{}, err }
    if ct.RowsAffected() == 0 { return ledger.Account{}, errs.ErrNotFound }
    return a, nil
}

// --- Entry reads ---

// ListEntries returns entries for a user with lines populated.
func (s *Store) ListEntries(ctx context.Context, userID uuid.UUID) ([]ledger.JournalEntry, error) {
    rows, err := s.pool.Query(ctx, `
        select id, user_id, date, currency, memo, category, metadata, is_reversed
        from entries
        where user_id = $1
        order by date asc, id asc
    `, userID)
    if err != nil { return nil, err }
    defer rows.Close()
    entries := make([]ledger.JournalEntry, 0)
    ids := make([]uuid.UUID, 0)
    for rows.Next() {
        var e ledger.JournalEntry
        var mdBytes []byte
        if err := rows.Scan(&e.ID, &e.UserID, &e.Date, &e.Currency, &e.Memo, &e.Category, &mdBytes, &e.IsReversed); err != nil { return nil, err }
        if len(mdBytes) > 0 {
            var m meta.Metadata
            if err := m.UnmarshalJSON(mdBytes); err == nil { e.Metadata = m }
        }
        e.Lines = ledger.JournalLines{ByID: map[uuid.UUID]*ledger.JournalLine{}}
        entries = append(entries, e)
        ids = append(ids, e.ID)
    }
    if len(entries) == 0 { return entries, nil }
    // Load lines for these entries
    lineRows, err := s.pool.Query(ctx, `
        select id, entry_id, account_id, side, amount_minor
        from entry_lines
        where entry_id = any($1)
        order by id asc
    `, ids)
    if err != nil { return nil, err }
    defer lineRows.Close()
    // index entries by id
    idx := make(map[uuid.UUID]*ledger.JournalEntry, len(entries))
    for i := range entries { idx[entries[i].ID] = &entries[i] }
    for lineRows.Next() {
        var id, entryID, accountID uuid.UUID
        var side string
        var minor int64
        if err := lineRows.Scan(&id, &entryID, &accountID, &side, &minor); err != nil { return nil, err }
        e := idx[entryID]
        if e == nil { continue }
        amt, _ := money.NewAmountFromMinorUnits(e.Currency, minor)
        ln := &ledger.JournalLine{ID: id, EntryID: entryID, AccountID: accountID, Side: ledger.Side(side), Amount: amt, Metadata: nil}
        if e.Lines.ByID == nil { e.Lines.ByID = map[uuid.UUID]*ledger.JournalLine{} }
        e.Lines.ByID[id] = ln
    }
    return entries, lineRows.Err()
}

// GetEntry returns an entry by id for a user with lines populated.
func (s *Store) GetEntry(ctx context.Context, userID, entryID uuid.UUID) (ledger.JournalEntry, error) {
    var e ledger.JournalEntry
    var mdBytes []byte
    err := s.pool.QueryRow(ctx, `
        select id, user_id, date, currency, memo, category, metadata, is_reversed
        from entries
        where id = $1 and user_id = $2
    `, entryID, userID).Scan(&e.ID, &e.UserID, &e.Date, &e.Currency, &e.Memo, &e.Category, &mdBytes, &e.IsReversed)
    if errors.Is(err, pgx.ErrNoRows) { return ledger.JournalEntry{}, errs.ErrNotFound }
    if err != nil { return ledger.JournalEntry{}, err }
    if len(mdBytes) > 0 {
        var m meta.Metadata
        if err := m.UnmarshalJSON(mdBytes); err == nil { e.Metadata = m }
    }
    e.Lines = ledger.JournalLines{ByID: map[uuid.UUID]*ledger.JournalLine{}}
    rows, err := s.pool.Query(ctx, `
        select id, account_id, side, amount_minor
        from entry_lines
        where entry_id = $1
        order by id asc
    `, entryID)
    if err != nil { return ledger.JournalEntry{}, err }
    defer rows.Close()
    for rows.Next() {
        var id, accountID uuid.UUID
        var side string
        var minor int64
        if err := rows.Scan(&id, &accountID, &side, &minor); err != nil { return ledger.JournalEntry{}, err }
        amt, _ := money.NewAmountFromMinorUnits(e.Currency, minor)
        e.Lines.ByID[id] = &ledger.JournalLine{ID: id, EntryID: entryID, AccountID: accountID, Side: ledger.Side(side), Amount: amt}
    }
    if err := rows.Err(); err != nil { return ledger.JournalEntry{}, err }
    return e, nil
}

// --- Entry writes ---

// CreateJournalEntry inserts an entry + its lines in a transaction.
func (s *Store) CreateJournalEntry(ctx context.Context, entry ledger.JournalEntry) (ledger.JournalEntry, error) {
    tx, err := s.pool.Begin(ctx)
    if err != nil { return ledger.JournalEntry{}, err }
    if err := createEntry(ctx, tx, entry); err != nil { _ = tx.Rollback(ctx); return ledger.JournalEntry{}, err }
    if err := tx.Commit(ctx); err != nil { return ledger.JournalEntry{}, err }
    return entry, nil
}

// UpdateJournalEntry updates fields of an entry (currently used to mark reversed).
func (s *Store) UpdateJournalEntry(ctx context.Context, entry ledger.JournalEntry) (ledger.JournalEntry, error) {
    md, _ := entry.Metadata.MarshalStableJSON()
    ct, err := s.pool.Exec(ctx, `
        update entries
        set memo=$1, category=$2, metadata=$3, is_reversed=$4
        where id=$5 and user_id=$6
    `, entry.Memo, entry.Category, md, entry.IsReversed, entry.ID, entry.UserID)
    if err != nil { return ledger.JournalEntry{}, err }
    if ct.RowsAffected() == 0 { return ledger.JournalEntry{}, errs.ErrNotFound }
    return entry, nil
}

// --- Idempotency ---

// GetEntryByIdempotencyKey resolves an entry by idempotency key for the user.
func (s *Store) GetEntryByIdempotencyKey(ctx context.Context, userID uuid.UUID, key string) (ledger.JournalEntry, bool, error) {
    var id uuid.UUID
    err := s.pool.QueryRow(ctx, `
        select entry_id from entry_idempotency where user_id=$1 and key=$2
    `, userID, key).Scan(&id)
    if errors.Is(err, pgx.ErrNoRows) { return ledger.JournalEntry{}, false, nil }
    if err != nil { return ledger.JournalEntry{}, false, err }
    e, err := s.GetEntry(ctx, userID, id)
    if err != nil { return ledger.JournalEntry{}, false, err }
    return e, true, nil
}

// SaveIdempotencyKey stores a mapping from (user,key) to entry id.
func (s *Store) SaveIdempotencyKey(ctx context.Context, userID uuid.UUID, key string, entryID uuid.UUID) error {
    _, err := s.pool.Exec(ctx, `
        insert into entry_idempotency (user_id, key, entry_id)
        values ($1,$2,$3)
        on conflict (user_id, key) do nothing
    `, userID, key, entryID)
    return err
}

// --- Batches / transactions ---

// BeginTx creates a batch transaction wrapper used by service batch endpoints.
func (s *Store) BeginTx(ctx context.Context) (*Tx, error) {
    tx, err := s.pool.Begin(ctx)
    if err != nil { return nil, err }
    return &Tx{tx: tx}, nil
}

// Tx wraps a pgx.Tx and implements the minimal methods used in batch flows.
type Tx struct{ tx pgx.Tx }

func (t *Tx) CreateAccount(ctx context.Context, a ledger.Account) (ledger.Account, error) {
    if err := a.Metadata.Validate(); err != nil { return ledger.Account{}, err }
    md, _ := a.Metadata.MarshalStableJSON()
    if _, err := t.tx.Exec(ctx, `
        insert into accounts (id, user_id, name, currency, type, "group", vendor, metadata, system, active)
        values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
    `, a.ID, a.UserID, a.Name, strings.ToUpper(a.Currency), a.Type, strings.ToLower(a.Group), a.Vendor, md, a.System, a.Active); err != nil {
        return ledger.Account{}, err
    }
    return a, nil
}

func (t *Tx) CreateJournalEntry(ctx context.Context, e ledger.JournalEntry) (ledger.JournalEntry, error) {
    if err := createEntry(ctx, t.tx, e); err != nil { return ledger.JournalEntry{}, err }
    return e, nil
}

func (t *Tx) Commit(ctx context.Context) error { return t.tx.Commit(ctx) }
func (t *Tx) Rollback(ctx context.Context) error { return t.tx.Rollback(ctx) }

// createEntry inserts the entry header and its lines within the provided executor.
func createEntry(ctx context.Context, ex pgx.Tx, e ledger.JournalEntry) error {
    md, _ := e.Metadata.MarshalStableJSON()
    if _, err := ex.Exec(ctx, `
        insert into entries (id, user_id, date, currency, memo, category, metadata, is_reversed)
        values ($1,$2,$3,$4,$5,$6,$7,$8)
    `, e.ID, e.UserID, e.Date, strings.ToUpper(e.Currency), e.Memo, e.Category, md, e.IsReversed); err != nil {
        return err
    }
    // lines
    for _, ln := range e.Lines.ByID {
        minor, _ := ln.Amount.MinorUnits()
        if _, err := ex.Exec(ctx, `
            insert into entry_lines (id, entry_id, account_id, side, amount_minor)
            values ($1,$2,$3,$4,$5)
        `, ln.ID, e.ID, ln.AccountID, ln.Side, minor); err != nil {
            return fmt.Errorf("insert line: %w", err)
        }
    }
    return nil
}
