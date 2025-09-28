// Entry handlers: creation, reversal, listing and reporting endpoints.
package httpapi

import (
    "encoding/base64"
    "net/http"
    "sort"
    "strconv"
    "strings"
    "time"

    "github.com/google/uuid"
    "github.com/tinoosan/ledger/internal/ledger"
    "github.com/govalues/money"
)

func (s *Server) postEntry(w http.ResponseWriter, r *http.Request) {
    // Request has already been validated and is present in context
    v := r.Context().Value(ctxKeyPostEntry)
    in, ok := v.(ledger.JournalEntry)
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
        // decimals using money.Amount.Decimal()
        dstr, cstr := "0", "0"
        if debit > 0 {
            if a, err := money.NewAmountFromMinorUnits(acc.Currency, debit); err == nil {
                dstr = a.Decimal().String()
            }
        }
        if credit > 0 {
            if a, err := money.NewAmountFromMinorUnits(acc.Currency, credit); err == nil {
                cstr = a.Decimal().String()
            }
        }
        resp.Accounts = append(resp.Accounts, trialBalanceAccount{
            AccountID: id,
            Name: acc.Name,
            Path: acc.Path(),
            Currency: acc.Currency,
            DebitMinor: debit,
            CreditMinor: credit,
            Debit: dstr,
            Credit: cstr,
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
    // ensure deterministic order: Date asc, ID asc
    sort.Slice(entries, func(i, j int) bool {
        if entries[i].Date.Equal(entries[j].Date) {
            return entries[i].ID.String() < entries[j].ID.String()
        }
        return entries[i].Date.Before(entries[j].Date)
    })
    // parse optional params
    var from, to *time.Time
    if raw := r.URL.Query().Get("from"); raw != "" {
        if t, err := time.Parse(time.RFC3339, raw); err == nil { tt := t.UTC(); from = &tt } else { toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid from"}); return }
    }
    if raw := r.URL.Query().Get("to"); raw != "" {
        if t, err := time.Parse(time.RFC3339, raw); err == nil { tt := t.UTC(); to = &tt } else { toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid to"}); return }
    }
    lim := 50
    if raw := r.URL.Query().Get("limit"); raw != "" {
        if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 200 { lim = n }
    }
    cursor := r.URL.Query().Get("cursor")
    // build window by time
    win := make([]ledger.JournalEntry, 0, len(entries))
    for _, e := range entries {
        if from != nil && e.Date.Before(*from) { continue }
        if to != nil && e.Date.After(*to) { continue }
        win = append(win, e)
    }
    // start index from cursor
    start := 0
    if cursor != "" {
        if b, err := base64.StdEncoding.DecodeString(cursor); err == nil {
            parts := strings.Split(string(b), "|")
            if len(parts) == 2 {
                if ts, err := time.Parse(time.RFC3339Nano, parts[0]); err == nil {
                    cid, _ := uuid.Parse(parts[1])
                    for i := range win {
                        if win[i].Date.After(ts) { break }
                        if win[i].Date.Equal(ts) && win[i].ID == cid { start = i + 1; break }
                    }
                }
            }
        }
    }
    end := start + lim
    if end > len(win) { end = len(win) }
    page := win[start:end]
    resp := listEntriesResponse{Items: make([]entryResponse, 0, len(page))}
    for _, e := range page { resp.Items = append(resp.Items, toEntryResponse(e)) }
    if end < len(win) {
        c := base64.StdEncoding.EncodeToString([]byte(page[len(page)-1].Date.Format(time.RFC3339Nano) + "|" + page[len(page)-1].ID.String()))
        resp.NextCursor = &c
    }
    toJSON(w, http.StatusOK, resp)
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
            Amount:      ln.Amount.Decimal().String(),
        })
    }
    return entryResponse{
        ID:            e.ID,
        UserID:        e.UserID,
        Date:          e.Date,
        Currency:      e.Currency,
        Memo:          e.Memo,
        Category:      e.Category,
        Lines:         lines,
    }
}
