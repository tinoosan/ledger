package httpapi

import (
    "net/http"

    chi "github.com/go-chi/chi/v5"
    "github.com/google/uuid"
)

// GET /idempotency/entries/{client_entry_id}?user_id=
func (s *Server) getEntryByClientID(w http.ResponseWriter, r *http.Request) {
    clientID := chi.URLParam(r, "client_entry_id")
    userID := r.URL.Query().Get("user_id")
    if clientID == "" || userID == "" { toJSON(w, http.StatusBadRequest, errorResponse{Error: "user_id and client_entry_id required"}); return }
    uid, err := uuid.Parse(userID); if err != nil { toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid user_id"}); return }
    e, ok, err := s.repo.EntryByClientID(r.Context(), uid, clientID)
    if err != nil { toJSON(w, http.StatusInternalServerError, errorResponse{Error: "lookup error"}); return }
    if !ok { w.WriteHeader(http.StatusNotFound); return }
    toJSON(w, http.StatusOK, toEntryResponse(e))
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
func (s *Server) readyz(w http.ResponseWriter, r *http.Request)  { w.WriteHeader(http.StatusOK) }
