package memory

// Package memory provides a simple in-memory implementation used for development and tests.
// It keeps code paths easy to follow while allowing us to plug in a real DB later.
import (
    "context"
    "errors"
    "sort"
    "sync"
    "time"

    "github.com/google/uuid"
    "github.com/tinoosan/ledger/internal/ledger"
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
}

// New constructs an empty in-memory store.
func New() *Store {
    return &Store{
        users:           make(map[uuid.UUID]struct{}),
        accounts:        make(map[uuid.UUID]ledger.Account),
        entries:         make(map[uuid.UUID]*ledger.JournalEntry),
        entryKeysByUser: make(map[uuid.UUID][]entryKey),
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

// EntryByID returns a single entry for a user.
func (s *Store) EntryByID(_ context.Context, userID, entryID uuid.UUID) (ledger.JournalEntry, error) {
    s.mu.RLock(); defer s.mu.RUnlock()
    e, ok := s.entries[entryID]
    if !ok || e.UserID != userID { return ledger.JournalEntry{}, errors.New("entry not found") }
    return *e, nil
}

// EntryByClientID resolves entry via client entry id.
func (s *Store) EntryByClientID(_ context.Context, userID uuid.UUID, clientID string) (ledger.JournalEntry, bool, error) {
    s.mu.RLock(); defer s.mu.RUnlock()
    for _, e := range s.entries {
        if e.UserID == userID && e.ClientEntryID == clientID {
            return *e, true, nil
        }
    }
    return ledger.JournalEntry{}, false, nil
}

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
    if !ok || a.UserID != userID { return ledger.Account{}, errors.New("account not found") }
    return a, nil
}

// UpdateAccount persists changes to an account.
func (s *Store) UpdateAccount(_ context.Context, a ledger.Account) (ledger.Account, error) {
    s.mu.Lock(); defer s.mu.Unlock()
    s.accounts[a.ID] = a
    return a, nil
}

// insertEntryIndexLocked inserts k into the per-user sorted index, keeping order asc by (Date, ID).
// Caller must hold s.mu (write lock).
func (s *Store) insertEntryIndexLocked(userID uuid.UUID, k entryKey) {
    arr := s.entryKeysByUser[userID]
    // binary search for first position > k (stable insert after equal)
    i := sort.Search(len(arr), func(i int) bool {
        if arr[i].Date.After(k.Date) { return true }
        if arr[i].Date.Equal(k.Date) { return arr[i].ID.String() > k.ID.String() }
        return false
    })
    // insert at i
    if i == len(arr) {
        s.entryKeysByUser[userID] = append(arr, k)
        return
    }
    arr = append(arr, entryKey{})
    copy(arr[i+1:], arr[i:])
    arr[i] = k
    s.entryKeysByUser[userID] = arr
}

// rangeByTime returns a copy of keys within [from,to] inclusive for a user.
func (s *Store) rangeByTime(userID uuid.UUID, from, to *time.Time) []entryKey {
    s.mu.RLock(); defer s.mu.RUnlock()
    arr := s.entryKeysByUser[userID]
    if len(arr) == 0 { return nil }
    // find start
    start := 0
    if from != nil {
        f := *from
        start = sort.Search(len(arr), func(i int) bool {
            if arr[i].Date.After(f) || arr[i].Date.Equal(f) { return true }
            return false
        })
    }
    // find end (exclusive)
    end := len(arr)
    if to != nil {
        t := *to
        end = sort.Search(len(arr), func(i int) bool {
            if arr[i].Date.After(t) { return true }
            return false
        })
    }
    if start < 0 { start = 0 }
    if end > len(arr) { end = len(arr) }
    if start > end { return nil }
    out := make([]entryKey, end-start)
    copy(out, arr[start:end])
    return out
}
