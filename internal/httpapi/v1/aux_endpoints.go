package v1

import (
    "context"
    "net/http"
    "time"
)

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
func (s *Server) readyz(w http.ResponseWriter, r *http.Request)  {
    // If the underlying stores implement ReadyChecker, call it with a short timeout
    type readyIf interface{ Ready(context.Context) error }
    deadline := 800 * time.Millisecond
    ctx, cancel := context.WithTimeout(r.Context(), deadline)
    defer cancel()
    if rc, ok := any(s.accReader).(readyIf); ok {
        if err := rc.Ready(ctx); err != nil { w.WriteHeader(http.StatusServiceUnavailable); return }
    }
    if rc, ok := any(s.entryReader).(readyIf); ok {
        if err := rc.Ready(ctx); err != nil { w.WriteHeader(http.StatusServiceUnavailable); return }
    }
    if rc, ok := any(s.idemStore).(readyIf); ok {
        if err := rc.Ready(ctx); err != nil { w.WriteHeader(http.StatusServiceUnavailable); return }
    }
    w.WriteHeader(http.StatusOK)
}

// openapiSpec serves the local OpenAPI file for convenience in dev.
func (s *Server) openapiSpec(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/yaml")
    http.ServeFile(w, r, "openapi/openapi.yaml")
}
