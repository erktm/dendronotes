package auth

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// rateLimiter limits login attempts per client IP and globally. The global
// bucket bounds the total argon2 work even if an attacker rotates addresses.
type rateLimiter struct {
	mu     sync.Mutex
	perKey map[string]*keyLimiter
	every  time.Duration
	burst  int
	global *rate.Limiter
	now    func() time.Time
}

type keyLimiter struct {
	lim  *rate.Limiter
	seen time.Time
}

const (
	maxTrackedKeys = 10_000
	idleKeyTTL     = time.Hour
)

func newRateLimiter(every time.Duration, burst int, globalEvery time.Duration, globalBurst int) *rateLimiter {
	return &rateLimiter{
		perKey: map[string]*keyLimiter{},
		every:  every,
		burst:  burst,
		global: rate.NewLimiter(rate.Every(globalEvery), globalBurst),
		now:    time.Now,
	}
}

// allow consumes one attempt for key. If the attempt is not allowed, it
// returns how long the client should wait.
func (l *rateLimiter) allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.cleanupLocked(now)

	k, ok := l.perKey[key]
	if !ok {
		k = &keyLimiter{lim: rate.NewLimiter(rate.Every(l.every), l.burst)}
		l.perKey[key] = k
	}
	k.seen = now

	r := k.lim.ReserveN(now, 1)
	if d := r.DelayFrom(now); d > 0 {
		r.CancelAt(now)
		return false, d
	}
	g := l.global.ReserveN(now, 1)
	if d := g.DelayFrom(now); d > 0 {
		g.CancelAt(now)
		r.CancelAt(now)
		return false, d
	}
	return true, 0
}

func (l *rateLimiter) cleanupLocked(now time.Time) {
	if len(l.perKey) < maxTrackedKeys/10 {
		return
	}
	for key, k := range l.perKey {
		if now.Sub(k.seen) > idleKeyTTL {
			delete(l.perKey, key)
		}
	}
	if len(l.perKey) >= maxTrackedKeys {
		// Address flood: fall back to the global limit only.
		clear(l.perKey)
	}
}
