package v1

import (
    "encoding/json"
    "net/http"
    "github.com/tinoosan/ledger/internal/ledger"
    "github.com/tinoosan/ledger/internal/meta"
    "time"
)

// postEntriesBatch handles POST /v1/entries:batch (and /v1/entries/batch)
// Atomic: all-or-nothing. Returns 201 with {entries:[...]} or 422 with {errors:[...]}
func (s *Server) postEntriesBatch(w http.ResponseWriter, r *http.Request) {
    if !requireJSON(w, r) { return }
    // Require Idempotency-Key for batch endpoints
    if r.Header.Get("Idempotency-Key") == "" { writeErr(w, http.StatusBadRequest, "idempotency_required", "idempotency_required"); return }
    var req struct{ Entries []postEntryRequest `json:"entries"` }
    dec := json.NewDecoder(r.Body)
    dec.DisallowUnknownFields()
    if err := dec.Decode(&req); err != nil { toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON: "+err.Error()}); return }
    if len(req.Entries) == 0 { toJSON(w, http.StatusBadRequest, errorResponse{Error: "entries is required"}); return }
    if len(req.Entries) > 500 { unprocessable(w, "too_many_items", "too_many_items"); return }
    // Idempotency for batch (optional)
    if key := r.Header.Get("Idempotency-Key"); key != "" {
        type normEntry struct{
            UserID string `json:"user_id"`; Date string `json:"date"`; Currency string `json:"currency"`; Memo string `json:"memo"`; Category string `json:"category"`; Metadata meta.Metadata `json:"metadata,omitempty"`; Lines []postEntryLine `json:"lines"`
        }
        type normReq struct{ Entries []normEntry `json:"entries"` }
        n := normReq{Entries: make([]normEntry, 0, len(req.Entries))}
        for _, e := range req.Entries {
            n.Entries = append(n.Entries, normEntry{UserID: e.UserID.String(), Date: e.Date.Format(time.RFC3339Nano), Currency: e.Currency, Memo: e.Memo, Category: string(e.Category), Metadata: meta.New(e.Metadata), Lines: e.Lines})
        }
        nb, _ := json.Marshal(n)
        h := hashBytes(nb)
        s.batchIdemMu.RLock()
        if prev, ok := s.batchIdem[key]; ok {
            if prev.BodyHash == h { s.batchIdemMu.RUnlock(); w.WriteHeader(prev.Status); _, _ = w.Write(prev.Payload); return }
            s.batchIdemMu.RUnlock(); conflict(w, "idempotency_mismatch"); return
        }
        s.batchIdemMu.RUnlock()
        rw := &captureWriter{ResponseWriter: w}
        drafts := make([]ledger.JournalEntry, 0, len(req.Entries))
        for _, e := range req.Entries { drafts = append(drafts, toEntryDomain(e)) }
        created, errsList, err := s.svc.CreateEntriesBatch(r.Context(), drafts)
        if err != nil { writeErr(rw, http.StatusBadRequest, err.Error(), ""); s.storeBatch(key, h, rw); return }
        if len(errsList) > 0 {
            type item struct{ Index int `json:"index"`; Code string `json:"code"`; Error string `json:"error"` }
            out := struct{ Errors []item `json:"errors"` }{Errors: make([]item, 0, len(errsList))}
            for _, e := range errsList { out.Errors = append(out.Errors, item{Index: e.Index, Code: e.Code, Error: e.Err.Error()}) }
            toJSON(rw, http.StatusUnprocessableEntity, out); s.storeBatch(key, h, rw); return
        }
        resp := struct{ Entries []entryResponse `json:"entries"` }{Entries: make([]entryResponse, 0, len(created))}
        for _, e := range created { resp.Entries = append(resp.Entries, toEntryResponse(e)) }
        toJSON(rw, http.StatusCreated, resp); s.storeBatch(key, h, rw); return
    }

    // Should not reach here; enforced above
}
