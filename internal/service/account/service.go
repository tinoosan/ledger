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
    AccountsByUserID(ctx context.Context, userID uuid.UUID) ([]ledger.Account, error)
    AccountByID(ctx context.Context, userID, accountID uuid.UUID) (ledger.Account, error)
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
}

type service struct {
    repo   Repo
    writer Writer
}

func New(repo Repo, writer Writer) Service { return &service{repo: repo, writer: writer} }

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
    return nil
}

func (s *service) Create(ctx context.Context, account ledger.Account) (ledger.Account, error) {
    if err := s.ValidateCreate(account); err != nil {
        return ledger.Account{}, err
    }
    // Ensure unique path per user (case-insensitive on method/vendor)
    existing, err := s.repo.AccountsByUserID(ctx, account.UserID)
    if err != nil {
        return ledger.Account{}, err
    }
    desired := pathKey(account.Type, account.Method, account.Vendor)
    for _, a := range existing {
        if a.UserID == account.UserID && pathKey(a.Type, a.Method, a.Vendor) == desired {
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
    }
    return s.writer.CreateAccount(ctx, acc)
}

func (s *service) List(ctx context.Context, userID uuid.UUID) ([]ledger.Account, error) {
    if userID == uuid.Nil {
        return nil, errs.ErrInvalid
    }
    return s.repo.AccountsByUserID(ctx, userID)
}

// pathKey returns a normalized path Type:method:vendor for uniqueness checks.
func pathKey(t ledger.AccountType, method, vendor string) string {
    return string(t) + ":" + strings.ToLower(method) + ":" + strings.ToLower(vendor)
}

// ErrPathExists indicates an account with the same normalized path already exists for the user.
var ErrPathExists = errors.New("account path already exists for user")

// Update applies allowed changes to name/method/vendor/metadata using a complete domain account.
func (s *service) Update(ctx context.Context, a ledger.Account) (ledger.Account, error) {
    if a.UserID == uuid.Nil || a.ID == uuid.Nil {
        return ledger.Account{}, errs.ErrInvalid
    }
    current, err := s.repo.AccountByID(ctx, a.UserID, a.ID)
    if err != nil { return ledger.Account{}, err }
    if current.UserID != a.UserID { return ledger.Account{}, errs.ErrForbidden }
    if current.Metadata != nil && strings.EqualFold(current.Metadata["system"], "true") {
        return ledger.Account{}, errs.ErrSystemAccount
    }
    // Enforce immutability on Type/Currency
    if current.Type != a.Type || current.Currency != a.Currency {
        return ledger.Account{}, errs.ErrImmutable
    }
    // If method/vendor changed, ensure unique path
    if current.Method != a.Method || current.Vendor != a.Vendor {
        existing, err := s.repo.AccountsByUserID(ctx, a.UserID)
        if err != nil { return ledger.Account{}, err }
        desired := pathKey(a.Type, a.Method, a.Vendor)
        for _, other := range existing {
            if other.ID == a.ID { continue }
            if other.UserID == a.UserID && pathKey(other.Type, other.Method, other.Vendor) == desired {
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
    acc, err := s.repo.AccountByID(ctx, userID, accountID)
    if err != nil { return err }
    if acc.UserID != userID { return errs.ErrForbidden }
    if acc.Metadata != nil && strings.EqualFold(acc.Metadata["system"], "true") {
        return errs.ErrSystemAccount
    }
    if acc.Metadata == nil { acc.Metadata = map[string]string{} }
    acc.Metadata["active"] = "false"
    if _, err := s.writer.UpdateAccount(ctx, acc); err != nil { return err }
    return nil
}

// Merge functionality intentionally omitted per design: perform merges by
// posting a transfer entry externally and deactivating the source account.
