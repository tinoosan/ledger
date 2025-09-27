// Account handlers: CRUD-lite (create, list, update, deactivate).
package httpapi

import (
    "net/http"

    "github.com/tinoosan/ledger/internal/service/account"
    "errors"
    "strings"
)

func (s *Server) postAccount(w http.ResponseWriter, r *http.Request) {
    v := r.Context().Value(ctxKeyPostAccount)
    in, ok := v.(account.CreateInput)
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
    resp := accountResponse{ID: acc.ID, UserID: acc.UserID, Name: acc.Name, Currency: acc.Currency, Type: acc.Type, Method: acc.Method, Vendor: acc.Vendor, Path: acc.Path()}
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
        out = append(out, accountResponse{ID: a.ID, UserID: a.UserID, Name: a.Name, Currency: a.Currency, Type: a.Type, Method: a.Method, Vendor: a.Vendor, Path: a.Path()})
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
