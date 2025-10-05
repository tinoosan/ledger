package v1

import (
    "context"
    "crypto"
    "crypto/hmac"
    "crypto/rsa"
    "crypto/sha256"
    "encoding/base64"
    "encoding/json"
    "errors"
    "math/big"
    "net/http"
    "os"
    "strconv"
    "strings"
    "sync"
    "time"

    chimw "github.com/go-chi/chi/v5/middleware"
    "log/slog"
)

type JWTClaims struct {
	Issuer    string `json:"iss,omitempty"`
	Subject   string `json:"sub,omitempty"`
	Audience  any    `json:"aud,omitempty"` // string or []string
	ExpiresAt int64  `json:"exp,omitempty"`
	NotBefore int64  `json:"nbf,omitempty"`
	IssuedAt  int64  `json:"iat,omitempty"`
}

func parseBearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", false
	}
	if !strings.HasPrefix(h, "Bearer ") && !strings.HasPrefix(h, "bearer ") {
		return "", false
	}
	return strings.TrimSpace(h[len("Bearer "):]), true
}

func base64URLDecode(s string) ([]byte, error) {
	// JWT uses base64url without padding
	if m := len(s) % 4; m != 0 {
		s += strings.Repeat("=", 4-m)
	}
	return base64.URLEncoding.DecodeString(s)
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
	Kid string `json:"kid,omitempty"`
}

func verifyHS256(token, secret string) (JWTClaims, error) {
	var empty JWTClaims
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return empty, errors.New("invalid token format")
	}
	headerB, err := base64URLDecode(parts[0])
	if err != nil {
		return empty, errors.New("bad header b64")
	}
	payloadB, err := base64URLDecode(parts[1])
	if err != nil {
		return empty, errors.New("bad payload b64")
	}
	sigB, err := base64URLDecode(parts[2])
	if err != nil {
		return empty, errors.New("bad signature b64")
	}

	// Expect alg HS256
	var hdr jwtHeader
	if err := json.Unmarshal(headerB, &hdr); err != nil {
		return empty, errors.New("bad header json")
	}
	if !strings.EqualFold(hdr.Alg, "HS256") {
		return empty, errors.New("unsupported alg")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0]))
	mac.Write([]byte{"."[0]})
	mac.Write([]byte(parts[1]))
	sum := mac.Sum(nil)
	if !hmac.Equal(sigB, sum) {
		return empty, errors.New("invalid signature")
	}

	var claims JWTClaims
	if err := json.Unmarshal(payloadB, &claims); err != nil {
		return empty, errors.New("bad claims json")
	}
	return claims, nil
}

func audContains(aud any, expected string) bool {
	if expected == "" {
		return true
	}
	switch v := aud.(type) {
	case string:
		return strings.EqualFold(v, expected)
	case []any:
		for _, it := range v {
			if s, ok := it.(string); ok && strings.EqualFold(s, expected) {
				return true
			}
		}
	case []string:
		for _, s := range v {
			if strings.EqualFold(s, expected) {
				return true
			}
		}
	}
	return false
}

// JWKS (RS256) support -----------------------------------------------------

type jwkRSA struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwksDoc struct {
	Keys []jwkRSA `json:"keys"`
}

type jwksCache struct {
	url   string
	ttl   time.Duration
	mu    sync.RWMutex
	exp   time.Time
	keys  map[string]*rsa.PublicKey
	httpc *http.Client
}

func newJWKSCache(url string, ttl time.Duration) *jwksCache {
	return &jwksCache{url: url, ttl: ttl, keys: make(map[string]*rsa.PublicKey), httpc: &http.Client{Timeout: 5 * time.Second}}
}

func (c *jwksCache) get(ctx context.Context, kid string) *rsa.PublicKey {
	c.mu.RLock()
	if k, ok := c.keys[kid]; ok && time.Now().Before(c.exp) {
		c.mu.RUnlock()
		return k
	}
	c.mu.RUnlock()
	// refresh
	_ = c.refresh(ctx)
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.keys[kid]
}

func (c *jwksCache) refresh(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Now().Before(c.exp) {
		return nil
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	resp, err := c.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var doc jwksDoc
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&doc); err != nil {
		return err
	}
	keys := make(map[string]*rsa.PublicKey)
	for _, k := range doc.Keys {
		if !strings.EqualFold(k.Kty, "RSA") || k.N == "" || k.E == "" || k.Kid == "" {
			continue
		}
		nBytes, err := base64URLDecode(k.N)
		if err != nil {
			continue
		}
		eBytes, err := base64URLDecode(k.E)
		if err != nil {
			continue
		}
		n := new(big.Int).SetBytes(nBytes)
		eb := new(big.Int).SetBytes(eBytes)
		if !eb.IsInt64() {
			continue
		}
		pub := &rsa.PublicKey{N: n, E: int(eb.Int64())}
		keys[k.Kid] = pub
	}
	c.keys = keys
	c.exp = time.Now().Add(c.ttl)
	return nil
}

func verifyRS256(token string, lookup func(kid string) *rsa.PublicKey) (JWTClaims, error) {
	var empty JWTClaims
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return empty, errors.New("invalid token format")
	}
	headerB, err := base64URLDecode(parts[0])
	if err != nil {
		return empty, errors.New("bad header b64")
	}
	payloadB, err := base64URLDecode(parts[1])
	if err != nil {
		return empty, errors.New("bad payload b64")
	}
	sigB, err := base64URLDecode(parts[2])
	if err != nil {
		return empty, errors.New("bad signature b64")
	}
	var hdr jwtHeader
	if err := json.Unmarshal(headerB, &hdr); err != nil {
		return empty, errors.New("bad header json")
	}
	if !strings.EqualFold(hdr.Alg, "RS256") {
		return empty, errors.New("unsupported alg")
	}
	if hdr.Kid == "" {
		return empty, errors.New("missing kid")
	}
	pub := lookup(hdr.Kid)
	if pub == nil {
		return empty, errors.New("unknown kid")
	}
	signed := parts[0] + "." + parts[1]
	sum := sha256.Sum256([]byte(signed))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sigB); err != nil {
		return empty, errors.New("invalid signature")
	}
	var claims JWTClaims
	if err := json.Unmarshal(payloadB, &claims); err != nil {
		return empty, errors.New("bad claims json")
	}
	return claims, nil
}

// Prefer RS256 via JWKS when configured; fallback to HS256 otherwise.
func authJWTFromEnv() func(http.Handler) http.Handler {
    // Note: logger obtained via slog.Default(); router wires slog.SetDefault.
    // We avoid logging any token or secret material; only reasons and safe claims.
    // If LOG_LEVEL=DEBUG, these messages help diagnose auth failures.
    logger := slog.Default()
    jwksURL := strings.TrimSpace(os.Getenv("JWT_JWKS_URL"))
    secret := strings.TrimSpace(os.Getenv("JWT_HS256_SECRET"))
    iss := strings.TrimSpace(os.Getenv("JWT_ISSUER"))
    aud := strings.TrimSpace(os.Getenv("JWT_AUDIENCE"))
    var cache *jwksCache
    if jwksURL != "" {
        ttl := 300 * time.Second
        if v := strings.TrimSpace(os.Getenv("JWT_JWKS_TTL")); v != "" {
            if n, err := strconv.Atoi(v); err == nil && n > 0 {
                ttl = time.Duration(n) * time.Second
            }
        }
        cache = newJWKSCache(jwksURL, ttl)
        logger.Debug("auth configured: RS256 via JWKS", "jwks_url", jwksURL, "ttl_seconds", int64(cache.ttl/time.Second))
    }
    if jwksURL == "" && secret == "" {
        return nil
    }
    if jwksURL == "" && secret != "" {
        logger.Debug("auth configured: HS256 via shared secret")
    }

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Attach request id if present for correlation
            reqID := chimw.GetReqID(r.Context())
            switch r.URL.Path {
            case "/healthz", "/readyz", "/metrics", "/v1/openapi.yaml":
                next.ServeHTTP(w, r)
                return
            }
            if strings.HasPrefix(r.URL.Path, "/v1/dictionary/") {
                next.ServeHTTP(w, r)
                return
            }

            tok, ok := parseBearerToken(r)
            if !ok {
                logger.Debug("auth failed: missing or malformed Authorization header", "req_id", reqID, "path", r.URL.Path, "method", r.Method)
                w.WriteHeader(http.StatusUnauthorized)
                return
            }

            var claims JWTClaims
            var err error
            if cache != nil {
                claims, err = verifyRS256(tok, func(kid string) *rsa.PublicKey { return cache.get(r.Context(), kid) })
                if err != nil && secret != "" {
                    logger.Debug("RS256 verify failed; attempting HS256 fallback", "req_id", reqID, "path", r.URL.Path, "method", r.Method, "err", err.Error())
                    claims, err = verifyHS256(tok, secret)
                }
            } else if secret != "" {
                claims, err = verifyHS256(tok, secret)
            }
            if err != nil {
                logger.Debug("auth failed: signature/structure verification", "req_id", reqID, "path", r.URL.Path, "method", r.Method, "err", err.Error())
                w.WriteHeader(http.StatusUnauthorized)
                return
            }

            now := time.Now().Unix()
            if claims.NotBefore != 0 && now < claims.NotBefore {
                logger.Debug("auth failed: token not yet valid (nbf)", "req_id", reqID, "path", r.URL.Path, "method", r.Method, "nbf", claims.NotBefore, "now", now)
                w.WriteHeader(http.StatusUnauthorized)
                return
            }
            if claims.ExpiresAt != 0 && now >= claims.ExpiresAt {
                logger.Debug("auth failed: token expired (exp)", "req_id", reqID, "path", r.URL.Path, "method", r.Method, "exp", claims.ExpiresAt, "now", now)
                w.WriteHeader(http.StatusUnauthorized)
                return
            }
            if iss != "" && !strings.EqualFold(claims.Issuer, iss) {
                logger.Debug("auth failed: issuer mismatch", "req_id", reqID, "path", r.URL.Path, "method", r.Method, "got_iss", claims.Issuer, "expected_iss", iss)
                w.WriteHeader(http.StatusUnauthorized)
                return
            }
            if aud != "" && !audContains(claims.Audience, aud) {
                logger.Debug("auth failed: audience mismatch", "req_id", reqID, "path", r.URL.Path, "method", r.Method, "expected_aud", aud)
                w.WriteHeader(http.StatusUnauthorized)
                return
            }
            // Successful auth at debug for traceability
            logger.Debug("auth ok", "req_id", reqID, "path", r.URL.Path, "method", r.Method)
            next.ServeHTTP(w, r)
        })
    }
}
