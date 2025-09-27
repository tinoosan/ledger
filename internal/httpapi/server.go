package httpapi

import (
    "context"
    "encoding/json"
    "errors"
    "net/http"
    "time"

    "github.com/google/uuid"
    "github.com/govalues/money"

    "github.com/tinoosan/ledger/internal/ledger"
)

// Repository abstracts read-side operations needed by the API.
type Repository interface {
    // AccountsByIDs returns accounts for the given user filtered by the provided ids.
    AccountsByIDs(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]ledger.Account, error)
}

// Writer abstracts write-side operations needed by the API.
type Writer interface {
    // CreateJournalEntry persists the entry and its lines atomically.
    CreateJournalEntry(ctx context.Context, entry ledger.JournalEntry) (ledger.JournalEntry, error)
}

// Server wires handlers to a mux.
type Server struct {
    repo   Repository
    writer Writer
    mux    *http.ServeMux
}

// New constructs a Server instance.
func New(repo Repository, writer Writer) *Server {
    s := &Server{repo: repo, writer: writer, mux: http.NewServeMux()}
    s.routes()
    return s
}

// Mux exposes the configured http.Handler.
func (s *Server) Mux() http.Handler { return s.mux }

func (s *Server) routes() {
    s.mux.HandleFunc("/entries", s.handlePostEntry)
}

type postEntryRequest struct {
    UserID        uuid.UUID        `json:"user_id"`
    Date          time.Time        `json:"date"`
    Currency      string           `json:"currency"`
    Memo          string           `json:"memo"`
    Category      ledger.Category  `json:"category"`
    ClientEntryID string           `json:"client_entry_id"`
    Lines         []postEntryLine  `json:"lines"`
}

type postEntryLine struct {
    AccountID   uuid.UUID   `json:"account_id"`
    Side        ledger.Side `json:"side"`
    AmountMinor int64       `json:"amount_minor"`
}

type errorResponse struct {
    Error string `json:"error"`
}

type entryResponse struct {
    ID            uuid.UUID       `json:"id"`
    UserID        uuid.UUID       `json:"user_id"`
    Date          time.Time       `json:"date"`
    Currency      string          `json:"currency"`
    Memo          string          `json:"memo"`
    Category      ledger.Category `json:"category"`
    ClientEntryID string          `json:"client_entry_id"`
    Lines         []lineResponse  `json:"lines"`
}

type lineResponse struct {
    ID          uuid.UUID   `json:"id"`
    AccountID   uuid.UUID   `json:"account_id"`
    Side        ledger.Side `json:"side"`
    AmountMinor int64       `json:"amount_minor"`
}

func (s *Server) handlePostEntry(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        w.WriteHeader(http.StatusMethodNotAllowed)
        json.NewEncoder(w).Encode(errorResponse{Error: "method not allowed"})
        return
    }

    var req postEntryRequest
    dec := json.NewDecoder(r.Body)
    dec.DisallowUnknownFields()
    if err := dec.Decode(&req); err != nil {
        w.WriteHeader(http.StatusBadRequest)
        _ = json.NewEncoder(w).Encode(errorResponse{Error: "invalid JSON: " + err.Error()})
        return
    }

    if err := validatePostEntryRequest(r.Context(), s.repo, &req); err != nil {
        w.WriteHeader(http.StatusBadRequest)
        _ = json.NewEncoder(w).Encode(errorResponse{Error: err.Error()})
        return
    }

    // Build domain entry
    entryID := uuid.New()
    jl := ledger.JournalLines{ByID: make(map[uuid.UUID]*ledger.JournalLine, len(req.Lines))}
    for _, ln := range req.Lines {
        amt, _ := money.NewAmountFromMinorUnits(req.Currency, ln.AmountMinor)
        lineID := uuid.New()
        jl.ByID[lineID] = &ledger.JournalLine{
            ID:        lineID,
            EntryID:   entryID,
            AccountID: ln.AccountID,
            Side:      ln.Side,
            Amount:    amt,
            Metadata:  nil,
        }
    }

    entry := ledger.JournalEntry{
        ID:            entryID,
        UserID:        req.UserID,
        Date:          req.Date,
        Currency:      req.Currency,
        Memo:          req.Memo,
        Category:      req.Category,
        ClientEntryID: req.ClientEntryID,
        Lines:         jl,
    }

    saved, err := s.writer.CreateJournalEntry(r.Context(), entry)
    if err != nil {
        w.WriteHeader(http.StatusInternalServerError)
        _ = json.NewEncoder(w).Encode(errorResponse{Error: "could not persist entry"})
        return
    }

    // Format response
    resp := toEntryResponse(saved)
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated)
    _ = json.NewEncoder(w).Encode(resp)
}

func toEntryResponse(e ledger.JournalEntry) entryResponse {
    var lines []lineResponse
    lines = make([]lineResponse, 0, len(e.Lines.ByID))
    for id, ln := range e.Lines.ByID {
        units, _ := ln.Amount.MinorUnits()
        lines = append(lines, lineResponse{
            ID:          id,
            AccountID:   ln.AccountID,
            Side:        ln.Side,
            AmountMinor: units,
        })
    }
    return entryResponse{
        ID:            e.ID,
        UserID:        e.UserID,
        Date:          e.Date,
        Currency:      e.Currency,
        Memo:          e.Memo,
        Category:      e.Category,
        ClientEntryID: e.ClientEntryID,
        Lines:         lines,
    }
}

func validatePostEntryRequest(ctx context.Context, repo Repository, req *postEntryRequest) error {
    if req.UserID == uuid.Nil {
        return errors.New("user_id is required")
    }
    if req.Currency == "" {
        return errors.New("currency is required")
    }
    if len(req.Lines) < 2 {
        return errors.New("at least 2 lines")
    }

    // Gather account IDs and validate basic fields
    ids := make([]uuid.UUID, 0, len(req.Lines))
    var sumDebits, sumCredits int64
    for i, ln := range req.Lines {
        if ln.AccountID == uuid.Nil {
            return errors.New("line[" + itoa(i) + "]: account_id required")
        }
        if ln.AmountMinor <= 0 {
            return errors.New("line[" + itoa(i) + "]: amount must be > 0")
        }
        switch ln.Side {
        case ledger.SideDebit:
            sumDebits += ln.AmountMinor
        case ledger.SideCredit:
            sumCredits += ln.AmountMinor
        default:
            return errors.New("line[" + itoa(i) + "]: side must be debit or credit")
        }
        ids = append(ids, ln.AccountID)
    }
    if sumDebits != sumCredits {
        return errors.New("sum(debits) must equal sum(credits)")
    }

    // Validate accounts belong to the user and currency is consistent
    accMap, err := repo.AccountsByIDs(ctx, req.UserID, ids)
    if err != nil {
        return err
    }
    if len(accMap) != len(unique(ids)) {
        return errors.New("unknown or unauthorized accounts")
    }
    for i, ln := range req.Lines {
        acc, ok := accMap[ln.AccountID]
        if !ok {
            return errors.New("line[" + itoa(i) + "]: account not found for user")
        }
        if acc.UserID != req.UserID {
            return errors.New("line[" + itoa(i) + "]: account does not belong to user")
        }
        if acc.Currency != req.Currency {
            return errors.New("line[" + itoa(i) + "]: account currency mismatch")
        }
    }

    return nil
}

func unique(ids []uuid.UUID) []uuid.UUID {
    seen := make(map[uuid.UUID]struct{}, len(ids))
    out := make([]uuid.UUID, 0, len(ids))
    for _, id := range ids {
        if _, ok := seen[id]; ok {
            continue
        }
        seen[id] = struct{}{}
        out = append(out, id)
    }
    return out
}

// small, allocation-free int to string for errors
func itoa(n int) string {
    if n == 0 {
        return "0"
    }
    neg := false
    if n < 0 {
        neg = true
        n = -n
    }
    var buf [20]byte
    i := len(buf)
    for n > 0 {
        i--
        buf[i] = byte('0' + n%10)
        n /= 10
    }
    if neg {
        i--
        buf[i] = '-'
    }
    return string(buf[i:])
}

