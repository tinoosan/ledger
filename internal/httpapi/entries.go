package httpapi

import (
    "encoding/json"
    "net/http"

    "github.com/tinoosan/ledger/internal/ledger"
    "github.com/tinoosan/ledger/internal/service/journal"
)

func (s *Server) postEntry(w http.ResponseWriter, r *http.Request) {
    // Request has already been validated and is present in context
    v := r.Context().Value(ctxKeyPostEntry)
    in, ok := v.(journal.EntryInput)
    if !ok {
        writeError(w, http.StatusInternalServerError, "validated request missing")
        return
    }

    saved, err := s.svc.CreateEntry(r.Context(), in)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "could not persist entry")
        return
    }

    // Format response
    resp := toEntryResponse(saved)
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated)
    _ = json.NewEncoder(w).Encode(resp)
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
