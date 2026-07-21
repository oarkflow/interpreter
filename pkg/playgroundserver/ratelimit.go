package playgroundserver

import (
	"sync"
	"time"
)

type rateLimiter struct {
	mu      sync.Mutex
	clients map[string]*clientCounter
	limit   int
	window  time.Duration
}

type clientCounter struct {
	count int
	reset time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{clients: make(map[string]*clientCounter), limit: limit, window: window}
}

func (rl *rateLimiter) allow(key string, now time.Time) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cc, ok := rl.clients[key]
	if !ok || now.After(cc.reset) {
		rl.clients[key] = &clientCounter{count: 1, reset: now.Add(rl.window)}
		return true
	}
	if cc.count >= rl.limit {
		return false
	}
	cc.count++
	return true
}

func (rl *rateLimiter) prune(now time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for key, cc := range rl.clients {
		if now.After(cc.reset) {
			delete(rl.clients, key)
		}
	}
}

func startRateLimiterCleanup(rl *rateLimiter, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for now := range ticker.C {
		rl.prune(now)
	}
}

func startAuthCleanup(auth *authManager, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for now := range ticker.C {
		auth.cleanup(now)
	}
}
