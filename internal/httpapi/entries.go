package httpapi

import (
    "net/http"
    "time"

    "github.com/google/uuid"
    "github.com/tinoosan/ledger/internal/ledger"
    "github.com/tinoosan/ledger/internal/service/journal"
)

func (s *Server) postEntry(w http.ResponseWriter, r *http.Request) {
    // Request has already been validated and is present in context
    v := r.Context().Value(ctxKeyPostEntry)
    in, ok := v.(journal.EntryInput)
    if !ok {
        toJSON(w, http.StatusInternalServerError, errorResponse{Error: "validated request missing"})
        return
    }

    saved, err := s.svc.CreateEntry(r.Context(), in)
    if err != nil {
        toJSON(w, http.StatusInternalServerError, errorResponse{Error: "could not persist entry"})
        return
    }

    // Format response
    resp := toEntryResponse(saved)
    toJSON(w, http.StatusCreated, resp)
}

// reverseEntry handles POST /entries/reverse
func (s *Server) reverseEntry(w http.ResponseWriter, r *http.Request) {
    v := r.Context().Value(ctxKeyReverseEntry)
    req, ok := v.(reverseEntryRequest)
    if !ok {
        toJSON(w, http.StatusInternalServerError, errorResponse{Error: "validated request missing"})
        return
    }
    date := time.Now().UTC()
    if req.Date != nil {
        date = req.Date.UTC()
    }
    saved, err := s.svc.ReverseEntry(r.Context(), req.UserID, req.EntryID, date)
    if err != nil {
        toJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
        return
    }
    toJSON(w, http.StatusCreated, toEntryResponse(saved))
}

// trialBalance handles GET /trial-balance
func (s *Server) trialBalance(w http.ResponseWriter, r *http.Request) {
    v := r.Context().Value(ctxKeyTrialBalance)
    q, ok := v.(trialBalanceQuery)
    if !ok {
        toJSON(w, http.StatusInternalServerError, errorResponse{Error: "validated query missing"})
        return
    }
    // Compute TB map of accountID -> net amount
    nets, err := s.svc.TrialBalance(r.Context(), q.UserID, q.AsOf)
    if err != nil {
        toJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
        return
    }
    // Load account details
    ids := make([]uuid.UUID, 0, len(nets))
    for id := range nets { ids = append(ids, id) }
    accs, err := s.repo.AccountsByIDs(r.Context(), q.UserID, ids)
    if err != nil {
        toJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load accounts"})
        return
    }
    // Build response
    resp := trialBalanceResponse{UserID: q.UserID, AsOf: q.AsOf}
    resp.Accounts = make([]trialBalanceAccount, 0, len(nets))
    for id, amt := range nets {
        acc := accs[id]
        units, _ := amt.MinorUnits()
        var debit, credit int64
        if units >= 0 { debit, credit = units, 0 } else { debit, credit = 0, -units }
        resp.Accounts = append(resp.Accounts, trialBalanceAccount{
            AccountID: id,
            Name: acc.Name,
            Path: acc.Path(),
            Currency: acc.Currency,
            DebitMinor: debit,
            CreditMinor: credit,
            Type: acc.Type,
        })
    }
    toJSON(w, http.StatusOK, resp)
}

// listEntries handles GET /entries
func (s *Server) listEntries(w http.ResponseWriter, r *http.Request) {
    v := r.Context().Value(ctxKeyListEntries)
    q, ok := v.(listEntriesQuery)
    if !ok {
        toJSON(w, http.StatusInternalServerError, errorResponse{Error: "validated query missing"})
        return
    }
    entries, err := s.svc.ListEntries(r.Context(), q.UserID)
    if err != nil {
        toJSON(w, http.StatusInternalServerError, errorResponse{Error: "could not fetch entries"})
        return
    }
    // Map entries to response
    out := make([]entryResponse, 0, len(entries))
    for _, e := range entries {
        out = append(out, toEntryResponse(e))
    }
    toJSON(w, http.StatusOK, out)
}

func toEntryResponse(e ledger.JournalEntry) entryResponse {
    lines := make([]lineResponse, 0, len(e.Lines.ByID))
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
