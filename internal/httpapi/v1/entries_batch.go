package v1

import (
    "encoding/json"
    "net/http"
)

// postEntriesBatch handles POST /entries/batch
// Accepts an array of entry requests and attempts to create each, returning per-item results.
func (s *Server) postEntriesBatch(w http.ResponseWriter, r *http.Request) {
    var reqs []postEntryRequest
    dec := json.NewDecoder(r.Body)
    dec.DisallowUnknownFields()
    if err := dec.Decode(&reqs); err != nil {
        toJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON: " + err.Error()})
        return
    }
    if len(reqs) == 0 {
        toJSON(w, http.StatusBadRequest, errorResponse{Error: "empty payload"})
        return
    }
    type item struct {
        Index  int            `json:"index"`
        Entry  *entryResponse `json:"entry,omitempty"`
        Error  string         `json:"error,omitempty"`
        Code   string         `json:"code,omitempty"`
    }
    results := make([]item, 0, len(reqs))
    for i := range reqs {
        e := toEntryDomain(reqs[i])
        if err := s.svc.ValidateEntry(r.Context(), e); err != nil {
            code, msg := mapValidationError(err)
            results = append(results, item{Index: i, Error: msg, Code: code})
            continue
        }
        saved, err := s.svc.CreateEntry(r.Context(), e)
        if err != nil {
            results = append(results, item{Index: i, Error: err.Error(), Code: "error"})
            continue
        }
        er := toEntryResponse(saved)
        results = append(results, item{Index: i, Entry: &er})
    }
    toJSON(w, http.StatusOK, results)
}

