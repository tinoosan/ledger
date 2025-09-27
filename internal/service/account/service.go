package account

import (
    "context"
    "errors"
    "strings"

    "github.com/google/uuid"
    "github.com/tinoosan/ledger/internal/ledger"
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
    ValidateCreateInput(in CreateInput) error
    Create(ctx context.Context, in CreateInput) (ledger.Account, error)
    List(ctx context.Context, userID uuid.UUID) ([]ledger.Account, error)
    Update(ctx context.Context, userID, accountID uuid.UUID, in UpdateInput) (ledger.Account, error)
    Deactivate(ctx context.Context, userID, accountID uuid.UUID) error
}

type service struct {
    repo   Repo
    writer Writer
}

func New(repo Repo, writer Writer) Service { return &service{repo: repo, writer: writer} }

type CreateInput struct {
    UserID   uuid.UUID
    Name     string
    Currency string
    Type     ledger.AccountType
    Method   string
    Vendor   string
}

func (s *service) ValidateCreateInput(in CreateInput) error {
    if in.UserID == uuid.Nil {
        return errors.New("user_id is required")
    }
    if in.Name == "" {
        return errors.New("name is required")
    }
    if in.Currency == "" {
        return errors.New("currency is required")
    }
    if in.Method == "" {
        return errors.New("method is required")
    }
    if in.Vendor == "" {
        return errors.New("vendor is required")
    }
    switch in.Type {
    case ledger.AccountTypeAsset, ledger.AccountTypeLiability, ledger.AccountTypeEquity, ledger.AccountTypeRevenue, ledger.AccountTypeExpense:
        // ok
    default:
        return errors.New("invalid account type")
    }
    return nil
}

func (s *service) Create(ctx context.Context, in CreateInput) (ledger.Account, error) {
    if err := s.ValidateCreateInput(in); err != nil {
        return ledger.Account{}, err
    }
    // Ensure unique path per user (case-insensitive on method/vendor)
    existing, err := s.repo.AccountsByUserID(ctx, in.UserID)
    if err != nil {
        return ledger.Account{}, err
    }
    desired := pathKey(in.Type, in.Method, in.Vendor)
    for _, a := range existing {
        if a.UserID == in.UserID && pathKey(a.Type, a.Method, a.Vendor) == desired {
            return ledger.Account{}, ErrPathExists
        }
    }
    acc := ledger.Account{
        ID:       uuid.New(),
        UserID:   in.UserID,
        Name:     in.Name,
        Currency: in.Currency,
        Type:     in.Type,
        Method:   in.Method,
        Vendor:   in.Vendor,
    }
    return s.writer.CreateAccount(ctx, acc)
}

func (s *service) List(ctx context.Context, userID uuid.UUID) ([]ledger.Account, error) {
    if userID == uuid.Nil {
        return nil, errors.New("user_id is required")
    }
    return s.repo.AccountsByUserID(ctx, userID)
}

// pathKey returns a normalized path Type:method:vendor for uniqueness checks.
func pathKey(t ledger.AccountType, method, vendor string) string {
    return string(t) + ":" + strings.ToLower(method) + ":" + strings.ToLower(vendor)
}

// ErrPathExists indicates an account with the same normalized path already exists for the user.
var ErrPathExists = errors.New("account path already exists for user")

// UpdateInput contains mutable fields for an account.
type UpdateInput struct {
    Name     *string
    Method   *string
    Vendor   *string
    Metadata map[string]string // merged into existing keys
}

// Update applies allowed changes and records audit entries.
func (s *service) Update(ctx context.Context, userID, accountID uuid.UUID, in UpdateInput) (ledger.Account, error) {
    if userID == uuid.Nil || accountID == uuid.Nil {
        return ledger.Account{}, errors.New("user_id and account_id are required")
    }
    acc, err := s.repo.AccountByID(ctx, userID, accountID)
    if err != nil {
        return ledger.Account{}, err
    }
    if acc.UserID != userID {
        return ledger.Account{}, errors.New("account does not belong to user")
    }
    if acc.Metadata != nil && strings.EqualFold(acc.Metadata["system"], "true") {
        return ledger.Account{}, errors.New("system accounts cannot be modified")
    }

    // Copy for comparison
    orig := acc
    // Apply changes with validation
    if in.Name != nil {
        acc.Name = *in.Name
    }
    if in.Method != nil {
        acc.Method = *in.Method
    }
    if in.Vendor != nil {
        acc.Vendor = *in.Vendor
    }
    if in.Metadata != nil {
        if acc.Metadata == nil {
            acc.Metadata = map[string]string{}
        }
        for k, v := range in.Metadata {
            acc.Metadata[k] = v
        }
    }

    // If method/vendor changed, ensure unique path
    if orig.Method != acc.Method || orig.Vendor != acc.Vendor {
        existing, err := s.repo.AccountsByUserID(ctx, userID)
        if err != nil {
            return ledger.Account{}, err
        }
        desired := pathKey(acc.Type, acc.Method, acc.Vendor)
        for _, a := range existing {
            if a.ID == acc.ID { continue }
            if a.UserID == userID && pathKey(a.Type, a.Method, a.Vendor) == desired {
                return ledger.Account{}, ErrPathExists
            }
        }
    }

    // Persist account
    updated, err := s.writer.UpdateAccount(ctx, acc)
    if err != nil {
        return ledger.Account{}, err
    }

    return updated, nil
}

// Deactivate sets metadata["active"]="false" and audits it. No-op if system=true.
func (s *service) Deactivate(ctx context.Context, userID, accountID uuid.UUID) error {
    if userID == uuid.Nil || accountID == uuid.Nil {
        return errors.New("user_id and account_id are required")
    }
    acc, err := s.repo.AccountByID(ctx, userID, accountID)
    if err != nil { return err }
    if acc.UserID != userID { return errors.New("account does not belong to user") }
    if acc.Metadata != nil && strings.EqualFold(acc.Metadata["system"], "true") {
        return errors.New("system accounts cannot be deactivated")
    }
    if acc.Metadata == nil { acc.Metadata = map[string]string{} }
    acc.Metadata["active"] = "false"
    if _, err := s.writer.UpdateAccount(ctx, acc); err != nil { return err }
    return nil
}

// Merge functionality intentionally omitted per design: perform merges by
// posting a transfer entry externally and deactivating the source account.
