// Entry fetch helper endpoints.
package httpapi

import (
    "net/http"

    chi "github.com/go-chi/chi/v5"
    "github.com/google/uuid"
    "github.com/tinoosan/ledger/internal/errs"
    "errors"
)

// getEntry handles GET /entries/{id}
func (s *Server) getEntry(w http.ResponseWriter, r *http.Request) {
    idStr := chi.URLParam(r, "id")
    id, err := uuid.Parse(idStr)
    if err != nil {
        toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid entry id"})
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
    e, err := s.repo.EntryByID(r.Context(), userID, id)
    if err != nil {
        if errors.Is(err, errs.ErrNotFound) { notFound(w) } else { writeErr(w, http.StatusInternalServerError, "failed to load entry", "") }
        return
    }
    toJSON(w, http.StatusOK, toEntryResponse(e))
}
