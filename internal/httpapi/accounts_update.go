package httpapi

import (
    "encoding/json"
    "net/http"

    chi "github.com/go-chi/chi/v5"
    "github.com/google/uuid"
    "github.com/tinoosan/ledger/internal/service/account"
)

// updateAccount handles PATCH /accounts/{id}
// Allows updating name, method, vendor, and metadata. Enforces immutability on type/currency.
func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) {
    idStr := chi.URLParam(r, "id")
    id, err := uuid.Parse(idStr)
    if err != nil {
        toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid account id"})
        return
    }
    var payload struct {
        Name     *string            `json:"name"`
        Method   *string            `json:"method"`
        Vendor   *string            `json:"vendor"`
        Metadata map[string]string  `json:"metadata"`
    }
    dec := json.NewDecoder(r.Body)
    dec.DisallowUnknownFields()
    if err := dec.Decode(&payload); err != nil {
        toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON: "+err.Error()})
        return
    }
    in := account.UpdateInput{
        Name: payload.Name,
        Method: payload.Method,
        Vendor: payload.Vendor,
        Metadata: payload.Metadata,
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
    acc, err := s.accountSvc.Update(r.Context(), userID, id, in)
    if err != nil {
        // map known errors
        if err.Error() == "system accounts cannot be modified" {
            toJSON(w, http.StatusForbidden, errorResponse{Error: err.Error()})
            return
        }
        if err == account.ErrPathExists {
            toJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
            return
        }
        toJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
        return
    }
    resp := accountResponse{ID: acc.ID, UserID: acc.UserID, Name: acc.Name, Currency: acc.Currency, Type: acc.Type, Method: acc.Method, Vendor: acc.Vendor, Path: acc.Path(), Metadata: acc.Metadata}
    toJSON(w, http.StatusOK, resp)
}

// deactivateAccount handles DELETE /accounts/{id} by soft-deactivating via metadata.active=false
func (s *Server) deactivateAccount(w http.ResponseWriter, r *http.Request) {
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
    if err := s.accountSvc.Deactivate(r.Context(), userID, id); err != nil {
        if err.Error() == "system accounts cannot be deactivated" {
            toJSON(w, http.StatusForbidden, errorResponse{Error: err.Error()})
            return
        }
        toJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
        return
    }
    w.WriteHeader(http.StatusNoContent)
}
