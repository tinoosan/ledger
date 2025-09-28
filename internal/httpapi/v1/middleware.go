// Package httpapi contains HTTP handlers and middleware.
package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/govalues/money"
	"github.com/tinoosan/ledger/internal/ledger"
	"github.com/tinoosan/ledger/internal/meta"
	"strings"
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
			// Body size already limited by global middleware
			if !requireJSON(w, r) {
				return
			}
			var req postEntryRequest
			dec := json.NewDecoder(r.Body)
			dec.DisallowUnknownFields()
			if err := dec.Decode(&req); err != nil {
				toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON: " + err.Error()})
				return
			}

			// Validate metadata if provided
			if req.Metadata != nil {
				if err := meta.New(req.Metadata).Validate(); err != nil {
					unprocessable(w, "validation_error", "validation_error")
					return
				}
			}
			// Convert to service EntryInput and validate via service layer
			e := toEntryDomain(req)
			if err := s.svc.ValidateEntry(r.Context(), e); err != nil {
				code, msg := mapValidationError(err)
				unprocessable(w, msg, code)
				return
			}

			ctx := context.WithValue(r.Context(), ctxKeyPostEntry, e)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// validateListEntries parses and validates query params for GET /entries.
func (s *Server) validateListEntries() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			vals := r.URL.Query()
			raw := vals.Get("user_id")
			if raw == "" {
				toJSON(w, http.StatusBadRequest, errorResponse{Error: "user_id is required"})
				return
			}
			userID, err := uuid.Parse(raw)
			if err != nil {
				toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid user_id"})
				return
			}
			leq := listEntriesQuery{UserID: userID}
			if c := r.URL.Query().Get("currency"); c != "" {
				leq.Currency = c
			}
			if m := r.URL.Query().Get("memo"); m != "" {
				leq.Memo = m
			}
			if cat := r.URL.Query().Get("category"); cat != "" {
				leq.Category = cat
			}
			if ir := r.URL.Query().Get("is_reversed"); ir != "" {
				if ir == "true" || ir == "1" {
					b := true
					leq.IsReversed = &b
				}
				if ir == "false" || ir == "0" {
					b := false
					leq.IsReversed = &b
				}
			}
			ctx := context.WithValue(r.Context(), ctxKeyListEntries, leq)
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
				toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON: " + err.Error()})
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
			userID, err := uuid.Parse(userStr)
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
			ctx := context.WithValue(r.Context(), ctxKeyTrialBalance, trialBalanceQuery{UserID: userID, AsOf: asOf})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// validatePostAccount parses and validates POST /accounts body and stores CreateInput.
func (s *Server) validatePostAccount() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Body size already limited by global middleware
			if !requireJSON(w, r) {
				return
			}
			var req postAccountRequest
			dec := json.NewDecoder(r.Body)
			dec.DisallowUnknownFields()
			if err := dec.Decode(&req); err != nil {
				toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON: " + err.Error()})
				return
			}
			if req.Metadata != nil {
				if err := meta.New(req.Metadata).Validate(); err != nil {
					unprocessable(w, "validation_error", "validation_error")
					return
				}
			}
			in := toAccountDomain(req)
			if err := s.accountSvc.ValidateCreate(in); err != nil {
				toJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
				return
			}
			ctx := context.WithValue(r.Context(), ctxKeyPostAccount, in)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// limitRequestBody caps the request body size using http.MaxBytesReader.
func limitRequestBody(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil && maxBytes > 0 {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
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
			userID, err := uuid.Parse(raw)
			if err != nil {
				toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid user_id"})
				return
			}
			query := listAccountsQuery{UserID: userID}
			if n := r.URL.Query().Get("name"); n != "" {
				query.Name = n
			}
			if c := r.URL.Query().Get("currency"); c != "" {
				query.Currency = c
			}
			if g := r.URL.Query().Get("group"); g != "" {
				query.Group = g
			}
			if v := r.URL.Query().Get("vendor"); v != "" {
				query.Vendor = v
			}
			if t := r.URL.Query().Get("type"); t != "" {
				query.Type = t
			}
			if sys := r.URL.Query().Get("system"); sys != "" {
				if sys == "true" || sys == "1" {
					b := true
					query.System = &b
				}
				if sys == "false" || sys == "0" {
					b := false
					query.System = &b
				}
			}
			if act := r.URL.Query().Get("active"); act != "" {
				if act == "true" || act == "1" {
					b := true
					query.Active = &b
				}
				if act == "false" || act == "0" {
					b := false
					query.Active = &b
				}
			}
			ctx := context.WithValue(r.Context(), ctxKeyListAccounts, query)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func toAccountDomain(req postAccountRequest) ledger.Account {
	return ledger.Account{
		UserID:   req.UserID,
		Name:     req.Name,
		Currency: req.Currency,
		Type:     req.Type,
		Group:    req.Group,
		Vendor:   req.Vendor,
		System:   req.System,
		Metadata: meta.New(req.Metadata),
	}
}

func toEntryDomain(req postEntryRequest) ledger.JournalEntry {
	// Construct domain JournalEntry with money.Amount lines
	lines := ledger.JournalLines{ByID: make(map[uuid.UUID]*ledger.JournalLine, len(req.Lines))}
	for _, line := range req.Lines {
		amt, _ := money.NewAmountFromMinorUnits(strings.ToUpper(req.Currency), line.AmountMinor)
		id := uuid.New()
		lines.ByID[id] = &ledger.JournalLine{ID: id, AccountID: line.AccountID, Side: line.Side, Amount: amt}
	}
	return ledger.JournalEntry{
		UserID:   req.UserID,
		Date:     req.Date,
		Currency: strings.ToUpper(req.Currency),
		Memo:     req.Memo,
		Category: req.Category,
		Metadata: meta.New(req.Metadata),
		Lines:    lines,
	}
}
