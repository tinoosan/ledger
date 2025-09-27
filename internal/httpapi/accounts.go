package httpapi

import (
    "net/http"

    "github.com/tinoosan/ledger/internal/service/account"
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
        toJSON(w, http.StatusInternalServerError, errorResponse{Error: "could not create account"})
        return
    }
    resp := accountResponse{ID: acc.ID, UserID: acc.UserID, Name: acc.Name, Currency: acc.Currency, Type: acc.Type}
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
        out = append(out, accountResponse{ID: a.ID, UserID: a.UserID, Name: a.Name, Currency: a.Currency, Type: a.Type})
    }
    toJSON(w, http.StatusOK, out)
}
