package ledger

import (
	"time"

	"github.com/google/uuid"
	"github.com/govalues/money"
)

// Side represents the accounting position of a journal line.
type Side string

const (
	// SideDebit records a value on the debit side of an account.
	SideDebit Side = "debit"
	// SideCredit records a value on the credit side of an account.
	SideCredit Side = "credit"
)

// AccountType enumerates the broad classification of an account in the ledger.
type AccountType string

const (
	// AccountTypeAsset increases on the debit side and holds resources owned by a user.
	AccountTypeAsset AccountType = "asset"
	// AccountTypeLiability increases on the credit side and tracks obligations.
	AccountTypeLiability AccountType = "liability"
	// AccountTypeEquity captures the owner's residual interest in the entity.
	AccountTypeEquity AccountType = "equity"
	// AccountTypeRevenue represents inflows that increase equity.
	AccountTypeRevenue AccountType = "revenue"
	// AccountTypeExpense represents outflows that decrease equity.
	AccountTypeExpense AccountType = "expense"
)

// Category identifies the posting logic applied to a journal entry.
type Category string

const (
	CategoryUncategorized Category = "uncategorized"
	CategoryGeneral       Category = "general"
	CategoryEatingOut     Category = "eating_out"
	CategoryGroceries     Category = "groceries"
	CategoryTransport     Category = "transport"
	CategoryShopping      Category = "shopping"
	CategoryEntertainment Category = "entertainment"
	CategoryBills         Category = "bills"
	CategoryTravel        Category = "travel"
	CategoryExpenses      Category = "expenses"
	CategoryIncome        Category = "income"
	CategoryTransfers     Category = "transfers"
	CategorySavings       Category = "savings"
	CategoryCharity       Category = "charity"
	CategoryFamily        Category = "family"
	CategoryGifts         Category = "gifts"
	CategoryPersonalCare  Category = "personal_care"
	CategoryBusiness      Category = "business"
)

// User captures the owner of ledger data.
type User struct {
	ID    uuid.UUID
	Email *string
}

// Account represents a ledger account belonging to a user.
type Account struct {
	ID       uuid.UUID
	UserID   uuid.UUID
	Name     string
	Currency string
	Type     AccountType
}

// JournalEntry captures metadata for a collection of journal lines.
type JournalEntry struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	Date          time.Time
	Currency      string
	Memo          string
	Category      Category
	ClientEntryID string
	Lines         JournalLines
}

// JournalLines groups the set of lines that belong to a journal entry.
type JournalLines struct {
	ByID map[uuid.UUID]*JournalLine
}

// JournalLine links a journal entry to an account with an amount on a side.
type JournalLine struct {
	ID        uuid.UUID
	EntryID   uuid.UUID
	AccountID uuid.UUID
	Side      Side
	Amount    money.Amount
	Metadata  map[string]string
}
