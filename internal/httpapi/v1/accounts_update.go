package v1

import (
    "encoding/json"
    "net/http"

    chi "github.com/go-chi/chi/v5"
    "github.com/google/uuid"
    "github.com/tinoosan/ledger/internal/service/account"
    "github.com/tinoosan/ledger/internal/errs"
    "errors"
    "github.com/tinoosan/ledger/internal/meta"
)

// updateAccount handles PATCH /accounts/{id}
// Allows updating name, method, vendor, and metadata. Enforces immutability on type/currency.
func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) {
    if !requireJSON(w, r) { return }
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
    // load current, apply patch in http layer
    acc, err := s.accReader.GetAccount(r.Context(), userID, id)
    if err != nil {
        if errors.Is(err, errs.ErrNotFound) { notFound(w) } else { writeErr(w, http.StatusInternalServerError, "failed to load account", "") }
        return
    }
    if payload.Name != nil { acc.Name = *payload.Name }
    if payload.Method != nil { acc.Method = *payload.Method }
    if payload.Vendor != nil { acc.Vendor = *payload.Vendor }
    if payload.Metadata != nil {
        // validate and merge
        m := meta.New(payload.Metadata)
        if err := m.Validate(); err != nil { unprocessable(w, "validation_error", "validation_error"); return }
        if acc.Metadata == nil { acc.Metadata = meta.Metadata{} }
        acc.Metadata.Merge(m)
    }
    acc, err = s.accountSvc.Update(r.Context(), acc)
    if err != nil {
        if errors.Is(err, errs.ErrSystemAccount) { writeErr(w, http.StatusForbidden, "system_account", "system_account"); return }
        if errors.Is(err, errs.ErrForbidden) { forbidden(w, "forbidden") ; return }
        if errors.Is(err, errs.ErrInvalid) { badRequest(w, "invalid") ; return }
        if errors.Is(err, errs.ErrImmutable) { unprocessable(w, "immutable", "immutable"); return }
        if errors.Is(err, errs.ErrUnprocessable) { unprocessable(w, "validation_error", "validation_error"); return }
        if errors.Is(err, account.ErrPathExists) { conflict(w, err.Error()); return }
        writeErr(w, http.StatusBadRequest, err.Error(), "")
        return
    }
    resp := accountResponse{ID: acc.ID, UserID: acc.UserID, Name: acc.Name, Currency: acc.Currency, Type: acc.Type, Method: acc.Method, Vendor: acc.Vendor, Path: acc.Path(), Metadata: acc.Metadata, System: acc.System, Active: acc.Active}
    toJSON(w, http.StatusOK, resp)
}

// deactivateAccount handles DELETE /accounts/{id} by soft-deactivating (active=false)
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
        if errors.Is(err, errs.ErrSystemAccount) { writeErr(w, http.StatusForbidden, "system_account", "system_account"); return }
        if errors.Is(err, errs.ErrForbidden) { forbidden(w, "forbidden"); return }
        if errors.Is(err, errs.ErrInvalid) { badRequest(w, "invalid"); return }
        if errors.Is(err, errs.ErrNotFound) { notFound(w); return }
        writeErr(w, http.StatusBadRequest, err.Error(), "")
        return
    }
    w.WriteHeader(http.StatusNoContent)
}
