// Package dispatch runs send jobs through a worker pool. A shared limiter caps
// the request rate. The pool retries a job after a 429 reply, a 5xx reply or a
// network error.
package dispatch

import (
	"context"
	"sync"
	"time"
)

// Limiter keeps the sends of all workers at or under a number of messages per
// minute. It hands out start times. A send of weight n starts at its time and
// holds back the next n slots, so a burst of n messages is paid for after it,
// not before it.
type Limiter struct {
	interval time.Duration

	mu    sync.Mutex
	next  time.Time // The earliest start of the next send.
	until time.Time // No send starts before this time (a 429 pause).
}

// NewLimiter allows rpm messages per minute.
func NewLimiter(rpm int) *Limiter {
	return &Limiter{interval: time.Minute / time.Duration(rpm)}
}

// Wait blocks until a send of the given weight may start, or until ctx ends.
func (l *Limiter) Wait(ctx context.Context, weight int) error {
	for {
		start := l.reserve(weight)
		if err := sleep(ctx, time.Until(start)); err != nil {
			return err
		}
		// A 429 can arrive while this caller sleeps. Then the slot is void.
		l.mu.Lock()
		paused := time.Now().Before(l.until)
		l.mu.Unlock()
		if !paused {
			return nil
		}
	}
}

// reserve claims the next start time and moves the slot clock by weight slots.
func (l *Limiter) reserve(weight int) time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	start := latest(time.Now(), l.next, l.until)
	l.next = start.Add(time.Duration(max(weight, 1)) * l.interval)
	return start
}

// Pause holds all senders back for d.
func (l *Limiter) Pause(d time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if t := time.Now().Add(d); t.After(l.until) {
		l.until = t
	}
}

func latest(ts ...time.Time) time.Time {
	t := ts[0]
	for _, o := range ts[1:] {
		if o.After(t) {
			t = o
		}
	}
	return t
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
