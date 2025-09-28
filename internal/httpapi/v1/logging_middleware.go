package v1

import (
    "net"
    "net/http"
    "runtime/debug"
    "strings"
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
            // Avoid noisy logs for health endpoints
            if r.URL.Path != "/healthz" && r.URL.Path != "/readyz" {
                l.Info("request started",
                    "req_id", reqID,
                    "method", r.Method,
                    "path", r.URL.Path,
                    "ip", clientIP(r),
                    "ua", r.UserAgent(),
                )
            }

            next.ServeHTTP(ww, r)

            dur := time.Since(start)
            // Choose level by status code
            lvl := levelForStatus(ww.Status())
            if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
                // Downgrade health logs to debug
                lvl = slog.LevelDebug
            }
            attrs := []any{
                "req_id", reqID,
                "method", r.Method,
                "path", r.URL.Path,
                "status", ww.Status(),
                "bytes", ww.BytesWritten(),
                "duration_ms", dur.Milliseconds(),
            }
            if ip := clientIP(r); ip != "" { attrs = append(attrs, "ip", ip) }
            if ua := r.UserAgent(); ua != "" { attrs = append(attrs, "ua", ua) }
            switch lvl {
            case slog.LevelError:
                l.Error("request complete", attrs...)
            case slog.LevelWarn:
                l.Warn("request complete", attrs...)
            case slog.LevelDebug:
                l.Debug("request complete", attrs...)
            default:
                l.Info("request complete", attrs...)
            }
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
                    // Avoid logging request body or query params; include stack for diagnostics
                    l.Error("panic", "req_id", reqID, "err", rec, "path", r.URL.Path, "method", r.Method, "stack", string(debug.Stack()))
                    w.WriteHeader(http.StatusInternalServerError)
                }
            }()
            next.ServeHTTP(w, r)
        })
    }
}

func levelForStatus(status int) slog.Level {
    switch {
    case status >= 500:
        return slog.LevelError
    case status >= 400:
        return slog.LevelWarn
    default:
        return slog.LevelInfo
    }
}

func clientIP(r *http.Request) string {
    // Prefer X-Forwarded-For first value
    if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
        parts := strings.Split(xff, ",")
        if len(parts) > 0 { return strings.TrimSpace(parts[0]) }
    }
    // Fallback to RemoteAddr host part
    host, _, err := net.SplitHostPort(r.RemoteAddr)
    if err == nil { return host }
    return r.RemoteAddr
}
