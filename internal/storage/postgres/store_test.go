package postgres

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/govalues/money"
	"github.com/tinoosan/ledger/internal/ledger"
)

func getTestDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres store tests")
	}
	return dsn
}

func mustOpen(t *testing.T, dsn string) *Store {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return s
}

func applyInitSQL(t *testing.T, dsn string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open for init: %v", err)
	}
	defer s.Close()
	// Resolve init SQL path relative to this test file so CWD doesn't matter
	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../../../"))
	path := filepath.Join(repoRoot, "db", "migrations", "0001_init.sql")
	b, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatalf("read init sql: %v", err)
	}
	// Exec may contain multiple statements; pgx supports this
	if _, err := s.pool.Exec(ctx, string(b)); err != nil {
		t.Fatalf("apply init sql: %v", err)
	}
}

func truncateAll(t *testing.T, dsn string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open for truncate: %v", err)
	}
	defer s.Close()
	_, _ = s.pool.Exec(ctx, `truncate table entry_idempotency, entry_lines, entries, accounts, users cascade`)
}

func TestStore_AccountsAndEntries(t *testing.T) {
	dsn := getTestDSN(t)
	applyInitSQL(t, dsn)
	truncateAll(t, dsn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	s := mustOpen(t, dsn)
	defer s.Close()

	if err := s.Ready(ctx); err != nil {
		t.Fatalf("ready: %v", err)
	}

	// Seed dev data to get a user + accounts
	user, accs, err := s.SeedDev(ctx)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if user.ID == uuid.Nil || len(accs) < 3 {
		t.Fatalf("unexpected seed: %+v", user)
	}

	// Accounts: list + get + update
	list, err := s.ListAccounts(ctx, user.ID)
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if len(list) < 3 {
		t.Fatalf("expected >=3 accounts, got %d", len(list))
	}
	got, err := s.GetAccount(ctx, user.ID, list[0].ID)
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	got.Name = got.Name + " (upd)"
	if _, err := s.UpdateAccount(ctx, got); err != nil {
		t.Fatalf("update account: %v", err)
	}

	// Entries: create + list + get + update
	// Use the first two accounts for a balanced entry in GBP
	if len(list) < 2 {
		t.Fatalf("need at least 2 accounts")
	}
	a1 := list[0]
	a2 := list[1]
	if a1.Currency != a2.Currency {
		t.Skip("seed produced different currencies; skipping entry test")
	}
	// Construct entry
	amt, _ := money.NewAmountFromMinorUnits(a1.Currency, 1234)
	e := newBalancedEntry(user.ID, a1.ID, a2.ID, amt)
	created, err := s.CreateJournalEntry(ctx, e)
	if err != nil {
		t.Fatalf("create entry: %v", err)
	}
	if created.ID == uuid.Nil || len(created.Lines.ByID) != 2 {
		t.Fatalf("unexpected created entry: %+v", created)
	}

	gotE, err := s.GetEntry(ctx, user.ID, created.ID)
	if err != nil {
		t.Fatalf("get entry: %v", err)
	}
	if len(gotE.Lines.ByID) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(gotE.Lines.ByID))
	}

	listE, err := s.ListEntries(ctx, user.ID)
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(listE) < 1 {
		t.Fatalf("expected >=1 entry")
	}

	// Update entry (mark reversed)
	gotE.IsReversed = true
	if _, err := s.UpdateJournalEntry(ctx, gotE); err != nil {
		t.Fatalf("update entry: %v", err)
	}

	// Idempotency mapping
	key := "test-key-1"
	if err := s.SaveIdempotencyKey(ctx, user.ID, key, created.ID); err != nil {
		t.Fatalf("save idem: %v", err)
	}
	if _, ok, err := s.GetEntryByIdempotencyKey(ctx, user.ID, key); err != nil || !ok {
		t.Fatalf("get idem: %v ok=%v", err, ok)
	}
}

// helper creates a balanced entry with two lines
func newBalancedEntry(userID, accDebit, accCredit uuid.UUID, amt money.Amount) ledger.JournalEntry {
	lines := ledger.JournalLines{ByID: map[uuid.UUID]*ledger.JournalLine{}}
	dID := uuid.New()
	cID := uuid.New()
	eID := uuid.Nil
	lines.ByID[dID] = &ledger.JournalLine{ID: dID, EntryID: eID, AccountID: accDebit, Side: ledger.SideDebit, Amount: amt}
	lines.ByID[cID] = &ledger.JournalLine{ID: cID, EntryID: eID, AccountID: accCredit, Side: ledger.SideCredit, Amount: amt}
	return ledger.JournalEntry{
		ID:       uuid.New(),
		UserID:   userID,
		Date:     time.Now().UTC(),
		Currency: amt.Curr().Code(),
		Memo:     "test-entry",
		Category: ledger.CategoryGeneral,
		Lines:    lines,
	}
}
