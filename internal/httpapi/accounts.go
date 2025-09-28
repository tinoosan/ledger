// Account handlers: CRUD-lite (create, list, update, deactivate).
package httpapi

import (
    "net/http"

    "github.com/tinoosan/ledger/internal/service/account"
    "errors"
    "strings"
    chi "github.com/go-chi/chi/v5"
    "github.com/google/uuid"
    "github.com/tinoosan/ledger/internal/ledger"
)

func (s *Server) postAccount(w http.ResponseWriter, r *http.Request) {
    v := r.Context().Value(ctxKeyPostAccount)
    in, ok := v.(ledger.Account)
    if !ok {
        toJSON(w, http.StatusInternalServerError, errorResponse{Error: "validated request missing"})
        return
    }
    acc, err := s.accountSvc.Create(r.Context(), in)
    if err != nil {
        if errors.Is(err, account.ErrPathExists) {
            toJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
            return
        }
        toJSON(w, http.StatusInternalServerError, errorResponse{Error: "could not create account"})
        return
    }
    resp := accountResponse{ID: acc.ID, UserID: acc.UserID, Name: acc.Name, Currency: acc.Currency, Type: acc.Type, Method: acc.Method, Vendor: acc.Vendor, Path: acc.Path(), Metadata: acc.Metadata}
    toJSON(w, http.StatusCreated, resp)
}

func (s *Server) listAccounts(w http.ResponseWriter, r *http.Request) {
    v := r.Context().Value(ctxKeyListAccounts)
    q, ok := v.(listAccountsQuery)
    if !ok {
        toJSON(w, http.StatusInternalServerError, errorResponse{Error: "validated query missing"})
        return
    }
    accs, err := s.accountSvc.List(r.Context(), q.UserID)
    if err != nil {
        toJSON(w, http.StatusInternalServerError, errorResponse{Error: "could not fetch accounts"})
        return
    }
    out := make([]accountResponse, 0, len(accs))
    for _, a := range accs {
        if q.Method != "" && !equalsFold(a.Method, q.Method) { continue }
        if q.Vendor != "" && !equalsFold(a.Vendor, q.Vendor) { continue }
        if q.Type   != "" && !equalsFold(string(a.Type), q.Type) { continue }
        out = append(out, accountResponse{ID: a.ID, UserID: a.UserID, Name: a.Name, Currency: a.Currency, Type: a.Type, Method: a.Method, Vendor: a.Vendor, Path: a.Path(), Metadata: a.Metadata})
    }
    toJSON(w, http.StatusOK, out)
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
    acc, err := s.repo.AccountByID(r.Context(), userID, id)
    if err != nil {
        toJSON(w, http.StatusNotFound, errorResponse{Error: "not_found", Code: "not_found"})
        return
    }
    resp := accountResponse{ID: acc.ID, UserID: acc.UserID, Name: acc.Name, Currency: acc.Currency, Type: acc.Type, Method: acc.Method, Vendor: acc.Vendor, Path: acc.Path(), Metadata: acc.Metadata}
    toJSON(w, http.StatusOK, resp)
}
