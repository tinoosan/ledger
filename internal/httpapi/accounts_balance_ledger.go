// Account balance and ledger endpoints. Running balance is computed per page.
package httpapi

import (
    "encoding/base64"
    "net/http"
    "sort"
    "strconv"
    "strings"
    "time"

    chi "github.com/go-chi/chi/v5"
    "github.com/google/uuid"
    "github.com/govalues/money"
)

// GET /accounts/{id}/balance?user_id=&as_of=
func (s *Server) getAccountBalance(w http.ResponseWriter, r *http.Request) {
    idStr := chi.URLParam(r, "id")
    id, err := uuid.Parse(idStr)
    if err != nil { toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid account id"}); return }
    userIDStr := r.URL.Query().Get("user_id")
    if userIDStr == "" { toJSON(w, http.StatusBadRequest, errorResponse{Error: "user_id is required"}); return }
    userID, err := uuid.Parse(userIDStr); if err != nil { toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid user_id"}); return }
    var asOf *time.Time
    if v := r.URL.Query().Get("as_of"); v != "" {
        t, err := time.Parse(time.RFC3339, v); if err != nil { toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid as_of"}); return }
        tt := t.UTC(); asOf = &tt
    }
    // ensure account exists and owned by user
    if _, err := s.repo.AccountByID(r.Context(), userID, id); err != nil {
        toJSON(w, http.StatusNotFound, errorResponse{Error: "not_found", Code: "not_found"})
        return
    }
    bal, err := s.svc.AccountBalance(r.Context(), userID, id, asOf)
    if err != nil { toJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()}); return }
    units, _ := bal.MinorUnits()
    curr := bal.Curr().Code()
    resp := map[string]any{"user_id": userID, "account_id": id, "as_of": asOf, "currency": curr, "balance_minor": units}
    toJSON(w, http.StatusOK, resp)
}

// GET /accounts/{id}/ledger?user_id=&from=&to=&limit=&cursor=
func (s *Server) getAccountLedger(w http.ResponseWriter, r *http.Request) {
    idStr := chi.URLParam(r, "id")
    id, err := uuid.Parse(idStr)
    if err != nil { toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid account id"}); return }
    userID, err := uuid.Parse(r.URL.Query().Get("user_id"))
    if err != nil { toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid user_id"}); return }
    // ensure account exists and owned by user
    if _, err := s.repo.AccountByID(r.Context(), userID, id); err != nil {
        toJSON(w, http.StatusNotFound, errorResponse{Error: "not_found", Code: "not_found"})
        return
    }
    var from, to *time.Time
    if v := r.URL.Query().Get("from"); v != "" { if t, err := time.Parse(time.RFC3339, v); err == nil { tt := t.UTC(); from = &tt } else { toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid from"}); return } }
    if v := r.URL.Query().Get("to"); v != "" { if t, err := time.Parse(time.RFC3339, v); err == nil { tt := t.UTC(); to = &tt } else { toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid to"}); return } }
    lim := 50
    if v := r.URL.Query().Get("limit"); v != "" { if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 { lim = n } }
    cursor := r.URL.Query().Get("cursor")

    // Build a flat list of lines for the account
    entries, err := s.repo.EntriesByUserID(r.Context(), userID)
    if err != nil { toJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load entries"}); return }
    type rec struct{ date time.Time; entryID, lineID uuid.UUID; side string; amount int64 }
    list := make([]rec, 0, 64)
    var curr string
    for _, e := range entries {
        if from != nil && e.Date.Before(*from) { continue }
        if to != nil && e.Date.After(*to) { continue }
        for lid, ln := range e.Lines.ByID {
            if ln.AccountID != id { continue }
            units, _ := ln.Amount.MinorUnits()
            sd := string(ln.Side)
            list = append(list, rec{date: e.Date, entryID: e.ID, lineID: lid, side: sd, amount: units})
            if curr == "" { curr = ln.Amount.Curr().Code() }
        }
    }
    sort.Slice(list, func(i, j int) bool {
        if list[i].date.Equal(list[j].date) {
            if list[i].entryID == list[j].entryID { return list[i].lineID.String() < list[j].lineID.String() }
            return list[i].entryID.String() < list[j].entryID.String()
        }
        return list[i].date.Before(list[j].date)
    })
    // Apply cursor
    start := 0
    if cursor != "" {
        if b, err := base64.StdEncoding.DecodeString(cursor); err == nil {
            parts := strings.Split(string(b), "|")
            if len(parts) == 2 {
                if ts, err := time.Parse(time.RFC3339Nano, parts[0]); err == nil {
                    cid, _ := uuid.Parse(parts[1])
                    for i := range list {
                        if list[i].date.After(ts) { break }
                        if list[i].date.Equal(ts) && list[i].lineID == cid { start = i + 1; break }
                    }
                }
            }
        }
    }
    // Running balance from beginning to current page start
    bal, err := s.svc.AccountBalance(r.Context(), userID, id, to)
    if err != nil { toJSON(w, http.StatusInternalServerError, errorResponse{Error: "balance error"}); return }
    // But we need balance up to just before page start; recompute if needed
    // Simple approach: recompute from start zero to start index
    if start > 0 {
        // Get currency from bal
        _, _ = bal.MinorUnits()
        // recompute using list[:start]
        for _, rc := range list[:start] {
            // Use sign: debit +, credit - (already using minor units)
            if rc.side == "debit" { bal, _ = bal.Add(mustAmount(curr, rc.amount)) } else { bal, _ = bal.Sub(mustAmount(curr, rc.amount)) }
        }
    } else {
        // compute from zero up to start directly
        bal = mustAmount(curr, 0)
    }
    // Page results
    end := start + lim
    if end > len(list) { end = len(list) }
    items := list[start:end]

    // Build response with running balance
    type item struct { Date time.Time `json:"date"`; EntryID uuid.UUID `json:"entry_id"`; LineID uuid.UUID `json:"line_id"`; Side string `json:"side"`; AmountMinor int64 `json:"amount_minor"`; RunningMinor int64 `json:"running_balance_minor"` }
    resp := struct { UserID uuid.UUID `json:"user_id"`; AccountID uuid.UUID `json:"account_id"`; Currency string `json:"currency"`; Items []item `json:"items"`; NextCursor *string `json:"next_cursor,omitempty"` }{UserID: userID, AccountID: id, Currency: curr}
    for _, rc := range items {
        if rc.side == "debit" { bal, _ = bal.Add(mustAmount(curr, rc.amount)) } else { bal, _ = bal.Sub(mustAmount(curr, rc.amount)) }
        run, _ := bal.MinorUnits()
        resp.Items = append(resp.Items, item{Date: rc.date, EntryID: rc.entryID, LineID: rc.lineID, Side: rc.side, AmountMinor: rc.amount, RunningMinor: run})
    }
    if end < len(list) {
        c := base64.StdEncoding.EncodeToString([]byte(items[len(items)-1].date.Format(time.RFC3339Nano) + "|" + items[len(items)-1].lineID.String()))
        resp.NextCursor = &c
    }
    toJSON(w, http.StatusOK, resp)
}

func mustAmount(curr string, units int64) money.Amount {
    a, _ := money.NewAmountFromMinorUnits(curr, units)
    return a
}
