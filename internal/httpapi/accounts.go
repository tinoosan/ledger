// Account handlers: CRUD-lite (create, list, update, deactivate).
package httpapi

import (
    "net/http"

    "github.com/tinoosan/ledger/internal/service/account"
    "errors"
    chi "github.com/go-chi/chi/v5"
    "github.com/google/uuid"
    "github.com/tinoosan/ledger/internal/ledger"
    "github.com/tinoosan/ledger/internal/errs"
    "strings"
)

func (s *Server) postAccount(w http.ResponseWriter, r *http.Request) {
    ctxVal := r.Context().Value(ctxKeyPostAccount)
    accountInput, ok := ctxVal.(ledger.Account)
    if !ok {
        toJSON(w, http.StatusInternalServerError, errorResponse{Error: "validated request missing"})
        return
    }
    createdAccount, err := s.accountSvc.Create(r.Context(), accountInput)
    if err != nil {
        if errors.Is(err, account.ErrPathExists) { conflict(w, err.Error()); return }
        if errors.Is(err, errs.ErrInvalid) { badRequest(w, "invalid"); return }
        writeErr(w, http.StatusInternalServerError, "could not create account", "")
        return
    }
    resp := accountResponse{ID: createdAccount.ID, UserID: createdAccount.UserID, Name: createdAccount.Name, Currency: createdAccount.Currency, Type: createdAccount.Type, Method: createdAccount.Method, Vendor: createdAccount.Vendor, Path: createdAccount.Path(), Metadata: createdAccount.Metadata}
    toJSON(w, http.StatusCreated, resp)
}

func (s *Server) listAccounts(w http.ResponseWriter, r *http.Request) {
    ctxVal := r.Context().Value(ctxKeyListAccounts)
    query, ok := ctxVal.(listAccountsQuery)
    if !ok {
        toJSON(w, http.StatusInternalServerError, errorResponse{Error: "validated query missing"})
        return
    }
    accounts, err := s.accountSvc.List(r.Context(), query.UserID)
    if err != nil {
        toJSON(w, http.StatusInternalServerError, errorResponse{Error: "could not fetch accounts"})
        return
    }
    responses := make([]accountResponse, 0, len(accounts))
    for _, account := range accounts {
        if query.Method != "" && !equalsFold(account.Method, query.Method) { continue }
        if query.Vendor != "" && !equalsFold(account.Vendor, query.Vendor) { continue }
        if query.Type   != "" && !equalsFold(string(account.Type), query.Type) { continue }
        responses = append(responses, accountResponse{ID: account.ID, UserID: account.UserID, Name: account.Name, Currency: account.Currency, Type: account.Type, Method: account.Method, Vendor: account.Vendor, Path: account.Path(), Metadata: account.Metadata})
    }
    toJSON(w, http.StatusOK, responses)
}

func equalsFold(a, b string) bool {
    if len(a) != len(b) { // quick path; ToLower alloc only if needed
        return strings.EqualFold(a, b)
    }
    for i := 0; i < len(a); i++ {
        ca, cb := a[i], b[i]
        if ca == cb { continue }
        if 'A' <= ca && ca <= 'Z' { ca += 'a' - 'A' }
        if 'A' <= cb && cb <= 'Z' { cb += 'a' - 'A' }
        if ca != cb { return false }
    }
    return true
}

// getAccount handles GET /accounts/{id}?user_id=...
func (s *Server) getAccount(w http.ResponseWriter, r *http.Request) {
    idStr := chi.URLParam(r, "id")
    id, err := uuid.Parse(idStr)
    if err != nil {
        toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid account id"})
        return
    }
    userIDStr := r.URL.Query().Get("user_id")
    if userIDStr == "" {
        toJSON(w, http.StatusBadRequest, errorResponse{Error: "user_id is required"})
        return
    }
    userID, err := uuid.Parse(userIDStr)
    if err != nil {
        toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid user_id"})
        return
    }
    acc, err := s.accReader.GetAccount(r.Context(), userID, id)
    if err != nil {
        if errors.Is(err, errs.ErrNotFound) { notFound(w) } else { writeErr(w, http.StatusInternalServerError, "failed to load account", "") }
        return
    }
    resp := accountResponse{ID: acc.ID, UserID: acc.UserID, Name: acc.Name, Currency: acc.Currency, Type: acc.Type, Method: acc.Method, Vendor: acc.Vendor, Path: acc.Path(), Metadata: acc.Metadata}
    toJSON(w, http.StatusOK, resp)
}
