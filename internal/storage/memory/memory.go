package memory

// Package memory provides a simple in-memory implementation used for development and tests.
// It keeps code paths easy to follow while allowing us to plug in a real DB later.
import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/ledger/internal/errs"
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
	mu           sync.RWMutex
	userSet      map[uuid.UUID]struct{}
	accountsByID map[uuid.UUID]ledger.Account
	entriesByID  map[uuid.UUID]*ledger.JournalEntry
	// Per-user sorted index of entries for efficient ordered scans and paging
	entryIndexByUser map[uuid.UUID][]entryKey
	// Idempotency: userID -> key -> entryID
	idempotencyByUser map[uuid.UUID]map[string]uuid.UUID
}

// New constructs an empty in-memory store.
func New() *Store {
	return &Store{
		userSet:           make(map[uuid.UUID]struct{}),
		accountsByID:      make(map[uuid.UUID]ledger.Account),
		entriesByID:       make(map[uuid.UUID]*ledger.JournalEntry),
		entryIndexByUser:  make(map[uuid.UUID][]entryKey),
		idempotencyByUser: make(map[uuid.UUID]map[string]uuid.UUID),
	}
}

// Ready implements a no-op readiness check for memory store.
func (s *Store) Ready(ctx context.Context) error { return nil }

// clone helpers to avoid shared state exposure
func cloneAccount(a ledger.Account) ledger.Account {
	cloned := a
	cloned.Metadata = a.Metadata.Clone()
	return cloned
}

func cloneEntry(e ledger.JournalEntry) ledger.JournalEntry {
	cloned := e
	cloned.Metadata = e.Metadata.Clone()
	return cloned
}

// Seed helpers for local dev/tests.
func (s *Store) SeedUser(u ledger.User)       { s.mu.Lock(); s.userSet[u.ID] = struct{}{}; s.mu.Unlock() }
func (s *Store) SeedAccount(a ledger.Account) { s.mu.Lock(); s.accountsByID[a.ID] = a; s.mu.Unlock() }
func (s *Store) Reset() {
	s.mu.Lock()
	s.userSet = map[uuid.UUID]struct{}{}
	s.accountsByID = map[uuid.UUID]ledger.Account{}
	s.entriesByID = map[uuid.UUID]*ledger.JournalEntry{}
	s.entryIndexByUser = map[uuid.UUID][]entryKey{}
	s.idempotencyByUser = map[uuid.UUID]map[string]uuid.UUID{}
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
		if acc, ok := s.accountsByID[id]; ok && acc.UserID == userID {
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
	e.Metadata = entry.Metadata.Clone()
	s.entriesByID[e.ID] = &e
	s.insertEntryIndexLocked(e.UserID, entryKey{Date: e.Date, ID: e.ID})
	return cloneEntry(e), nil
}

// UpdateJournalEntry updates an existing journal entry by ID.
func (s *Store) UpdateJournalEntry(_ context.Context, entry ledger.JournalEntry) (ledger.JournalEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.entriesByID[entry.ID]; !ok {
		return ledger.JournalEntry{}, errs.ErrNotFound
	}
	e := entry
	e.Metadata = entry.Metadata.Clone()
	s.entriesByID[entry.ID] = &e
	return cloneEntry(e), nil
}

// EntriesByUserID returns all entries for a user.
func (s *Store) EntriesByUserID(_ context.Context, userID uuid.UUID) ([]ledger.JournalEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := s.entryIndexByUser[userID]
	out := make([]ledger.JournalEntry, 0, len(keys))
	for _, k := range keys {
		if e, ok := s.entriesByID[k.ID]; ok && e.UserID == userID {
			out = append(out, cloneEntry(*e))
		}
	}
	return out, nil
}

// ListEntries is an alias to EntriesByUserID to satisfy httpapi.EntryReader.
func (s *Store) ListEntries(ctx context.Context, userID uuid.UUID) ([]ledger.JournalEntry, error) {
	return s.EntriesByUserID(ctx, userID)
}

// EntryByID returns a single entry for a user.
// GetEntry returns a single entry for a user.
func (s *Store) GetEntry(_ context.Context, userID, entryID uuid.UUID) (ledger.JournalEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entriesByID[entryID]
	if !ok || e.UserID != userID {
		return ledger.JournalEntry{}, errs.ErrNotFound
	}
	return cloneEntry(*e), nil
}

// EntryByClientID resolves entry via client entry id.
// EntryByClientID removed (client idempotency not supported currently)

// AccountsByUserID returns accounts for a user.
func (s *Store) AccountsByUserID(_ context.Context, userID uuid.UUID) ([]ledger.Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ledger.Account, 0)
	for _, a := range s.accountsByID {
		if a.UserID == userID {
			out = append(out, cloneAccount(a))
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
	ca := cloneAccount(a)
	s.accountsByID[a.ID] = ca
	return cloneAccount(ca), nil
}

// AccountByID returns a user's account by ID.
// GetAccount returns a user's account by ID.
func (s *Store) GetAccount(_ context.Context, userID, accountID uuid.UUID) (ledger.Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.accountsByID[accountID]
	if !ok || a.UserID != userID {
		return ledger.Account{}, errs.ErrNotFound
	}
	return cloneAccount(a), nil
}

// UpdateAccount persists changes to an account.
func (s *Store) UpdateAccount(_ context.Context, a ledger.Account) (ledger.Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ca := cloneAccount(a)
	s.accountsByID[a.ID] = ca
	return cloneAccount(ca), nil
}

// ResolveEntryByIdempotencyKey implements httpapi.Repository.
func (s *Store) ResolveEntryByIdempotencyKey(_ context.Context, userID uuid.UUID, key string) (ledger.JournalEntry, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if m, ok := s.idempotencyByUser[userID]; ok {
		if eid, ok2 := m[key]; ok2 {
			if e, ok3 := s.entriesByID[eid]; ok3 {
				return *e, true, nil
			}
		}
	}
	return ledger.JournalEntry{}, false, nil
}

// SaveEntryIdempotencyKey implements httpapi.Repository.
func (s *Store) SaveEntryIdempotencyKey(_ context.Context, userID uuid.UUID, key string, entryID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.idempotencyByUser[userID]
	if !ok {
		m = make(map[string]uuid.UUID)
		s.idempotencyByUser[userID] = m
	}
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

// Batch transaction support (copy-on-write for created entities)
type batchTx struct {
	s        *Store
	accounts []ledger.Account
	entries  []ledger.JournalEntry
}

func (s *Store) BeginTx(_ context.Context) (*batchTx, error) {
	return &batchTx{s: s, accounts: []ledger.Account{}, entries: []ledger.JournalEntry{}}, nil
}

func (tx *batchTx) CreateAccount(_ context.Context, a ledger.Account) (ledger.Account, error) {
	tx.accounts = append(tx.accounts, a)
	return a, nil
}

func (tx *batchTx) CreateJournalEntry(_ context.Context, e ledger.JournalEntry) (ledger.JournalEntry, error) {
	tx.entries = append(tx.entries, e)
	return e, nil
}

func (tx *batchTx) Commit(_ context.Context) error {
	tx.s.mu.Lock()
	defer tx.s.mu.Unlock()
	// Check account conflicts and apply
	for _, a := range tx.accounts {
		for _, existing := range tx.s.accountsByID {
			if existing.UserID == a.UserID && existing.Currency == a.Currency && strings.EqualFold(existing.Path(), a.Path()) {
				return errs.ErrConflict
			}
		}
	}
	for _, a := range tx.accounts {
		ca := cloneAccount(a)
		tx.s.accountsByID[a.ID] = ca
	}
	for _, e := range tx.entries {
		ce := cloneEntry(e)
		tx.s.entriesByID[e.ID] = &ce
		tx.s.insertEntryIndexLocked(e.UserID, entryKey{Date: e.Date, ID: e.ID})
	}
	return nil
}

func (tx *batchTx) Rollback(_ context.Context) error { return nil }

// insertEntryIndexLocked inserts k into the per-user sorted index, keeping order asc by (Date, ID).
// Caller must hold s.mu (write lock).
func (s *Store) insertEntryIndexLocked(userID uuid.UUID, k entryKey) {
	keys := s.entryIndexByUser[userID]
	// binary search for first position > k (stable insert after equal)
	i := sort.Search(len(keys), func(i int) bool {
		if keys[i].Date.After(k.Date) {
			return true
		}
		if keys[i].Date.Equal(k.Date) {
			return keys[i].ID.String() > k.ID.String()
		}
		return false
	})
	// insert at i
	if i == len(keys) {
		s.entryIndexByUser[userID] = append(keys, k)
		return
	}
	keys = append(keys, entryKey{})
	copy(keys[i+1:], keys[i:])
	keys[i] = k
	s.entryIndexByUser[userID] = keys
}

// rangeByTime returns a copy of keys within [from,to] inclusive for a user.
// rangeByTime removed as unused; index operations are handled inline where needed.
