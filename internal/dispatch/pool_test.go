package dispatch

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dengqi/telesend/internal/telegram"
)

func TestRunHonorsConcurrencyAndRPM(t *testing.T) {
	const rpm = 1200 // one request every 50 ms
	var inflight, peak atomic.Int32
	var mu sync.Mutex
	var starts []time.Time

	jobs := make([]Job, 8)
	for i := range jobs {
		jobs[i] = Job{Name: "j", Run: func(context.Context) ([]int, error) {
			n := inflight.Add(1)
			for {
				p := peak.Load()
				if n <= p || peak.CompareAndSwap(p, n) {
					break
				}
			}
			mu.Lock()
			starts = append(starts, time.Now())
			mu.Unlock()
			time.Sleep(120 * time.Millisecond)
			inflight.Add(-1)
			return []int{1}, nil
		}}
	}
	before := runtime.NumGoroutine()
	res := Run(context.Background(), jobs, Options{Concurrency: 3, RPM: rpm, Timeout: time.Second, BackoffBase: time.Millisecond})

	for _, r := range res {
		if r.Err != nil || r.Skipped {
			t.Fatalf("unexpected result: %+v", r)
		}
	}
	if peak.Load() > 3 {
		t.Errorf("peak concurrency %d exceeds 3", peak.Load())
	}
	// Start times are recorded after the limiter grants a slot, so sort-free pairwise gaps hold.
	mu.Lock()
	defer mu.Unlock()
	for i := 1; i < len(starts); i++ {
		if gap := starts[i].Sub(starts[i-1]); gap < 40*time.Millisecond {
			t.Errorf("request %d started %v after the previous one; want >= ~50ms", i, gap)
		}
	}
	time.Sleep(50 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before {
		t.Errorf("goroutines leaked: %d before, %d after", before, after)
	}
}

func TestRetryAfter429(t *testing.T) {
	var calls atomic.Int32
	var first time.Time
	job := Job{Name: "j", Run: func(context.Context) ([]int, error) {
		if calls.Add(1) == 1 {
			first = time.Now()
			return nil, &telegram.APIError{Code: 429, Description: "Too Many Requests", RetryAfter: time.Second}
		}
		if d := time.Since(first); d < time.Second {
			t.Errorf("retried after %v; Telegram asked for 1s", d)
		}
		return []int{7}, nil
	}}
	res := Run(context.Background(), []Job{job}, Options{Concurrency: 1, RPM: 600, Retries: 2, Timeout: 5 * time.Second, BackoffBase: time.Millisecond})
	if res[0].Err != nil || res[0].Attempts != 2 {
		t.Fatalf("got %+v", res[0])
	}
}

func TestPermanentErrorIsNotRetriedAndStopsRun(t *testing.T) {
	var calls atomic.Int32
	bad := Job{Name: "bad", Run: func(context.Context) ([]int, error) {
		calls.Add(1)
		return nil, &telegram.APIError{Code: 400, Description: "Bad Request"}
	}}
	next := Job{Name: "next", Run: func(context.Context) ([]int, error) {
		t.Error("job after a failure must not start")
		return nil, nil
	}}
	res := Run(context.Background(), []Job{bad, next}, Options{Concurrency: 1, RPM: 600, Retries: 3, Timeout: time.Second, BackoffBase: time.Millisecond})
	if calls.Load() != 1 || res[0].Err == nil || !res[1].Skipped {
		t.Fatalf("calls=%d results=%+v", calls.Load(), res)
	}
}

func TestServerErrorRetriesThenGivesUp(t *testing.T) {
	var calls atomic.Int32
	job := Job{Name: "j", Run: func(context.Context) ([]int, error) {
		calls.Add(1)
		return nil, &telegram.APIError{Code: 502, Description: "Bad Gateway"}
	}}
	res := Run(context.Background(), []Job{job}, Options{Concurrency: 1, RPM: 6000, Retries: 2, Timeout: time.Second, BackoffBase: time.Millisecond})
	var api *telegram.APIError
	if calls.Load() != 3 || !errors.As(res[0].Err, &api) {
		t.Fatalf("calls=%d err=%v", calls.Load(), res[0].Err)
	}
}

func TestWeightUsesRPMSlots(t *testing.T) {
	// 1200 rpm gives one slot per 50 ms. A job of weight 4 waits for 3 more slots after the first.
	job := Job{Name: "album", Weight: 4, Run: func(context.Context) ([]int, error) { return nil, nil }}
	began := time.Now()
	Run(context.Background(), []Job{job}, Options{Concurrency: 1, RPM: 1200, Timeout: time.Second, BackoffBase: time.Millisecond})
	if d := time.Since(began); d < 140*time.Millisecond {
		t.Errorf("weight 4 finished in %v; want >= ~150ms", d)
	}
}
