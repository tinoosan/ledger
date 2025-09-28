package v1

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/tinoosan/ledger/internal/ledger"
	"github.com/tinoosan/ledger/internal/meta"
	"net/http"
)

// postAccountsBatch handles POST /v1/accounts:batch (and /v1/accounts/batch)
// Atomic: all-or-nothing. Returns 201 with {accounts:[...]} or 422 with {errors:[...]}
func (s *Server) postAccountsBatch(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	// Require Idempotency-Key for batch endpoints
	if r.Header.Get("Idempotency-Key") == "" {
		writeErr(w, http.StatusBadRequest, "idempotency_required", "idempotency_required")
		return
	}
	var req struct {
		UserID   uuid.UUID            `json:"user_id"`
		Accounts []postAccountRequest `json:"accounts"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON: " + err.Error()})
		return
	}
	if req.UserID == uuid.Nil {
		toJSON(w, http.StatusBadRequest, errorResponse{Error: "user_id is required"})
		return
	}
	if len(req.Accounts) == 0 {
		toJSON(w, http.StatusBadRequest, errorResponse{Error: "accounts is required"})
		return
	}
	if len(req.Accounts) > 100 {
		unprocessable(w, "too_many_items", "too_many_items")
		return
	}

	// Idempotency for batch (optional)
	if key := r.Header.Get("Idempotency-Key"); key != "" {
		// normalize body for stable hash
		type normAccount struct {
			UserID   uuid.UUID          `json:"user_id"`
			Name     string             `json:"name"`
			Currency string             `json:"currency"`
			Group    string             `json:"group"`
			Vendor   string             `json:"vendor"`
			Type     ledger.AccountType `json:"type"`
			Metadata meta.Metadata      `json:"metadata,omitempty"`
		}
		type normAcct struct {
			UserID   uuid.UUID     `json:"user_id"`
			Accounts []normAccount `json:"accounts"`
		}
		n := normAcct{UserID: req.UserID, Accounts: make([]normAccount, 0, len(req.Accounts))}
		for _, a := range req.Accounts {
			n.Accounts = append(n.Accounts, normAccount{UserID: req.UserID, Name: a.Name, Currency: a.Currency, Group: a.Group, Vendor: a.Vendor, Type: a.Type, Metadata: meta.New(a.Metadata)})
		}
		nb, _ := json.Marshal(n)
		h := hashBytes(nb)
		s.batchIdemMu.RLock()
		if prev, ok := s.batchIdem[key]; ok {
			if prev.BodyHash == h {
				s.batchIdemMu.RUnlock()
				w.WriteHeader(prev.Status)
				_, _ = w.Write(prev.Payload)
				return
			}
			s.batchIdemMu.RUnlock()
			conflict(w, "idempotency_mismatch")
			return
		}
		s.batchIdemMu.RUnlock()
		// wrap response writer to capture payload
		rw := &captureWriter{ResponseWriter: w}
		// continue processing with rw; at the end store
		// Build domain specs
		specs := make([]ledger.Account, 0, len(req.Accounts))
		for _, a := range req.Accounts {
			specs = append(specs, toAccountDomain(a))
		}
		created, errsList, err := s.accountSvc.EnsureAccountsBatch(r.Context(), req.UserID, specs)
		if err != nil {
			writeErr(rw, http.StatusBadRequest, err.Error(), "")
			s.storeBatch(key, h, rw)
			return
		}
		if len(errsList) > 0 {
			type item struct {
				Index int    `json:"index"`
				Code  string `json:"code"`
				Error string `json:"error"`
			}
			out := struct {
				Errors []item `json:"errors"`
			}{Errors: make([]item, 0, len(errsList))}
			for _, e := range errsList {
				out.Errors = append(out.Errors, item{Index: e.Index, Code: e.Code, Error: e.Err.Error()})
			}
			toJSON(rw, http.StatusUnprocessableEntity, out)
			s.storeBatch(key, h, rw)
			return
		}
		resp := struct {
			Accounts []accountResponse `json:"accounts"`
		}{Accounts: make([]accountResponse, 0, len(created))}
		for _, a := range created {
			resp.Accounts = append(resp.Accounts, accountResponse{ID: a.ID, UserID: a.UserID, Name: a.Name, Currency: a.Currency, Type: a.Type, Group: a.Group, Vendor: a.Vendor, Path: a.Path(), Metadata: a.Metadata, System: a.System, Active: a.Active})
		}
		toJSON(rw, http.StatusCreated, resp)
		s.storeBatch(key, h, rw)
		return
	}

	// Should not reach here; enforced above
}

type captureWriter struct {
	http.ResponseWriter
	status int
	buf    []byte
}

func (w *captureWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }
func (w *captureWriter) Write(b []byte) (int, error) {
	w.buf = append(w.buf, b...)
	return w.ResponseWriter.Write(b)
}

func (s *Server) storeBatch(key, bodyHash string, rw *captureWriter) {
	s.batchIdemMu.Lock()
	s.batchIdem[key] = storedBatch{BodyHash: bodyHash, Status: rw.status, Payload: append([]byte(nil), rw.buf...)}
	s.batchIdemMu.Unlock()
}
