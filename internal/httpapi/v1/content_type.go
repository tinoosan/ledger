package v1

import (
    "net/http"
    "strings"
)

// requireJSON ensures the request has Content-Type application/json (optionally with params).
// Writes 415 if not JSON and returns false; otherwise returns true.
func requireJSON(w http.ResponseWriter, r *http.Request) bool {
    ct := r.Header.Get("Content-Type")
    // allow charset or other params after ; and case-insensitive match
    if ct == "" { writeErr(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "unsupported_media_type"); return false }
    mime := strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
    if mime != "application/json" { writeErr(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "unsupported_media_type"); return false }
    return true
}

