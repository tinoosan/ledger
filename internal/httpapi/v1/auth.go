package v1

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/base64"
    "encoding/json"
    "errors"
    "net/http"
    "os"
    "strings"
    "time"
)

type jwtClaims struct {
    Iss string   `json:"iss,omitempty"`
    Sub string   `json:"sub,omitempty"`
    Aud any      `json:"aud,omitempty"` // string or []string
    Exp int64    `json:"exp,omitempty"`
    Nbf int64    `json:"nbf,omitempty"`
    Iat int64    `json:"iat,omitempty"`
}

func parseBearer(r *http.Request) (string, bool) {
    h := r.Header.Get("Authorization")
    if h == "" { return "", false }
    if !strings.HasPrefix(h, "Bearer ") && !strings.HasPrefix(h, "bearer ") { return "", false }
    return strings.TrimSpace(h[len("Bearer "):]), true
}

func base64urlDecode(s string) ([]byte, error) {
    // JWT uses base64url without padding
    if m := len(s) % 4; m != 0 { s += strings.Repeat("=", 4-m) }
    return base64.URLEncoding.DecodeString(s)
}

func verifyHS256(token, secret string) (jwtClaims, error) {
    var empty jwtClaims
    parts := strings.Split(token, ".")
    if len(parts) != 3 { return empty, errors.New("invalid token format") }
    headerB, err := base64urlDecode(parts[0])
    if err != nil { return empty, errors.New("bad header b64") }
    payloadB, err := base64urlDecode(parts[1])
    if err != nil { return empty, errors.New("bad payload b64") }
    sigB, err := base64urlDecode(parts[2])
    if err != nil { return empty, errors.New("bad signature b64") }

    // Expect alg HS256
    var hdr struct{ Alg, Typ string }
    if err := json.Unmarshal(headerB, &hdr); err != nil { return empty, errors.New("bad header json") }
    if !strings.EqualFold(hdr.Alg, "HS256") { return empty, errors.New("unsupported alg") }

    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(parts[0]))
    mac.Write([]byte{"."[0]})
    mac.Write([]byte(parts[1]))
    sum := mac.Sum(nil)
    if !hmac.Equal(sigB, sum) { return empty, errors.New("invalid signature") }

    var claims jwtClaims
    if err := json.Unmarshal(payloadB, &claims); err != nil { return empty, errors.New("bad claims json") }
    return claims, nil
}

func audContains(aud any, expected string) bool {
    if expected == "" { return true }
    switch v := aud.(type) {
    case string:
        return strings.EqualFold(v, expected)
    case []any:
        for _, it := range v { if s, ok := it.(string); ok && strings.EqualFold(s, expected) { return true } }
    case []string:
        for _, s := range v { if strings.EqualFold(s, expected) { return true } }
    }
    return false
}

// authJWTFromEnv returns a middleware that enforces Authorization: Bearer JWT (HS256)
// when JWT_HS256_SECRET is set. Optional checks: JWT_ISSUER, JWT_AUDIENCE.
func authJWTFromEnv() func(http.Handler) http.Handler {
    secret := strings.TrimSpace(os.Getenv("JWT_HS256_SECRET"))
    if secret == "" { return nil }
    iss := strings.TrimSpace(os.Getenv("JWT_ISSUER"))
    aud := strings.TrimSpace(os.Getenv("JWT_AUDIENCE"))
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // allow unauthenticated for health and spec
            switch r.URL.Path {
            case "/healthz", "/readyz", "/v1/openapi.yaml":
                next.ServeHTTP(w, r); return
            }
            if strings.HasPrefix(r.URL.Path, "/v1/dictionary/") {
                next.ServeHTTP(w, r); return
            }
            tok, ok := parseBearer(r)
            if !ok { w.WriteHeader(http.StatusUnauthorized); return }
            claims, err := verifyHS256(tok, secret)
            if err != nil { w.WriteHeader(http.StatusUnauthorized); return }
            now := time.Now().Unix()
            if claims.Nbf != 0 && now < claims.Nbf { w.WriteHeader(http.StatusUnauthorized); return }
            if claims.Exp != 0 && now >= claims.Exp { w.WriteHeader(http.StatusUnauthorized); return }
            if iss != "" && !strings.EqualFold(claims.Iss, iss) { w.WriteHeader(http.StatusUnauthorized); return }
            if aud != "" && !audContains(claims.Aud, aud) { w.WriteHeader(http.StatusUnauthorized); return }
            next.ServeHTTP(w, r)
        })
    }
}

