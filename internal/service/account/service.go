// Package account implements the account service rules: immutable identity fields,
// editable descriptive fields, soft-deletes, and per-user unique Path.
package account

import (
    "context"
    "errors"
    "strings"

    "github.com/google/uuid"
    "github.com/tinoosan/ledger/internal/ledger"
    "github.com/tinoosan/ledger/internal/errs"
)

type Repo interface {
    ListAccounts(ctx context.Context, userID uuid.UUID) ([]ledger.Account, error)
    GetAccount(ctx context.Context, userID, accountID uuid.UUID) (ledger.Account, error)
}

type Writer interface {
    CreateAccount(ctx context.Context, a ledger.Account) (ledger.Account, error)
    UpdateAccount(ctx context.Context, a ledger.Account) (ledger.Account, error)
}

type Service interface {
    ValidateCreate(a ledger.Account) error
    Create(ctx context.Context, a ledger.Account) (ledger.Account, error)
    List(ctx context.Context, userID uuid.UUID) ([]ledger.Account, error)
    Update(ctx context.Context, a ledger.Account) (ledger.Account, error)
    Deactivate(ctx context.Context, userID, accountID uuid.UUID) error
    EnsureOpeningBalanceAccount(ctx context.Context, userID uuid.UUID, currency string) (ledger.Account, error)
}

type service struct {
    repo   Repo
    writer Writer
}

func New(repo Repo, writer Writer) Service { return &service{repo: repo, writer: writer} }

// EnsureOpeningBalanceAccount returns the OpeningBalances system account for the currency,
// creating it if missing (idempotent per (user, currency)).
func (s *service) EnsureOpeningBalanceAccount(ctx context.Context, userID uuid.UUID, currency string) (ledger.Account, error) {
    if userID == uuid.Nil || currency == "" { return ledger.Account{}, errs.ErrInvalid }
    existing, err := s.repo.ListAccounts(ctx, userID)
    if err != nil { return ledger.Account{}, err }
    for _, a := range existing {
        if strings.EqualFold(a.Currency, currency) && strings.EqualFold(normalizedPathString(a), "equity:openingbalances") {
            return a, nil
        }
    }
    // create if missing
    a := ledger.Account{
        ID:       uuid.New(),
        UserID:   userID,
        Name:     "Opening Balances",
        Currency: currency,
        Type:     ledger.AccountTypeEquity,
        Method:   "OpeningBalances",
        Vendor:   "System",
        System:   true,
        Metadata: map[string]string{"active": "true"},
    }
    if err := s.ValidateCreate(a); err != nil { return ledger.Account{}, err }
    created, err := s.writer.CreateAccount(ctx, a)
    if err != nil { return ledger.Account{}, err }
    return created, nil
}

func (s *service) ValidateCreate(account ledger.Account) error {
    if account.UserID == uuid.Nil {
        return errs.ErrInvalid
    }
    if account.Name == "" {
        return errors.New("name is required")
    }
    if account.Currency == "" {
        return errors.New("currency is required")
    }
    if account.Method == "" {
        return errors.New("method is required")
    }
    if account.Vendor == "" {
        return errors.New("vendor is required")
    }
    switch account.Type {
    case ledger.AccountTypeAsset, ledger.AccountTypeLiability, ledger.AccountTypeEquity, ledger.AccountTypeRevenue, ledger.AccountTypeExpense:
        // ok
    default:
        return errors.New("invalid account type")
    }
    // OpeningBalances/system rules
    if account.System || strings.EqualFold(account.Method, "OpeningBalances") {
        if account.Type != ledger.AccountTypeEquity {
            return errors.New("opening balances must be equity type")
        }
        if !strings.EqualFold(account.Method, "OpeningBalances") {
            return errors.New("invalid system account method; expected OpeningBalances")
        }
    }
    return nil
}

func (s *service) Create(ctx context.Context, account ledger.Account) (ledger.Account, error) {
    if err := s.ValidateCreate(account); err != nil {
        return ledger.Account{}, err
    }
    // Ensure OpeningBalances system account exists for this currency
    if _, err := s.EnsureOpeningBalanceAccount(ctx, account.UserID, account.Currency); err != nil { return ledger.Account{}, err }
    // Ensure uniqueness over (user, path, currency)
    existing, err := s.repo.ListAccounts(ctx, account.UserID)
    if err != nil {
        return ledger.Account{}, err
    }
    desired := normalizedPathString(ledger.Account{Type: account.Type, Method: account.Method, Vendor: account.Vendor, System: account.System})
    for _, a := range existing {
        if a.UserID == account.UserID && strings.EqualFold(normalizedPathString(a), desired) && strings.EqualFold(a.Currency, account.Currency) {
            return ledger.Account{}, ErrPathExists
        }
    }
    acc := ledger.Account{
        ID:       uuid.New(),
        UserID:   account.UserID,
        Name:     account.Name,
        Currency: account.Currency,
        Type:     account.Type,
        Method:   account.Method,
        Vendor:   account.Vendor,
        System:   account.System,
    }
    if acc.Type == ledger.AccountTypeEquity && strings.EqualFold(acc.Method, "OpeningBalances") {
        acc.Vendor = "System"
        acc.System = true
        if acc.Metadata == nil { acc.Metadata = map[string]string{} }
        acc.Metadata["active"] = "true"
    }
    return s.writer.CreateAccount(ctx, acc)
}

func (s *service) List(ctx context.Context, userID uuid.UUID) ([]ledger.Account, error) {
    if userID == uuid.Nil {
        return nil, errs.ErrInvalid
    }
    return s.repo.ListAccounts(ctx, userID)
}

// normalizedPathString returns the normalized path for uniqueness checks.
// OpeningBalances are represented as "equity:openingbalances" irrespective of vendor.
func normalizedPathString(a ledger.Account) string {
    if a.Type == ledger.AccountTypeEquity && strings.EqualFold(a.Method, "OpeningBalances") {
        return "equity:openingbalances"
    }
    return string(a.Type) + ":" + strings.ToLower(a.Method) + ":" + strings.ToLower(a.Vendor)
}

// ErrPathExists indicates an account with the same normalized path already exists for the user.
var ErrPathExists = errors.New("account path already exists for user")

// Update applies allowed changes to name/method/vendor/metadata using a complete domain account.
func (s *service) Update(ctx context.Context, a ledger.Account) (ledger.Account, error) {
    if a.UserID == uuid.Nil || a.ID == uuid.Nil {
        return ledger.Account{}, errs.ErrInvalid
    }
    current, err := s.repo.GetAccount(ctx, a.UserID, a.ID)
    if err != nil { return ledger.Account{}, err }
    if current.UserID != a.UserID { return ledger.Account{}, errs.ErrForbidden }
    if current.System {
        return ledger.Account{}, errs.ErrSystemAccount
    }
    // Enforce immutability on Type/Currency
    if current.Type != a.Type || current.Currency != a.Currency {
        return ledger.Account{}, errs.ErrImmutable
    }
    // Forbid toggling system flag via update
    if a.System != current.System { return ledger.Account{}, errs.ErrImmutable }
    // If method/vendor changed, ensure unique (user, path, currency)
    if current.Method != a.Method || current.Vendor != a.Vendor {
        existing, err := s.repo.ListAccounts(ctx, a.UserID)
        if err != nil { return ledger.Account{}, err }
        desired := normalizedPathString(a)
        for _, other := range existing {
            if other.ID == a.ID { continue }
            if other.UserID == a.UserID && strings.EqualFold(normalizedPathString(other), desired) && strings.EqualFold(other.Currency, a.Currency) {
                return ledger.Account{}, ErrPathExists
            }
        }
    }
    return s.writer.UpdateAccount(ctx, a)
}

// Deactivate sets metadata["active"]="false". No-op if system=true.
func (s *service) Deactivate(ctx context.Context, userID, accountID uuid.UUID) error {
    if userID == uuid.Nil || accountID == uuid.Nil {
        return errs.ErrInvalid
    }
    acc, err := s.repo.GetAccount(ctx, userID, accountID)
    if err != nil { return err }
    if acc.UserID != userID { return errs.ErrForbidden }
    if acc.System {
        return errs.ErrSystemAccount
    }
    if acc.Metadata == nil { acc.Metadata = map[string]string{} }
    acc.Metadata["active"] = "false"
    if _, err := s.writer.UpdateAccount(ctx, acc); err != nil { return err }
    return nil
}

// Merge functionality intentionally omitted per design: perform merges by
// posting a transfer entry externally and deactivating the source account.
