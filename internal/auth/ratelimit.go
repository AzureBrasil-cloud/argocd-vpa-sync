package auth

import (
	"sync"
	"time"
)

// LoginLimiter throttles repeated failed logins from the same client: the
// first freeFailures failures are free, after which each further attempt
// must wait an exponentially growing delay (capped at maxDelay). A
// successful login clears the client's record.
type LoginLimiter struct {
	mu      sync.Mutex
	clients map[string]*loginRecord
	now     func() time.Time
}

type loginRecord struct {
	failures    int
	lockedUntil time.Time
	lastSeen    time.Time
}

const (
	freeFailures = 5
	baseDelay    = time.Second
	maxDelay     = time.Minute
	// forgetAfter drops a client's record once it has been quiet this long.
	forgetAfter = 15 * time.Minute
)

// NewLoginLimiter returns an empty LoginLimiter.
func NewLoginLimiter() *LoginLimiter {
	return &LoginLimiter{clients: map[string]*loginRecord{}, now: time.Now}
}

// Allow reports whether client may attempt a login now and, if not, how
// long it must wait.
func (l *LoginLimiter) Allow(client string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweep(now)
	rec, ok := l.clients[client]
	if !ok || !now.Before(rec.lockedUntil) {
		return true, 0
	}
	return false, rec.lockedUntil.Sub(now)
}

// Failure records a failed login from client.
func (l *LoginLimiter) Failure(client string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	rec, ok := l.clients[client]
	if !ok {
		rec = &loginRecord{}
		l.clients[client] = rec
	}
	rec.failures++
	rec.lastSeen = now
	if rec.failures >= freeFailures {
		delay := baseDelay << min(rec.failures-freeFailures, 6)
		rec.lockedUntil = now.Add(min(delay, maxDelay))
	}
}

// Success clears client's failure record.
func (l *LoginLimiter) Success(client string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.clients, client)
}

func (l *LoginLimiter) sweep(now time.Time) {
	for client, rec := range l.clients {
		if now.Sub(rec.lastSeen) > forgetAfter {
			delete(l.clients, client)
		}
	}
}
