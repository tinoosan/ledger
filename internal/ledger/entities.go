package ledger

import (
    "time"
    "strings"

    "github.com/google/uuid"
    "github.com/govalues/money"
    "github.com/tinoosan/ledger/internal/meta"
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
    // Method describes the instrument or sub-type (e.g., Bank, CreditCard, Cash, Savings, Loan, Rent, Salary).
    Method   string
    // Vendor identifies the specific institution or instance (e.g., Monzo, Amex, PayPal, LandlordLtd, EmployerX).
    Vendor   string
    // Metadata holds additional key-value attributes for the account.
    Metadata meta.Metadata `json:"metadata,omitempty"`
    // System marks reserved, immutable accounts (e.g., Equity:OpeningBalances).
    System   bool
    // Active indicates whether the account is active (soft-delete when false).
    Active   bool
}

// Path returns a colon-separated identifier for the account: Type:Method:Vendor.
// Example: assets:bank:monzo
func (a Account) Path() string {
    // Special-case OpeningBalances: show concise path without vendor and with lowercase
    if a.Type == AccountTypeEquity && strings.EqualFold(a.Method, "OpeningBalances") {
        return "equity:openingbalances"
    }
    return string(a.Type) + ":" + strings.ToLower(a.Method) + ":" + strings.ToLower(a.Vendor)
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
    // Metadata holds additional key-value attributes for the entry.
    Metadata      meta.Metadata `json:"metadata,omitempty"`
    // IsReversed marks that this entry has been reversed.
    IsReversed    bool
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
