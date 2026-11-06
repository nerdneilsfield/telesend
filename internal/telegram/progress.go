package telegram

import (
	"context"
	"io"
)

type progressKey struct{}

// WithProgress returns a context whose uploads report the file bytes that they
// wrote so far. fn gets the running total of one request, and it gets 0 when a
// request starts. fn runs on the upload goroutine, so it must return fast.
func WithProgress(ctx context.Context, fn func(sent int64)) context.Context {
	return context.WithValue(ctx, progressKey{}, fn)
}

func progressFrom(ctx context.Context) func(int64) {
	fn, _ := ctx.Value(progressKey{}).(func(int64))
	return fn
}

// countReader reports the bytes that pass through it.
type countReader struct {
	r  io.Reader
	n  int64
	fn func(int64)
}

func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		c.n += int64(n)
		c.fn(c.n)
	}
	return n, err
}
