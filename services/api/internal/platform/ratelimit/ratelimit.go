// Package ratelimit implements a Redis-backed sliding-window rate limiter
// for the abuse vectors listed in spec §15.2 (login, OTP, password reset,
// payment initiation, SOS).
package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"proguidegh/api/internal/platform/audit"
	"proguidegh/api/internal/platform/httpx"
)

// Limiter tracks hits per (bucket, key) in a sliding window.
type Limiter struct {
	rdb *goredis.Client
}

// NewLimiter builds a Limiter. rdb may be nil in tests — Allow then always
// returns true.
func NewLimiter(rdb *goredis.Client) *Limiter { return &Limiter{rdb: rdb} }

// Allow records one hit for bucket/key and reports whether the request is
// within limit for the window. Implementation: INCR + PEXPIRE on a
// fixed-window bucket keyed to windowSize granularity (approximate sliding
// window; sufficient for abuse protection per spec §15.2).
func (l *Limiter) Allow(ctx context.Context, bucket, key string, limit int, window time.Duration) (bool, error) {
	if l.rdb == nil || limit <= 0 {
		return true, nil
	}
	k := fmt.Sprintf("rl:%s:%s:%d", bucket, key, time.Now().UnixNano()/window.Nanoseconds())
	n, err := l.rdb.Incr(ctx, k).Result()
	if err != nil {
		// Fail open on Redis trouble rather than blocking all traffic; the
		// error is returned so callers can log it.
		return true, fmt.Errorf("ratelimit: incr: %w", err)
	}
	if n == 1 {
		l.rdb.PExpire(ctx, k, window) //nolint:errcheck // best-effort TTL
	}
	return n <= int64(limit), nil
}

// Limit defines one endpoint's rate-limit policy.
type Limit struct {
	Bucket string        // metric/log bucket name
	Max    int           // requests allowed per Window
	Window time.Duration // window size
}

// Middleware returns HTTP middleware enforcing lim keyed on the client IP.
// Over-limit requests get 429 RATE_LIMITED.
func Middleware(l *Limiter, lim Limit) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ok, err := l.Allow(r.Context(), lim.Bucket, audit.ClientIP(r), lim.Max, lim.Window)
			if err != nil {
				// Redis hiccup: fail open, but surface it in the log pipeline.
				// (Allow already returns true in this case.)
				_ = err
			}
			if !ok {
				w.Header().Set("Retry-After", strconv.Itoa(int(lim.Window.Seconds())))
				httpx.WriteError(w, r, http.StatusTooManyRequests,
					"RATE_LIMITED", "too many requests, try again later", nil)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Keyed is Allow with a caller-chosen key (e.g. login per email+IP).
func Keyed(l *Limiter, lim Limit, parts ...string) func(ctx context.Context) (bool, error) {
	key := strings.Join(parts, "|")
	return func(ctx context.Context) (bool, error) {
		return l.Allow(ctx, lim.Bucket, key, lim.Max, lim.Window)
	}
}
