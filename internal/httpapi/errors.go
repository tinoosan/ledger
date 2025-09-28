package httpapi

import (
    "net/http"
    "strings"
)

// errorResponse is the standard error payload for the API.
type errorResponse struct {
    Error string `json:"error"`
    Code  string `json:"code,omitempty"`
}

func writeErr(w http.ResponseWriter, status int, msg, code string) {
    toJSON(w, status, errorResponse{Error: msg, Code: code})
}

func badRequest(w http.ResponseWriter, msg string) { writeErr(w, http.StatusBadRequest, msg, "") }
func notFound(w http.ResponseWriter)               { writeErr(w, http.StatusNotFound, "not_found", "not_found") }
func forbidden(w http.ResponseWriter, msg string)  { writeErr(w, http.StatusForbidden, msg, "") }
func conflict(w http.ResponseWriter, msg string)   { writeErr(w, http.StatusConflict, msg, "") }
func unprocessable(w http.ResponseWriter, msg, code string) {
    writeErr(w, http.StatusUnprocessableEntity, msg, code)
}

// mapValidationError normalizes domain validation errors into a code and message.
func mapValidationError(err error) (code, msg string) {
    if err == nil { return "", "" }
    msg = err.Error()
    code = "validation_error"
    switch {
    case msg == "at least 2 lines":
        code = "too_few_lines"
    case strings.Contains(msg, "amount must be > 0"):
        code = "invalid_amount"
    case strings.Contains(msg, "currency mismatch"):
        code = "mixed_currency"
    case msg == "sum(debits) must equal sum(credits)":
        code = "unbalanced_entry"
    }
    return code, msg
}

