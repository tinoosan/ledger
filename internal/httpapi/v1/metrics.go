package v1

import (
    "net/http"
    "time"

    chimw "github.com/go-chi/chi/v5/middleware"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
    httpRequestsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Namespace: "ledger",
            Name:      "http_requests_total",
            Help:      "Total number of HTTP requests",
        },
        []string{"method", "status"},
    )
    httpRequestDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Namespace: "ledger",
            Name:      "http_request_duration_seconds",
            Help:      "Duration of HTTP requests in seconds",
            Buckets:   prometheus.DefBuckets,
        },
        []string{"method", "status"},
    )
)

func metricsHandler() http.Handler {
    return promhttp.Handler()
}

func metricsMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
        start := time.Now()
        next.ServeHTTP(ww, r)
        status := ww.Status()
        method := r.Method
        httpRequestsTotal.WithLabelValues(method, itoa(status)).Inc()
        httpRequestDuration.WithLabelValues(method, itoa(status)).Observe(time.Since(start).Seconds())
    })
}

// small local int to ascii to avoid fmt in hot path (already present in journal service)
func itoa(n int) string {
    if n == 0 { return "0" }
    neg := false
    if n < 0 { neg = true; n = -n }
    var buf [20]byte
    i := len(buf)
    for n > 0 { i--; buf[i] = byte('0' + n%10); n /= 10 }
    if neg { i--; buf[i] = '-' }
    return string(buf[i:])
}

