package middleware

import (
	"strconv"
	"sync"
	"time"

	"zawaj/pkg/response"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// visitor is a per-key token bucket plus its last-seen time for eviction.
type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// keyedLimiter holds one token bucket per client key (IP), evicting idle keys so
// the map cannot grow unbounded. It is an in-process limiter: adequate for a
// single instance; move to a shared store (Redis) when running multiple replicas.
type keyedLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rps      rate.Limit
	burst    int
	calls    int // guarded by mu; drives opportunistic sweeps
}

func newKeyedLimiter(rps float64, burst int) *keyedLimiter {
	kl := &keyedLimiter{
		visitors: make(map[string]*visitor),
		rps:      rate.Limit(rps),
		burst:    burst,
	}
	return kl
}

// get returns the bucket for key, creating it if needed, and every 1000th call
// sweeps keys idle longer than idleTTL so the map cannot grow unbounded. All
// state (map + call counter) is mutated under mu, keeping it race-free.
func (kl *keyedLimiter) get(key string, now time.Time, idleTTL time.Duration) *rate.Limiter {
	kl.mu.Lock()
	defer kl.mu.Unlock()

	kl.calls++
	if kl.calls%1000 == 0 {
		for k, v := range kl.visitors {
			if now.Sub(v.lastSeen) > idleTTL {
				delete(kl.visitors, k)
			}
		}
	}

	v, ok := kl.visitors[key]
	if !ok {
		lim := rate.NewLimiter(kl.rps, kl.burst)
		kl.visitors[key] = &visitor{limiter: lim, lastSeen: now}
		return lim
	}
	v.lastSeen = now
	return v.limiter
}

// RateLimit rejects requests from a client (keyed by ClientIP) that exceed rps
// with the given burst, returning 429 + Retry-After. rps <= 0 disables limiting.
// ClientIP is only trustworthy when the engine's trusted proxies are configured
// correctly (see router.New, which does not trust X-Forwarded-For by default).
func RateLimit(rps float64, burst int) gin.HandlerFunc {
	if rps <= 0 {
		return func(c *gin.Context) { c.Next() }
	}
	kl := newKeyedLimiter(rps, burst)
	const idleTTL = 10 * time.Minute

	return func(c *gin.Context) {
		now := time.Now()
		if !kl.get(c.ClientIP(), now, idleTTL).Allow() {
			c.Writer.Header().Set("Retry-After", strconv.Itoa(1))
			response.Error(c, 429, response.CodeRateLimited, "too many requests")
			c.Abort()
			return
		}
		c.Next()
	}
}
