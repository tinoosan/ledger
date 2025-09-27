package httpapi

import (
    "context"
    "encoding/json"
    "net/http"

    "github.com/tinoosan/ledger/internal/service/journal"
)

type ctxKey string

const ctxKeyPostEntry ctxKey = "validatedPostEntry"

// validatePostEntry ensures the POST /entries request adheres to business invariants
// and stores the validated request struct in the request context for the handler to use.
func (s *Server) validatePostEntry() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            var req postEntryRequest
            dec := json.NewDecoder(r.Body)
            dec.DisallowUnknownFields()
            if err := dec.Decode(&req); err != nil {
                toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON: "+err.Error()})
                return
            }

            // Convert to service EntryInput and validate via service layer
            in := toEntryInput(req)
            if err := s.svc.ValidateEntryInput(r.Context(), in); err != nil {
                toJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
                return
            }

            ctx := context.WithValue(r.Context(), ctxKeyPostEntry, in)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

func writeError(w http.ResponseWriter, code int, msg string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    _ = json.NewEncoder(w).Encode(errorResponse{Error: msg})
}


func toEntryInput(req postEntryRequest) journal.EntryInput {
    lines := make([]journal.LineInput, 0, len(req.Lines))
    for _, ln := range req.Lines {
        lines = append(lines, journal.LineInput{
            AccountID:   ln.AccountID,
            Side:        ln.Side,
            AmountMinor: ln.AmountMinor,
        })
    }
    return journal.EntryInput{
        UserID:        req.UserID,
        Date:          req.Date,
        Currency:      req.Currency,
        Memo:          req.Memo,
        Category:      req.Category,
        ClientEntryID: req.ClientEntryID,
        Lines:         lines,
    }
}
