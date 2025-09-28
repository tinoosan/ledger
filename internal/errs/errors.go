package errs

import "errors"

// Common sentinel errors for cross-layer signaling.
var (
    ErrNotFound = errors.New("not_found")
    ErrForbidden = errors.New("forbidden")
    ErrConflict  = errors.New("conflict")
    ErrInvalid   = errors.New("invalid")
    // ErrUnprocessable is used for semantic validation failures (HTTP 422)
    ErrUnprocessable = errors.New("unprocessable")
)
