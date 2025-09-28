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
    "github.com/tinoosan/ledger/internal/errs"
    "errors"
)

// GET /accounts/{id}/balance?user_id=&as_of=
func (s *Server) getAccountBalance(w http.ResponseWriter, r *http.Request) {
    accountIDStr := chi.URLParam(r, "id")
    accountID, err := uuid.Parse(accountIDStr)
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
    if _, err := s.accReader.AccountByID(r.Context(), userID, accountID); err != nil {
        if errors.Is(err, errs.ErrNotFound) { notFound(w) } else { writeErr(w, http.StatusInternalServerError, "failed to load account", "") }
        return
    }
    balance, err := s.svc.AccountBalance(r.Context(), userID, accountID, asOf)
    if err != nil { toJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()}); return }
    balanceMinorUnits, _ := balance.MinorUnits()
    currency := balance.Curr().Code()
    resp := map[string]any{"user_id": userID, "account_id": accountID, "as_of": asOf, "currency": currency, "balance_minor": balanceMinorUnits, "balance": balance.Decimal().String()}
    toJSON(w, http.StatusOK, resp)
}

// GET /accounts/{id}/ledger?user_id=&from=&to=&limit=&cursor=
func (s *Server) getAccountLedger(w http.ResponseWriter, r *http.Request) {
    accountIDStr := chi.URLParam(r, "id")
    accountID, err := uuid.Parse(accountIDStr)
    if err != nil { toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid account id"}); return }
    userID, err := uuid.Parse(r.URL.Query().Get("user_id"))
    if err != nil { toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid user_id"}); return }
    // ensure account exists and owned by user
    if _, err := s.accReader.AccountByID(r.Context(), userID, accountID); err != nil {
        if errors.Is(err, errs.ErrNotFound) { notFound(w) } else { writeErr(w, http.StatusInternalServerError, "failed to load account", "") }
        return
    }
    var from, to *time.Time
    if v := r.URL.Query().Get("from"); v != "" { if t, err := time.Parse(time.RFC3339, v); err == nil { tt := t.UTC(); from = &tt } else { toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid from"}); return } }
    if v := r.URL.Query().Get("to"); v != "" { if t, err := time.Parse(time.RFC3339, v); err == nil { tt := t.UTC(); to = &tt } else { toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid to"}); return } }
    lim := 50
    if v := r.URL.Query().Get("limit"); v != "" { if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 { lim = n } }
    cursor := r.URL.Query().Get("cursor")

    // Build a flat list of lines for the account
    entries, err := s.entryReader.EntriesByUserID(r.Context(), userID)
    if err != nil { toJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load entries"}); return }
    type ledgerRecord struct{ date time.Time; entryID, lineID uuid.UUID; side string; amountMinor int64 }
    ledgerRecords := make([]ledgerRecord, 0, 64)
    var currency string
    for _, e := range entries {
        if from != nil && e.Date.Before(*from) { continue }
        if to != nil && e.Date.After(*to) { continue }
        for lineID, line := range e.Lines.ByID {
            if line.AccountID != accountID { continue }
            amountMinor, _ := line.Amount.MinorUnits()
            side := string(line.Side)
            ledgerRecords = append(ledgerRecords, ledgerRecord{date: e.Date, entryID: e.ID, lineID: lineID, side: side, amountMinor: amountMinor})
            if currency == "" { currency = line.Amount.Curr().Code() }
        }
    }
    sort.Slice(ledgerRecords, func(i, j int) bool {
        if ledgerRecords[i].date.Equal(ledgerRecords[j].date) {
            if ledgerRecords[i].entryID == ledgerRecords[j].entryID { return ledgerRecords[i].lineID.String() < ledgerRecords[j].lineID.String() }
            return ledgerRecords[i].entryID.String() < ledgerRecords[j].entryID.String()
        }
        return ledgerRecords[i].date.Before(ledgerRecords[j].date)
    })
    // Apply cursor
    start := 0
    if cursor != "" {
        if b, err := base64.StdEncoding.DecodeString(cursor); err == nil {
            parts := strings.Split(string(b), "|")
            if len(parts) == 2 {
                if ts, err := time.Parse(time.RFC3339Nano, parts[0]); err == nil {
                    cid, _ := uuid.Parse(parts[1])
                    for i := range ledgerRecords {
                        if ledgerRecords[i].date.After(ts) { break }
                        if ledgerRecords[i].date.Equal(ts) && ledgerRecords[i].lineID == cid { start = i + 1; break }
                    }
                }
            }
        }
    }
    // Running balance from beginning to current page start
    balance, err := s.svc.AccountBalance(r.Context(), userID, accountID, to)
    if err != nil { toJSON(w, http.StatusInternalServerError, errorResponse{Error: "balance error"}); return }
    // But we need balance up to just before page start; recompute if needed
    // Simple approach: recompute from start zero to start index
    if start > 0 {
        // Get currency from bal
        _, _ = balance.MinorUnits()
        // recompute using ledgerRecords[:start]
        for _, record := range ledgerRecords[:start] {
            // Use sign: debit +, credit - (already using minor units)
            if record.side == "debit" { balance, _ = balance.Add(mustAmount(currency, record.amountMinor)) } else { balance, _ = balance.Sub(mustAmount(currency, record.amountMinor)) }
        }
    } else {
        // compute from zero up to start directly
        balance = mustAmount(currency, 0)
    }
    // Page results
    end := start + lim
    if end > len(ledgerRecords) { end = len(ledgerRecords) }
    pageRecords := ledgerRecords[start:end]

    // Build response with running balance
    type item struct { Date time.Time `json:"date"`; EntryID uuid.UUID `json:"entry_id"`; LineID uuid.UUID `json:"line_id"`; Side string `json:"side"`; AmountMinor int64 `json:"amount_minor"`; Amount string `json:"amount"`; RunningMinor int64 `json:"running_balance_minor"`; Running string `json:"running_balance"` }
    resp := struct { UserID uuid.UUID `json:"user_id"`; AccountID uuid.UUID `json:"account_id"`; Currency string `json:"currency"`; Items []item `json:"items"`; NextCursor *string `json:"next_cursor,omitempty"` }{UserID: userID, AccountID: accountID, Currency: currency}
    for _, record := range pageRecords {
        amt := mustAmount(currency, record.amountMinor)
        if record.side == "debit" { balance, _ = balance.Add(amt) } else { balance, _ = balance.Sub(amt) }
        runningMinor, _ := balance.MinorUnits()
        resp.Items = append(resp.Items, item{Date: record.date, EntryID: record.entryID, LineID: record.lineID, Side: record.side, AmountMinor: record.amountMinor, Amount: amt.Decimal().String(), RunningMinor: runningMinor, Running: balance.Decimal().String()})
    }
    if end < len(ledgerRecords) {
        c := base64.StdEncoding.EncodeToString([]byte(pageRecords[len(pageRecords)-1].date.Format(time.RFC3339Nano) + "|" + pageRecords[len(pageRecords)-1].lineID.String()))
        resp.NextCursor = &c
    }
    toJSON(w, http.StatusOK, resp)
}

func mustAmount(curr string, units int64) money.Amount {
    a, _ := money.NewAmountFromMinorUnits(curr, units)
    return a
}
