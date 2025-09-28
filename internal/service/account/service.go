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
    "github.com/tinoosan/ledger/internal/slug"
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
    EnsureAccountsBatch(ctx context.Context, userID uuid.UUID, specs []ledger.Account) ([]ledger.Account, []ItemError, error)
}

type service struct {
    repo   Repo
    writer Writer
}

func New(repo Repo, writer Writer) Service { return &service{repo: repo, writer: writer} }

// ItemError represents a per-item failure in a batch operation.
type ItemError struct {
    Index int
    Code  string
    Err   error
}

// EnsureOpeningBalanceAccount returns the OpeningBalances system account for the currency,
// creating it if missing (idempotent per (user, currency)).
func (s *service) EnsureOpeningBalanceAccount(ctx context.Context, userID uuid.UUID, currency string) (ledger.Account, error) {
    if userID == uuid.Nil || currency == "" { return ledger.Account{}, errs.ErrInvalid }
    currency = strings.ToUpper(currency)
    existing, err := s.repo.ListAccounts(ctx, userID)
    if err != nil { return ledger.Account{}, err }
    for _, a := range existing {
        if strings.EqualFold(a.Currency, currency) && strings.EqualFold(normalizedPathString(a), "equity:opening_balances") {
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
        Group:    "opening_balances",
        Vendor:   "System",
        System:   true,
        Active:   true,
    }
    if err := s.ValidateCreate(a); err != nil { return ledger.Account{}, err }
    created, err := s.writer.CreateAccount(ctx, a)
    if err != nil { return ledger.Account{}, err }
    return created, nil
}

func (s *service) ValidateCreate(account ledger.Account) error {
    // Normalize currency to uppercase
    if account.Currency != "" { account.Currency = strings.ToUpper(account.Currency) }
    if account.UserID == uuid.Nil {
        return errs.ErrInvalid
    }
    if account.Name == "" {
        return errors.New("name is required")
    }
    if account.Currency == "" {
        return errors.New("currency is required")
    }
    if account.Group == "" {
        return errors.New("group is required")
    }
    if !slug.IsSlug(strings.ToLower(account.Group)) { return errors.New("invalid group slug") }
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
    if account.System || strings.EqualFold(account.Group, "opening_balances") {
        if account.Type != ledger.AccountTypeEquity {
            return errors.New("opening balances must be equity type")
        }
        if !strings.EqualFold(account.Group, "opening_balances") {
            return errors.New("invalid system account group; expected opening_balances")
        }
    }
    return nil
}

// EnsureAccountsBatch validates all specs and, if valid, creates them all.
// If any item fails validation or conflicts, no account is created and per-item errors are returned.
func (s *service) EnsureAccountsBatch(ctx context.Context, userID uuid.UUID, specs []ledger.Account) ([]ledger.Account, []ItemError, error) {
    if userID == uuid.Nil { return nil, nil, errs.ErrInvalid }
    // Normalize and validate all first
    errsList := make([]ItemError, 0)
    normalized := make([]ledger.Account, len(specs))
    for i, in := range specs {
        in.UserID = userID
        // Normalize path casing
        in.Group = strings.TrimSpace(in.Group)
        in.Vendor = strings.TrimSpace(in.Vendor)
        in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
        normalized[i] = in
        if err := s.ValidateCreate(in); err != nil {
            errsList = append(errsList, ItemError{Index: i, Code: "validation_error", Err: err})
            continue
        }
    }
    if len(errsList) > 0 { return nil, errsList, nil }
    // Ensure OpeningBalances per currency exist
    seenCurr := map[string]struct{}{}
    for _, a := range normalized {
        if _, ok := seenCurr[a.Currency]; ok { continue }
        if _, err := s.EnsureOpeningBalanceAccount(ctx, userID, a.Currency); err != nil {
            return nil, nil, err
        }
        seenCurr[a.Currency] = struct{}{}
    }
    // Uniqueness validation against current state
    existing, err := s.repo.ListAccounts(ctx, userID)
    if err != nil { return nil, nil, err }
    // detect intra-batch duplicates and conflicts with existing
    seen := make(map[string]int)
    for i, a := range normalized {
        desired := normalizedPathString(a) + "|" + strings.ToUpper(a.Currency)
        if prevIdx, ok := seen[desired]; ok {
            errsList = append(errsList, ItemError{Index: i, Code: "conflict", Err: ErrPathExists})
            errsList = append(errsList, ItemError{Index: prevIdx, Code: "conflict", Err: ErrPathExists})
            continue
        }
        seen[desired] = i
        for _, other := range existing {
            if other.UserID == userID && strings.EqualFold(normalizedPathString(other), normalizedPathString(a)) && strings.EqualFold(other.Currency, a.Currency) {
                errsList = append(errsList, ItemError{Index: i, Code: "conflict", Err: ErrPathExists})
                break
            }
        }
    }
    if len(errsList) > 0 { return nil, errsList, nil }
    // All good: create all under transaction if available
    type txBeginner interface{ BeginTx(context.Context) (interface{ CreateAccount(context.Context, ledger.Account) (ledger.Account, error); Commit(context.Context) error; Rollback(context.Context) error }, error) }
    if b, ok := s.writer.(txBeginner); ok {
        tx, err := b.BeginTx(ctx)
        if err != nil { return nil, nil, err }
        created := make([]ledger.Account, 0, len(normalized))
        for _, a := range normalized {
            acc := ledger.Account{
                ID:       uuid.New(),
                UserID:   a.UserID,
                Name:     a.Name,
                Currency: a.Currency,
                Type:     a.Type,
                Group:    a.Group,
                Vendor:   a.Vendor,
                System:   a.System,
                Active:   true,
                Metadata: a.Metadata,
            }
            if acc.Type == ledger.AccountTypeEquity && strings.EqualFold(acc.Group, "opening_balances") { acc.Vendor = "System"; acc.System = true }
            if _, err := tx.CreateAccount(ctx, acc); err != nil { _ = tx.Rollback(ctx); return nil, nil, err }
            created = append(created, acc)
        }
        if err := tx.Commit(ctx); err != nil { return nil, nil, err }
        return created, nil, nil
    }
    // Fallback (non-tx)
    created := make([]ledger.Account, 0, len(normalized))
    for _, a := range normalized {
        acc, err := s.Create(ctx, a)
        if err != nil { return nil, nil, err }
        created = append(created, acc)
    }
    return created, nil, nil
}

func (s *service) Create(ctx context.Context, account ledger.Account) (ledger.Account, error) {
    // Normalize currency again to be safe
    if account.Currency != "" { account.Currency = strings.ToUpper(account.Currency) }
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
    desired := normalizedPathString(ledger.Account{Type: account.Type, Group: account.Group, Vendor: account.Vendor, System: account.System})
    for _, a := range existing {
        if a.UserID == account.UserID && strings.EqualFold(normalizedPathString(a), desired) && strings.EqualFold(a.Currency, account.Currency) {
            return ledger.Account{}, ErrPathExists
        }
    }
    accNew := ledger.Account{ID: uuid.New(), UserID: account.UserID, Name: account.Name, Currency: account.Currency, Type: account.Type, Group: account.Group, Vendor: account.Vendor, System: account.System, Active: true, Metadata: account.Metadata}
    if accNew.Type == ledger.AccountTypeEquity && strings.EqualFold(accNew.Group, "opening_balances") { accNew.Vendor = "System"; accNew.System = true }
    return s.writer.CreateAccount(ctx, accNew)
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
    if a.Type == ledger.AccountTypeEquity && strings.EqualFold(a.Group, "opening_balances") {
        return "equity:opening_balances"
    }
    vendorSlug := slug.Slugify(a.Vendor)
    return strings.ToLower(string(a.Type)) + ":" + strings.ToLower(a.Group) + ":" + vendorSlug
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
    // If group/vendor changed, ensure unique (user, path, currency)
    if current.Group != a.Group || current.Vendor != a.Vendor {
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

// Deactivate sets Active=false (soft delete). No-op if system=true.
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
    acc.Active = false
    if _, err := s.writer.UpdateAccount(ctx, acc); err != nil { return err }
    return nil
}

// Merge functionality intentionally omitted per design: perform merges by
// posting a transfer entry externally and deactivating the source account.
