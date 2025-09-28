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
    ctxVal := r.Context().Value(ctxKeyPostEntry)
    entry, ok := ctxVal.(ledger.JournalEntry)
    if !ok {
        toJSON(w, http.StatusInternalServerError, errorResponse{Error: "validated request missing"})
        return
    }
    // Optional idempotency key header
    idemKey := r.Header.Get("Idempotency-Key")
    if idemKey != "" {
        if existing, ok, err := s.idemStore.GetEntryByIdempotencyKey(r.Context(), entry.UserID, idemKey); err == nil && ok {
            toJSON(w, http.StatusOK, toEntryResponse(existing))
            return
        }
    }

    saved, err := s.svc.CreateEntry(r.Context(), entry)
    if err != nil {
        toJSON(w, http.StatusInternalServerError, errorResponse{Error: "could not persist entry"})
        return
    }
    if idemKey != "" {
        _ = s.idemStore.SaveIdempotencyKey(r.Context(), saved.UserID, idemKey, saved.ID)
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
    ctxVal := r.Context().Value(ctxKeyTrialBalance)
    query, ok := ctxVal.(trialBalanceQuery)
    if !ok {
        toJSON(w, http.StatusInternalServerError, errorResponse{Error: "validated query missing"})
        return
    }
    // Compute TB map of accountID -> net amount
    netAmountsByAccount, err := s.svc.TrialBalance(r.Context(), query.UserID, query.AsOf)
    if err != nil {
        toJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
        return
    }
    // Load account details
    accountIDs := make([]uuid.UUID, 0, len(netAmountsByAccount))
    for accountID := range netAmountsByAccount { accountIDs = append(accountIDs, accountID) }
    accountsByID, err := s.accReader.FetchAccounts(r.Context(), query.UserID, accountIDs)
    if err != nil {
        toJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load accounts"})
        return
    }
    // Build response
    response := trialBalanceResponse{UserID: query.UserID, AsOf: query.AsOf}
    response.Accounts = make([]trialBalanceAccount, 0, len(netAmountsByAccount))
    for accountID, amount := range netAmountsByAccount {
        account := accountsByID[accountID]
        units, _ := amount.MinorUnits()
        var debit, credit int64
        if units >= 0 { debit, credit = units, 0 } else { debit, credit = 0, -units }
        // decimals using money.Amount.Decimal()
        dstr, cstr := "0", "0"
        if debit > 0 {
            if a, err := money.NewAmountFromMinorUnits(account.Currency, debit); err == nil {
                dstr = a.Decimal().String()
            }
        }
        if credit > 0 {
            if a, err := money.NewAmountFromMinorUnits(account.Currency, credit); err == nil {
                cstr = a.Decimal().String()
            }
        }
        response.Accounts = append(response.Accounts, trialBalanceAccount{
            AccountID: accountID,
            Name: account.Name,
            Path: account.Path(),
            Currency: account.Currency,
            DebitMinor: debit,
            CreditMinor: credit,
            Debit: dstr,
            Credit: cstr,
            Type: account.Type,
        })
    }
    toJSON(w, http.StatusOK, response)
}

// listEntries handles GET /entries
func (s *Server) listEntries(w http.ResponseWriter, r *http.Request) {
    ctxVal := r.Context().Value(ctxKeyListEntries)
    query, ok := ctxVal.(listEntriesQuery)
    if !ok {
        toJSON(w, http.StatusInternalServerError, errorResponse{Error: "validated query missing"})
        return
    }
    entries, err := s.svc.ListEntries(r.Context(), query.UserID)
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
    window := make([]ledger.JournalEntry, 0, len(entries))
    for _, entry := range entries {
        if from != nil && entry.Date.Before(*from) { continue }
        if to != nil && entry.Date.After(*to) { continue }
        window = append(window, entry)
    }
    // start index from cursor
    start := 0
    if cursor != "" {
        if b, err := base64.StdEncoding.DecodeString(cursor); err == nil {
            parts := strings.Split(string(b), "|")
            if len(parts) == 2 {
                if ts, err := time.Parse(time.RFC3339Nano, parts[0]); err == nil {
                    cid, _ := uuid.Parse(parts[1])
                    for i := range window {
                        if window[i].Date.After(ts) { break }
                        if window[i].Date.Equal(ts) && window[i].ID == cid { start = i + 1; break }
                    }
                }
            }
        }
    }
    end := start + lim
    if end > len(window) { end = len(window) }
    page := window[start:end]
    response := listEntriesResponse{Items: make([]entryResponse, 0, len(page))}
    for _, entry := range page { response.Items = append(response.Items, toEntryResponse(entry)) }
    if end < len(window) {
        c := base64.StdEncoding.EncodeToString([]byte(page[len(page)-1].Date.Format(time.RFC3339Nano) + "|" + page[len(page)-1].ID.String()))
        response.NextCursor = &c
    }
    toJSON(w, http.StatusOK, response)
}

func toEntryResponse(entry ledger.JournalEntry) entryResponse {
    lines := make([]lineResponse, 0, len(entry.Lines.ByID))
    for lineID, line := range entry.Lines.ByID {
        minorUnits, _ := line.Amount.MinorUnits()
        lines = append(lines, lineResponse{
            ID:          lineID,
            AccountID:   line.AccountID,
            Side:        line.Side,
            AmountMinor: minorUnits,
            Amount:      line.Amount.Decimal().String(),
        })
    }
    return entryResponse{
        ID:            entry.ID,
        UserID:        entry.UserID,
        Date:          entry.Date,
        Currency:      entry.Currency,
        Memo:          entry.Memo,
        Category:      entry.Category,
        Metadata:      entry.Metadata,
        Lines:         lines,
    }
}
