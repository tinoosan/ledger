package httpapi

import (
    "net/http"

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
