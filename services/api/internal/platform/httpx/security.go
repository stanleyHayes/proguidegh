package httpx

import "net/http"

// SecurityHeadersMiddleware sets the baseline response headers for a JSON
// API (P9-01). The API serves no HTML, so the policy is strict: nosniff,
// DENY framing, no referrer leakage and a deny-all content policy.
// HSTS is the TLS terminator's job (Cloudflare in production) and is only
// meaningful over HTTPS, so it is not set here.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// CORSMiddleware allows credentialed cross-origin calls from the configured
// web-app origins (P9-01). The web apps fetch the API directly with session
// cookies, so the origin must be reflected explicitly — never "*" with
// credentials. Origins not on the allowlist get no CORS headers (the
// browser blocks the response server-side-transparently).
func CORSMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, origin := range allowedOrigins {
		allowed[origin] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && allowed[origin] {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Set("Vary", "Origin")
				if r.Method == http.MethodOptions {
					h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
					h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Idempotency-Key")
					h.Set("Access-Control-Max-Age", "600")
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
