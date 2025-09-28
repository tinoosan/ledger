package v1

import (
    "net/http"
    "runtime/debug"
    "time"

    chimw "github.com/go-chi/chi/v5/middleware"
    "log/slog"
)

// requestLogger logs basic request info at INFO and panics at ERROR.
func requestLogger(l *slog.Logger) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
            start := time.Now()

            reqID := chimw.GetReqID(r.Context())
            l.Info("request started", "req_id", reqID, "method", r.Method, "path", r.URL.Path)

            next.ServeHTTP(ww, r)

            l.Info("request complete",
                "req_id", reqID,
                "status", ww.Status(),
                "bytes", ww.BytesWritten(),
                "duration", time.Since(start).String(),
            )
        })
    }
}

// recoverer logs panics as ERROR and returns 500.
func recoverer(l *slog.Logger) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            defer func() {
                if rec := recover(); rec != nil {
                    reqID := chimw.GetReqID(r.Context())
                    l.Error("panic", "req_id", reqID, "err", rec, "stack", string(debug.Stack()))
                    w.WriteHeader(http.StatusInternalServerError)
                }
            }()
            next.ServeHTTP(w, r)
        })
    }
}
