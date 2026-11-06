// Package dispatch runs send jobs through a worker pool. A shared limiter caps
// the request rate. The pool retries a job after a 429 reply, a 5xx reply or a
// network error.
package dispatch

import (
	"context"
	"sync"
	"time"
)

// Limiter keeps the sends of all workers at or under rpm messages in any
// window of one minute. It hands out start times by two rules:
//
//  1. Spacing: a send of weight n starts at its time and closes the next n
//     slots of 60 s ÷ rpm each. Single messages go out evenly.
//  2. Window: a send starts only when the weights of the sends of the last
//     minute plus its own weight stay at or under rpm.
type Limiter struct {
	rpm      int
	window   time.Duration
	interval time.Duration

	mu      sync.Mutex
	next    time.Time // The earliest start of the next send.
	until   time.Time // No send starts before this time (a 429 pause).
	history []send    // Reserved sends, in start order.
}

type send struct {
	at     time.Time
	weight int
}

// NewLimiter allows rpm messages per minute.
func NewLimiter(rpm int) *Limiter {
	return newLimiter(rpm, time.Minute)
}

func newLimiter(rpm int, window time.Duration) *Limiter {
	return &Limiter{rpm: rpm, window: window, interval: window / time.Duration(rpm)}
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

// reserve claims the next start time for a send of the given weight.
func (l *Limiter) reserve(weight int) time.Time {
	// A weight above rpm could never fit one window. Cap it, so the send still goes out.
	w := min(max(weight, 1), l.rpm)

	l.mu.Lock()
	defer l.mu.Unlock()
	start := latest(time.Now(), l.next, l.until)
	for {
		total, first := w, -1
		for i, h := range l.history {
			if h.at.Add(l.window).After(start) {
				if first < 0 {
					first = i
				}
				total += h.weight
			}
		}
		if total <= l.rpm {
			break
		}
		start = l.history[first].at.Add(l.window)
	}
	l.next = start.Add(time.Duration(w) * l.interval)
	// Drop the sends that left the window. Starts only move forward.
	keep := l.history[:0]
	for _, h := range l.history {
		if h.at.Add(l.window).After(start) {
			keep = append(keep, h)
		}
	}
	l.history = append(keep, send{at: start, weight: w})
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
