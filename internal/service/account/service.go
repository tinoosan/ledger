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
}

type Writer interface {
    CreateAccount(ctx context.Context, a ledger.Account) (ledger.Account, error)
}

type Service interface {
    ValidateCreateInput(in CreateInput) error
    Create(ctx context.Context, in CreateInput) (ledger.Account, error)
    List(ctx context.Context, userID uuid.UUID) ([]ledger.Account, error)
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
