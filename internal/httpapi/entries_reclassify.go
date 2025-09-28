package httpapi

import (
    "encoding/json"
    "net/http"
    "time"
    "strings"

    "github.com/google/uuid"
    "github.com/govalues/money"
    "github.com/tinoosan/ledger/internal/ledger"
)

// POST /entries/reclassify
// Body: { user_id, entry_id, date?, memo?, category?, lines }
func (s *Server) reclassifyEntry(w http.ResponseWriter, r *http.Request) {
    var body struct {
        UserID   uuid.UUID       `json:"user_id"`
        EntryID  uuid.UUID       `json:"entry_id"`
        Date     *time.Time      `json:"date,omitempty"`
        Memo     *string         `json:"memo,omitempty"`
        Category *ledger.Category `json:"category,omitempty"`
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
    saved, err := s.svc.Reclassify(r.Context(), body.UserID, body.EntryID, when, memo, cat, domLines)
    if err != nil {
        // Map validation to 422 using same rules as validatePostEntry
        msg := err.Error()
        code := "validation_error"
        switch {
        case msg == "at least 2 lines":
            code = "too_few_lines"
        case strings.Contains(msg, "amount must be > 0"):
            code = "invalid_amount"
        case strings.Contains(msg, "currency mismatch"):
            code = "mixed_currency"
        case msg == "sum(debits) must equal sum(credits)":
            code = "unbalanced_entry"
        case strings.Contains(msg, "not found"):
            toJSON(w, http.StatusNotFound, errorResponse{Error: "not_found", Code: "not_found"})
            return
        }
        toJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: msg, Code: code})
        return
    }
    toJSON(w, http.StatusCreated, toEntryResponse(saved))
}

// no additional helpers
