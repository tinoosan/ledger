package v1

import (
    "encoding/json"
    "net/http"
    "errors"

    "github.com/tinoosan/ledger/internal/service/account"
    "github.com/tinoosan/ledger/internal/errs"
)

// postAccountsBatch handles POST /accounts/batch
// Accepts an array of account requests and attempts to create each, returning per-item results.
func (s *Server) postAccountsBatch(w http.ResponseWriter, r *http.Request) {
    var reqs []postAccountRequest
    dec := json.NewDecoder(r.Body)
    dec.DisallowUnknownFields()
    if err := dec.Decode(&reqs); err != nil {
        toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON: "+err.Error()})
        return
    }
    if len(reqs) == 0 {
        toJSON(w, http.StatusBadRequest, errorResponse{Error: "empty payload"})
        return
    }
    type item struct {
        Index   int               `json:"index"`
        Account *accountResponse  `json:"account,omitempty"`
        Error   string            `json:"error,omitempty"`
        Code    string            `json:"code,omitempty"`
    }
    results := make([]item, 0, len(reqs))
    for i, rreq := range reqs {
        acc := toAccountDomain(rreq)
        if err := s.accountSvc.ValidateCreate(acc); err != nil {
            results = append(results, item{Index: i, Error: err.Error(), Code: "invalid"})
            continue
        }
        created, err := s.accountSvc.Create(r.Context(), acc)
        if err != nil {
            switch {
            case errors.Is(err, account.ErrPathExists):
                results = append(results, item{Index: i, Error: err.Error(), Code: "conflict"})
            case errors.Is(err, errs.ErrInvalid):
                results = append(results, item{Index: i, Error: "invalid", Code: "invalid"})
            case errors.Is(err, errs.ErrUnprocessable):
                results = append(results, item{Index: i, Error: "validation_error", Code: "validation_error"})
            default:
                results = append(results, item{Index: i, Error: err.Error(), Code: "error"})
            }
            continue
        }
        ar := accountResponse{ID: created.ID, UserID: created.UserID, Name: created.Name, Currency: created.Currency, Type: created.Type, Method: created.Method, Vendor: created.Vendor, Path: created.Path(), Metadata: created.Metadata, System: created.System, Active: created.Active}
        results = append(results, item{Index: i, Account: &ar})
    }
    toJSON(w, http.StatusOK, results)
}

