package memory

import (
    "context"
    "sync"

    "github.com/google/uuid"
    "github.com/tinoosan/ledger/internal/ledger"
)

// Store is an in-memory implementation of the repository+writer used by the API.
type Store struct {
    mu       sync.Mutex
    users    map[uuid.UUID]struct{}
    accounts map[uuid.UUID]ledger.Account
    entries  map[uuid.UUID]*ledger.JournalEntry
}

// New constructs an empty in-memory store.
func New() *Store {
    return &Store{
        users:    make(map[uuid.UUID]struct{}),
        accounts: make(map[uuid.UUID]ledger.Account),
        entries:  make(map[uuid.UUID]*ledger.JournalEntry),
    }
}

// Seed helpers for local dev/tests.
func (s *Store) SeedUser(u ledger.User)               { s.mu.Lock(); s.users[u.ID] = struct{}{}; s.mu.Unlock() }
func (s *Store) SeedAccount(a ledger.Account)         { s.mu.Lock(); s.accounts[a.ID] = a; s.mu.Unlock() }
func (s *Store) Reset()                               { s.mu.Lock(); s.users = map[uuid.UUID]struct{}{}; s.accounts = map[uuid.UUID]ledger.Account{}; s.entries = map[uuid.UUID]*ledger.JournalEntry{}; s.mu.Unlock() }

// AccountsByIDs implements httpapi.Repository.
func (s *Store) AccountsByIDs(_ context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]ledger.Account, error) {
    s.mu.Lock()
    defer s.mu.Unlock()
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
    return e, nil
}

// EntriesByUserID returns all entries for a user.
func (s *Store) EntriesByUserID(_ context.Context, userID uuid.UUID) ([]ledger.JournalEntry, error) {
    s.mu.Lock()
    defer s.mu.Unlock()
    out := make([]ledger.JournalEntry, 0)
    for _, e := range s.entries {
        if e.UserID == userID {
            out = append(out, *e)
        }
    }
    return out, nil
}

// AccountsByUserID returns accounts for a user.
func (s *Store) AccountsByUserID(_ context.Context, userID uuid.UUID) ([]ledger.Account, error) {
    s.mu.Lock()
    defer s.mu.Unlock()
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
