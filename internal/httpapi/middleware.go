// Package httpapi contains HTTP handlers and middleware.
package httpapi

import (
    "context"
    "encoding/json"
    "net/http"
    "time"
    "strings"

    "github.com/tinoosan/ledger/internal/service/journal"
    "github.com/google/uuid"
    "github.com/tinoosan/ledger/internal/service/account"
)

type ctxKey string

const ctxKeyPostEntry ctxKey = "validatedPostEntry"
const ctxKeyListEntries ctxKey = "validatedListEntries"
const ctxKeyPostAccount ctxKey = "validatedPostAccount"
const ctxKeyListAccounts ctxKey = "validatedListAccounts"
const ctxKeyReverseEntry ctxKey = "validatedReverseEntry"
const ctxKeyTrialBalance ctxKey = "validatedTrialBalance"

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
                // Map known validation messages to 422 with codes
                code := "validation_error"
                msg := err.Error()
                switch {
                case msg == "at least 2 lines":
                    code = "too_few_lines"
                case strings.Contains(msg, "amount must be > 0"):
                    code = "invalid_amount"
                case strings.Contains(msg, "currency mismatch"):
                    code = "mixed_currency"
                case msg == "sum(debits) must equal sum(credits)":
                    code = "unbalanced_entry"
                }
                toJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: msg, Code: code})
                return
            }

            ctx := context.WithValue(r.Context(), ctxKeyPostEntry, in)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

// validateListEntries parses and validates query params for GET /entries.
func (s *Server) validateListEntries() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            q := r.URL.Query()
            raw := q.Get("user_id")
            if raw == "" {
                toJSON(w, http.StatusBadRequest, errorResponse{Error: "user_id is required"})
                return
            }
            uid, err := uuid.Parse(raw)
            if err != nil {
                toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid user_id"})
                return
            }
            ctx := context.WithValue(r.Context(), ctxKeyListEntries, listEntriesQuery{UserID: uid})
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

// validateReverseEntry parses POST /entries/reverse body
func (s *Server) validateReverseEntry() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            var req reverseEntryRequest
            dec := json.NewDecoder(r.Body)
            dec.DisallowUnknownFields()
            if err := dec.Decode(&req); err != nil {
                toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON: "+err.Error()})
                return
            }
            if req.UserID == uuid.Nil || req.EntryID == uuid.Nil {
                toJSON(w, http.StatusBadRequest, errorResponse{Error: "user_id and entry_id are required"})
                return
            }
            ctx := context.WithValue(r.Context(), ctxKeyReverseEntry, req)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

// validateTrialBalance parses GET /trial-balance query
func (s *Server) validateTrialBalance() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            q := r.URL.Query()
            userStr := q.Get("user_id")
            if userStr == "" {
                toJSON(w, http.StatusBadRequest, errorResponse{Error: "user_id is required"})
                return
            }
            uid, err := uuid.Parse(userStr)
            if err != nil {
                toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid user_id"})
                return
            }
            var asOf *time.Time
            if raw := q.Get("as_of"); raw != "" {
                if t, err := time.Parse(time.RFC3339, raw); err == nil {
                    tt := t.UTC()
                    asOf = &tt
                } else {
                    toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid as_of"})
                    return
                }
            }
            ctx := context.WithValue(r.Context(), ctxKeyTrialBalance, trialBalanceQuery{UserID: uid, AsOf: asOf})
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

// validatePostAccount parses and validates POST /accounts body and stores CreateInput.
func (s *Server) validatePostAccount() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            var req postAccountRequest
            dec := json.NewDecoder(r.Body)
            dec.DisallowUnknownFields()
            if err := dec.Decode(&req); err != nil {
                toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON: "+err.Error()})
                return
            }
            in := toAccountCreateInput(req)
            if err := s.accountSvc.ValidateCreateInput(in); err != nil {
                toJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
                return
            }
            ctx := context.WithValue(r.Context(), ctxKeyPostAccount, in)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

// validateListAccounts validates query params for GET /accounts.
func (s *Server) validateListAccounts() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            raw := r.URL.Query().Get("user_id")
            if raw == "" {
                toJSON(w, http.StatusBadRequest, errorResponse{Error: "user_id is required"})
                return
            }
            uid, err := uuid.Parse(raw)
            if err != nil {
                toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid user_id"})
                return
            }
            q := listAccountsQuery{UserID: uid}
            if m := r.URL.Query().Get("method"); m != "" { q.Method = m }
            if v := r.URL.Query().Get("vendor"); v != "" { q.Vendor = v }
            if t := r.URL.Query().Get("type"); t != "" { q.Type = t }
            ctx := context.WithValue(r.Context(), ctxKeyListAccounts, q)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

func toAccountCreateInput(req postAccountRequest) account.CreateInput {
    return account.CreateInput{
        UserID:   req.UserID,
        Name:     req.Name,
        Currency: req.Currency,
        Type:     req.Type,
        Method:   req.Method,
        Vendor:   req.Vendor,
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
