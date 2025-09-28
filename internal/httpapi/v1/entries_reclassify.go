package v1

import (
    "encoding/json"
    "net/http"
    "time"

    "github.com/google/uuid"
    "github.com/govalues/money"
    "github.com/tinoosan/ledger/internal/ledger"
    "github.com/tinoosan/ledger/internal/errs"
    "errors"
)

// POST /entries/reclassify
// Body: { user_id, entry_id, date?, memo?, category?, lines }
func (s *Server) reclassifyEntry(w http.ResponseWriter, r *http.Request) {
    if !requireJSON(w, r) { return }
    var body struct {
        UserID   uuid.UUID       `json:"user_id"`
        EntryID  uuid.UUID       `json:"entry_id"`
        Date     *time.Time      `json:"date,omitempty"`
        Memo     *string         `json:"memo,omitempty"`
        Category *ledger.Category `json:"category,omitempty"`
        Metadata map[string]string `json:"metadata,omitempty"`
        Lines    []postEntryLine `json:"lines"`
    }
    dec := json.NewDecoder(r.Body)
    dec.DisallowUnknownFields()
    if err := dec.Decode(&body); err != nil {
        toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON: "+err.Error()})
        return
    }
    if body.UserID == uuid.Nil || body.EntryID == uuid.Nil {
        toJSON(w, http.StatusBadRequest, errorResponse{Error: "user_id and entry_id are required"})
        return
    }
    when := time.Now().UTC()
    if body.Date != nil { when = body.Date.UTC() }
    memo := ""
    if body.Memo != nil { memo = *body.Memo }
    var cat ledger.Category
    if body.Category != nil { cat = *body.Category }
    // map lines to domain lines
    domLines := make([]ledger.JournalLine, 0, len(body.Lines))
    for _, ln := range body.Lines {
        amt, _ := money.NewAmountFromMinorUnits("USD", ln.AmountMinor)
        // currency will be validated against original entry's currency; amounts are attached per line
        domLines = append(domLines, ledger.JournalLine{AccountID: ln.AccountID, Side: ln.Side, Amount: amt})
    }
    // call service
    saved, err := s.svc.Reclassify(r.Context(), body.UserID, body.EntryID, when, memo, cat, domLines, body.Metadata)
    if err != nil {
        // 404 detection
        if errors.Is(err, errs.ErrNotFound) { notFound(w); return }
        if errors.Is(err, errs.ErrAlreadyReversed) { unprocessable(w, "already_reversed", "already_reversed"); return }
        if errors.Is(err, errs.ErrForbidden) { forbidden(w, "forbidden"); return }
        if errors.Is(err, errs.ErrInvalid) { badRequest(w, "invalid"); return }
        // Validation mapping
        code, msg := mapValidationError(err)
        if code != "" { unprocessable(w, msg, code); return }
        badRequest(w, err.Error())
        return
    }
    toJSON(w, http.StatusCreated, toEntryResponse(saved))
}

// no additional helpers
