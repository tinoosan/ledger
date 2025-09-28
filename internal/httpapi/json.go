package httpapi

import (
    "encoding/json"
    "net/http"
)

// toJSON writes a JSON response with status code.
func toJSON(w http.ResponseWriter, status int, v any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(v)
}
