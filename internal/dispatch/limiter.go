// Package dispatch runs send jobs through a worker pool. A shared limiter caps
// the request rate. The pool retries a job after a 429 reply, a 5xx reply or a
// network error.
package dispatch

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Limiter spaces requests evenly at a fixed number per minute. It also holds
// every caller back when Telegram answers 429.
type Limiter struct {
	lim *rate.Limiter

	mu    sync.Mutex
	until time.Time
}

// NewLimiter allows rpm requests per minute with no burst.
func NewLimiter(rpm int) *Limiter {
	return &Limiter{lim: rate.NewLimiter(rate.Limit(float64(rpm)/60), 1)}
}

// Wait blocks until the caller may send, or until ctx ends.
func (l *Limiter) Wait(ctx context.Context) error {
	if err := l.lim.Wait(ctx); err != nil {
		return err
	}
	for {
		l.mu.Lock()
		d := time.Until(l.until)
		l.mu.Unlock()
		if d <= 0 {
			return nil
		}
		if err := sleep(ctx, d); err != nil {
			return err
		}
	}
}

// Pause holds all callers back for d.
func (l *Limiter) Pause(d time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if t := time.Now().Add(d); t.After(l.until) {
		l.until = t
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
