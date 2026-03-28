package server

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter manages per-IP rate limiters for different operations.
type RateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*ipLimiter

	tunnelRate rate.Limit
	tunnelBurst int
	connRate   rate.Limit
	connBurst  int

	cleanupInterval time.Duration
}

type ipLimiter struct {
	tunnel  *rate.Limiter
	conn    *rate.Limiter
	lastSeen time.Time
}

// NewRateLimiter creates a new rate limiter with the given limits.
func NewRateLimiter(tunnelPerMin, connPerMin int) *RateLimiter {
	rl := &RateLimiter{
		limiters:        make(map[string]*ipLimiter),
		tunnelRate:      rate.Limit(float64(tunnelPerMin) / 60.0),
		tunnelBurst:     tunnelPerMin,
		connRate:        rate.Limit(float64(connPerMin) / 60.0),
		connBurst:       connPerMin,
		cleanupInterval: 5 * time.Minute,
	}

	go rl.cleanup()
	return rl
}

// AllowTunnelCreation checks if a tunnel creation is allowed for the given IP.
func (rl *RateLimiter) AllowTunnelCreation(ip string) bool {
	limiter := rl.getLimiter(ip)
	return limiter.tunnel.Allow()
}

// AllowConnection checks if a WebSocket connection is allowed for the given IP.
func (rl *RateLimiter) AllowConnection(ip string) bool {
	limiter := rl.getLimiter(ip)
	return limiter.conn.Allow()
}

func (rl *RateLimiter) getLimiter(ip string) *ipLimiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if limiter, ok := rl.limiters[ip]; ok {
		limiter.lastSeen = time.Now()
		return limiter
	}

	limiter := &ipLimiter{
		tunnel:   rate.NewLimiter(rl.tunnelRate, rl.tunnelBurst),
		conn:     rate.NewLimiter(rl.connRate, rl.connBurst),
		lastSeen: time.Now(),
	}
	rl.limiters[ip] = limiter
	return limiter
}

// cleanup periodically removes stale rate limiter entries.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-rl.cleanupInterval * 2)
		for ip, limiter := range rl.limiters {
			if limiter.lastSeen.Before(cutoff) {
				delete(rl.limiters, ip)
			}
		}
		rl.mu.Unlock()
	}
}
