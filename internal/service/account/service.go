package account

import (
    "context"
    "errors"

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
    acc := ledger.Account{
        ID:       uuid.New(),
        UserID:   in.UserID,
        Name:     in.Name,
        Currency: in.Currency,
        Type:     in.Type,
    }
    return s.writer.CreateAccount(ctx, acc)
}

func (s *service) List(ctx context.Context, userID uuid.UUID) ([]ledger.Account, error) {
    if userID == uuid.Nil {
        return nil, errors.New("user_id is required")
    }
    return s.repo.AccountsByUserID(ctx, userID)
}

