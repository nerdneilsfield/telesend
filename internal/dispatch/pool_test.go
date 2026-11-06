package dispatch

import (
	"context"
	"errors"
	"runtime"
	"slices"
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

func TestWeightIsPaidAfterTheSend(t *testing.T) {
	// 1200 rpm gives one slot per 50 ms. A job of weight 4 starts at once and closes 4 slots.
	var mu sync.Mutex
	var starts []time.Time
	run := func(context.Context) ([]int, error) {
		mu.Lock()
		starts = append(starts, time.Now())
		mu.Unlock()
		return nil, nil
	}
	jobs := []Job{{Name: "album", Weight: 4, Run: run}, {Name: "next", Run: run}}
	began := time.Now()
	Run(context.Background(), jobs, Options{Concurrency: 1, RPM: 1200, Timeout: time.Second, BackoffBase: time.Millisecond})
	if d := starts[0].Sub(began); d > 30*time.Millisecond {
		t.Errorf("the album waited %v before its send; want it to start at once", d)
	}
	if gap := starts[1].Sub(starts[0]); gap < 180*time.Millisecond {
		t.Errorf("next send came %v after a weight-4 album; want >= ~200ms", gap)
	}
}

func TestEverySendClosesItsWeightInSlots(t *testing.T) {
	// 1200 rpm is one slot per 50 ms. With three workers, each send must still start at
	// least weight x 50 ms after the previous start. Over a minute that keeps
	// the total at or under rpm, because the burst of an album is paid for after it.
	const slot = 50 * time.Millisecond
	var mu sync.Mutex
	type send struct {
		at time.Time
		w  int
	}
	var sends []send
	weights := []int{3, 1, 4, 2, 3, 1}
	jobs := make([]Job, len(weights))
	for i, w := range weights {
		jobs[i] = Job{Name: "j", Weight: w, Run: func(context.Context) ([]int, error) {
			mu.Lock()
			sends = append(sends, send{time.Now(), w})
			mu.Unlock()
			return nil, nil
		}}
	}
	Run(context.Background(), jobs, Options{Concurrency: 3, RPM: 1200, Timeout: time.Second, BackoffBase: time.Millisecond})
	slices.SortFunc(sends, func(a, b send) int { return a.at.Compare(b.at) })
	for i := 1; i < len(sends); i++ {
		want := time.Duration(sends[i-1].w) * slot
		if gap := sends[i].at.Sub(sends[i-1].at); gap < want-10*time.Millisecond {
			t.Errorf("send %d started %v after a weight-%d send; want >= %v", i, gap, sends[i-1].w, want)
		}
	}
}

func TestAfter429PauseSendersStayEvenlySpaced(t *testing.T) {
	lim := NewLimiter(600) // 100 ms per slot
	lim.Pause(200 * time.Millisecond)
	var mu sync.Mutex
	var times []time.Time
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() {
			if err := lim.Wait(context.Background(), 1); err != nil {
				t.Error(err)
			}
			mu.Lock()
			times = append(times, time.Now())
			mu.Unlock()
		})
	}
	wg.Wait()
	slices.SortFunc(times, func(a, b time.Time) int { return a.Compare(b) })
	for i := 1; i < len(times); i++ {
		if gap := times[i].Sub(times[i-1]); gap < 80*time.Millisecond {
			t.Errorf("senders %d and %d are only %v apart after a pause; want >= ~100ms", i-1, i, gap)
		}
	}
}
