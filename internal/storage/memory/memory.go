package memory

// Package memory provides a simple in-memory implementation used for development and tests.
// It keeps code paths easy to follow while allowing us to plug in a real DB later.
import (
    "context"
    "sort"
    "sync"
    "time"

    "github.com/google/uuid"
    "github.com/tinoosan/ledger/internal/ledger"
    "github.com/tinoosan/ledger/internal/errs"
)

// entryKey tracks ordering for entries per user: sorted asc by (Date, ID)
type entryKey struct {
    Date time.Time
    ID   uuid.UUID
}

// Store is an in-memory implementation of the repository+writer used by the API.
// It is guarded by an RWMutex for concurrent reads/writes.
type Store struct {
    mu       sync.RWMutex
    users    map[uuid.UUID]struct{}
    accounts map[uuid.UUID]ledger.Account
    entries  map[uuid.UUID]*ledger.JournalEntry
    // Per-user sorted index of entries for efficient ordered scans and paging
    entryKeysByUser map[uuid.UUID][]entryKey
    // Idempotency: userID -> key -> entryID
    entryIdem map[uuid.UUID]map[string]uuid.UUID
}

// New constructs an empty in-memory store.
func New() *Store {
    return &Store{
        users:           make(map[uuid.UUID]struct{}),
        accounts:        make(map[uuid.UUID]ledger.Account),
        entries:         make(map[uuid.UUID]*ledger.JournalEntry),
        entryKeysByUser: make(map[uuid.UUID][]entryKey),
        entryIdem:       make(map[uuid.UUID]map[string]uuid.UUID),
    }
}

// Seed helpers for local dev/tests.
func (s *Store) SeedUser(u ledger.User)               { s.mu.Lock(); s.users[u.ID] = struct{}{}; s.mu.Unlock() }
func (s *Store) SeedAccount(a ledger.Account)         { s.mu.Lock(); s.accounts[a.ID] = a; s.mu.Unlock() }
func (s *Store) Reset() {
    s.mu.Lock()
    s.users = map[uuid.UUID]struct{}{}
    s.accounts = map[uuid.UUID]ledger.Account{}
    s.entries = map[uuid.UUID]*ledger.JournalEntry{}
    s.entryKeysByUser = map[uuid.UUID][]entryKey{}
    s.entryIdem = map[uuid.UUID]map[string]uuid.UUID{}
    s.mu.Unlock()
}

// AccountsByIDs implements httpapi.Repository.
func (s *Store) AccountsByIDs(_ context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]ledger.Account, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    out := make(map[uuid.UUID]ledger.Account, len(ids))
    seen := make(map[uuid.UUID]struct{}, len(ids))
    for _, id := range ids {
        if _, ok := seen[id]; ok {
            continue
        }
        seen[id] = struct{}{}
        if acc, ok := s.accounts[id]; ok && acc.UserID == userID {
            out[id] = acc
        }
    }
    return out, nil
}

// FetchAccounts is an alias to AccountsByIDs to satisfy httpapi.AccountReader.
func (s *Store) FetchAccounts(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]ledger.Account, error) {
    return s.AccountsByIDs(ctx, userID, ids)
}

// CreateJournalEntry implements httpapi.Writer.
func (s *Store) CreateJournalEntry(_ context.Context, entry ledger.JournalEntry) (ledger.JournalEntry, error) {
    s.mu.Lock()
    defer s.mu.Unlock()
    // store shallow copy
    e := entry
    s.entries[e.ID] = &e
    s.insertEntryIndexLocked(e.UserID, entryKey{Date: e.Date, ID: e.ID})
    return e, nil
}

// UpdateJournalEntry updates an existing journal entry by ID.
func (s *Store) UpdateJournalEntry(_ context.Context, entry ledger.JournalEntry) (ledger.JournalEntry, error) {
    s.mu.Lock(); defer s.mu.Unlock()
    if _, ok := s.entries[entry.ID]; !ok { return ledger.JournalEntry{}, errs.ErrNotFound }
    e := entry
    s.entries[entry.ID] = &e
    return e, nil
}

// EntriesByUserID returns all entries for a user.
func (s *Store) EntriesByUserID(_ context.Context, userID uuid.UUID) ([]ledger.JournalEntry, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    keys := s.entryKeysByUser[userID]
    out := make([]ledger.JournalEntry, 0, len(keys))
    for _, k := range keys {
        if e, ok := s.entries[k.ID]; ok && e.UserID == userID {
            out = append(out, *e)
        }
    }
    return out, nil
}

// ListEntries is an alias to EntriesByUserID to satisfy httpapi.EntryReader.
func (s *Store) ListEntries(ctx context.Context, userID uuid.UUID) ([]ledger.JournalEntry, error) {
    return s.EntriesByUserID(ctx, userID)
}

// EntryByID returns a single entry for a user.
func (s *Store) EntryByID(_ context.Context, userID, entryID uuid.UUID) (ledger.JournalEntry, error) {
    s.mu.RLock(); defer s.mu.RUnlock()
    e, ok := s.entries[entryID]
    if !ok || e.UserID != userID { return ledger.JournalEntry{}, errs.ErrNotFound }
    return *e, nil
}

// GetEntry is an alias to EntryByID to satisfy httpapi.EntryReader.
func (s *Store) GetEntry(ctx context.Context, userID, entryID uuid.UUID) (ledger.JournalEntry, error) {
    return s.EntryByID(ctx, userID, entryID)
}

// EntryByClientID resolves entry via client entry id.
// EntryByClientID removed (client idempotency not supported currently)

// AccountsByUserID returns accounts for a user.
func (s *Store) AccountsByUserID(_ context.Context, userID uuid.UUID) ([]ledger.Account, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    out := make([]ledger.Account, 0)
    for _, a := range s.accounts {
        if a.UserID == userID {
            out = append(out, a)
        }
    }
    return out, nil
}

// ListAccounts is an alias to AccountsByUserID to satisfy httpapi.AccountReader.
func (s *Store) ListAccounts(ctx context.Context, userID uuid.UUID) ([]ledger.Account, error) {
    return s.AccountsByUserID(ctx, userID)
}

// CreateAccount persists a new account.
func (s *Store) CreateAccount(_ context.Context, a ledger.Account) (ledger.Account, error) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.accounts[a.ID] = a
    return a, nil
}

// AccountByID returns a user's account by ID.
func (s *Store) AccountByID(_ context.Context, userID, accountID uuid.UUID) (ledger.Account, error) {
    s.mu.RLock(); defer s.mu.RUnlock()
    a, ok := s.accounts[accountID]
    if !ok || a.UserID != userID { return ledger.Account{}, errs.ErrNotFound }
    return a, nil
}

// GetAccount is an alias to AccountByID to satisfy httpapi.AccountReader.
func (s *Store) GetAccount(ctx context.Context, userID, accountID uuid.UUID) (ledger.Account, error) {
    return s.AccountByID(ctx, userID, accountID)
}

// UpdateAccount persists changes to an account.
func (s *Store) UpdateAccount(_ context.Context, a ledger.Account) (ledger.Account, error) {
    s.mu.Lock(); defer s.mu.Unlock()
    s.accounts[a.ID] = a
    return a, nil
}

// ResolveEntryByIdempotencyKey implements httpapi.Repository.
func (s *Store) ResolveEntryByIdempotencyKey(_ context.Context, userID uuid.UUID, key string) (ledger.JournalEntry, bool, error) {
    s.mu.RLock(); defer s.mu.RUnlock()
    if m, ok := s.entryIdem[userID]; ok {
        if eid, ok2 := m[key]; ok2 {
            if e, ok3 := s.entries[eid]; ok3 {
                return *e, true, nil
            }
        }
    }
    return ledger.JournalEntry{}, false, nil
}

// SaveEntryIdempotencyKey implements httpapi.Repository.
func (s *Store) SaveEntryIdempotencyKey(_ context.Context, userID uuid.UUID, key string, entryID uuid.UUID) error {
    s.mu.Lock(); defer s.mu.Unlock()
    m, ok := s.entryIdem[userID]
    if !ok { m = make(map[string]uuid.UUID); s.entryIdem[userID] = m }
    // Only set if absent to preserve idempotency
    if _, exists := m[key]; !exists {
        m[key] = entryID
    }
    return nil
}

// GetEntryByIdempotencyKey implements httpapi.IdempotencyStore.
func (s *Store) GetEntryByIdempotencyKey(ctx context.Context, userID uuid.UUID, key string) (ledger.JournalEntry, bool, error) {
    return s.ResolveEntryByIdempotencyKey(ctx, userID, key)
}

// SaveIdempotencyKey implements httpapi.IdempotencyStore.
func (s *Store) SaveIdempotencyKey(ctx context.Context, userID uuid.UUID, key string, entryID uuid.UUID) error {
    return s.SaveEntryIdempotencyKey(ctx, userID, key, entryID)
}

// insertEntryIndexLocked inserts k into the per-user sorted index, keeping order asc by (Date, ID).
// Caller must hold s.mu (write lock).
func (s *Store) insertEntryIndexLocked(userID uuid.UUID, k entryKey) {
    keys := s.entryKeysByUser[userID]
    // binary search for first position > k (stable insert after equal)
    i := sort.Search(len(keys), func(i int) bool {
        if keys[i].Date.After(k.Date) { return true }
        if keys[i].Date.Equal(k.Date) { return keys[i].ID.String() > k.ID.String() }
        return false
    })
    // insert at i
    if i == len(keys) {
        s.entryKeysByUser[userID] = append(keys, k)
        return
    }
    keys = append(keys, entryKey{})
    copy(keys[i+1:], keys[i:])
    keys[i] = k
    s.entryKeysByUser[userID] = keys
}

// rangeByTime returns a copy of keys within [from,to] inclusive for a user.
func (s *Store) rangeByTime(userID uuid.UUID, from, to *time.Time) []entryKey {
    s.mu.RLock(); defer s.mu.RUnlock()
    keys := s.entryKeysByUser[userID]
    if len(keys) == 0 { return nil }
    // find start
    start := 0
    if from != nil {
        f := *from
        start = sort.Search(len(keys), func(i int) bool {
            if keys[i].Date.After(f) || keys[i].Date.Equal(f) { return true }
            return false
        })
    }
    // find end (exclusive)
    end := len(keys)
    if to != nil {
        t := *to
        end = sort.Search(len(keys), func(i int) bool {
            if keys[i].Date.After(t) { return true }
            return false
        })
    }
    if start < 0 { start = 0 }
    if end > len(keys) { end = len(keys) }
    if start > end { return nil }
    subset := make([]entryKey, end-start)
    copy(subset, keys[start:end])
    return subset
}
