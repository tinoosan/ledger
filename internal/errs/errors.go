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
    // ErrSystemAccount indicates a system account cannot be modified/deactivated
    ErrSystemAccount = errors.New("system_account")
    // ErrImmutable indicates an attempt to change immutable fields
    ErrImmutable = errors.New("immutable")
    // Journal/domain validation errors
    ErrTooFewLines     = errors.New("too_few_lines")
    ErrInvalidAmount   = errors.New("invalid_amount")
    ErrMixedCurrency   = errors.New("mixed_currency")
    ErrUnbalancedEntry = errors.New("unbalanced_entry")
)
