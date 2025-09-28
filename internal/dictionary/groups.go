package dictionary

import "github.com/tinoosan/ledger/internal/ledger"

type GroupDef struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Reserved bool   `json:"reserved"`
}

var curated = map[ledger.AccountType][]GroupDef{
	ledger.AccountTypeEquity: {
		{Code: "opening_balances", Label: "Opening Balances", Reserved: true},
		{Code: "owner_equity", Label: "Owner Equity", Reserved: false},
	},
	ledger.AccountTypeAsset: {
		{Code: "bank", Label: "Bank", Reserved: false},
		{Code: "cash", Label: "Cash", Reserved: false},
		{Code: "wallet", Label: "Wallet", Reserved: false},
		{Code: "savings", Label: "Savings", Reserved: false},
		{Code: "investment", Label: "Investment", Reserved: false},
		{Code: "receivable", Label: "Receivable", Reserved: false},
	},
	ledger.AccountTypeLiability: {
		{Code: "credit_card", Label: "Credit Card", Reserved: false},
		{Code: "loan", Label: "Loan", Reserved: false},
		{Code: "payable", Label: "Payable", Reserved: false},
	},
	ledger.AccountTypeRevenue: {
		{Code: "salary", Label: "Salary", Reserved: false},
		{Code: "interest", Label: "Interest", Reserved: false},
		{Code: "refund", Label: "Refund", Reserved: false},
		{Code: "other_income", Label: "Other Income", Reserved: false},
	},
	ledger.AccountTypeExpense: {
		{Code: "groceries", Label: "Groceries", Reserved: false},
		{Code: "eating_out", Label: "Eating Out", Reserved: false},
		{Code: "rent", Label: "Rent", Reserved: false},
		{Code: "utilities", Label: "Utilities", Reserved: false},
		{Code: "transport", Label: "Transport", Reserved: false},
		{Code: "shopping", Label: "Shopping", Reserved: false},
		{Code: "entertainment", Label: "Entertainment", Reserved: false},
		{Code: "general", Label: "General", Reserved: false},
	},
}

func IsReserved(t ledger.AccountType, group string) bool {
	for _, g := range curated[t] {
		if g.Code == group && g.Reserved {
			return true
		}
	}
	return false
}

func GroupsFor(t *ledger.AccountType) []GroupDef {
	if t == nil { // all types
		out := make([]GroupDef, 0)
		for _, list := range curated {
			out = append(out, list...)
		}
		return out
	}
	return curated[*t]
}
