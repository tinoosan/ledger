package httpapi

import (
    "net/http"
)

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
func (s *Server) readyz(w http.ResponseWriter, r *http.Request)  { w.WriteHeader(http.StatusOK) }

// openapiSpec serves the local OpenAPI file for convenience in dev.
func (s *Server) openapiSpec(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/yaml")
    http.ServeFile(w, r, "openapi/openapi.yaml")
}
