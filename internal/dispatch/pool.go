package dispatch

import (
	"context"
	"errors"
	"io/fs"
	"math/rand/v2"
	"net"
	"sync"
	"time"

	"github.com/dengqi/telesend/internal/telegram"
)

// Job is one API request.
type Job struct {
	Name string
	// Weight is the number of rpm slots the job uses. An album of n items uses n.
	// Zero means 1.
	Weight int
	// Run makes one attempt. It must open its inputs again on every call.
	Run func(ctx context.Context) ([]int, error)
}

// Result is the outcome of one Job. Err is nil on success.
// Skipped is true when an earlier failure stopped the run before this job started.
type Result struct {
	Name     string
	IDs      []int
	Err      error
	Attempts int
	Skipped  bool
}

// Options configure Run.
type Options struct {
	Concurrency int
	RPM         int
	Retries     int
	// Timeout limits one attempt.
	Timeout time.Duration
	// BackoffBase is the first wait after a network error or 5xx reply. It doubles per retry.
	BackoffBase time.Duration
	// OnAttempt is called after each attempt with its duration and error. It may be nil.
	OnAttempt func(job string, attempt int, took time.Duration, err error)
	// OnRetry is called before each wait. It may be nil.
	OnRetry func(job string, attempt int, wait time.Duration, err error)
}

// Run executes jobs with at most Concurrency requests in flight and at most RPM
// requests per minute. It starts jobs in order. After the first failure it
// starts no more jobs, and it marks the rest as skipped. Run returns one Result per Job.
func Run(ctx context.Context, jobs []Job, o Options) []Result {
	results := make([]Result, len(jobs))
	for i, j := range jobs {
		results[i] = Result{Name: j.Name, Skipped: true}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	lim := NewLimiter(o.RPM)
	queue := make(chan int)
	var wg sync.WaitGroup
	for range min(o.Concurrency, len(jobs)) {
		wg.Go(func() {
			for i := range queue {
				results[i] = runJob(ctx, jobs[i], lim, o)
				if results[i].Attempts == 0 && errors.Is(results[i].Err, context.Canceled) {
					// The run stopped before this job made a request.
					results[i] = Result{Name: jobs[i].Name, Skipped: true}
				} else if results[i].Err != nil {
					cancel()
				}
			}
		})
	}
feed:
	for i := range jobs {
		select {
		case queue <- i:
		case <-ctx.Done():
			break feed
		}
	}
	close(queue)
	wg.Wait()
	return results
}

func runJob(ctx context.Context, j Job, lim *Limiter, o Options) Result {
	res := Result{Name: j.Name}
	for attempt := 0; ; attempt++ {
		for range max(j.Weight, 1) {
			if err := lim.Wait(ctx); err != nil {
				res.Err = err
				return res
			}
		}
		res.Attempts++
		actx, cancel := context.WithTimeout(ctx, o.Timeout)
		began := time.Now()
		ids, err := j.Run(actx)
		cancel()
		if o.OnAttempt != nil {
			o.OnAttempt(j.Name, res.Attempts, time.Since(began), err)
		}
		if err == nil {
			res.IDs, res.Err = ids, nil
			return res
		}
		res.Err = err
		wait, retry := retryPolicy(err, attempt, o.BackoffBase)
		if !retry || attempt >= o.Retries || ctx.Err() != nil {
			return res
		}
		if wait > 0 && isRateLimit(err) {
			lim.Pause(wait)
		}
		if o.OnRetry != nil {
			o.OnRetry(j.Name, attempt+1, wait, err)
		}
		if isRateLimit(err) {
			continue // Wait pauses all workers, so this worker needs no sleep of its own.
		}
		if err := sleep(ctx, wait); err != nil {
			return res
		}
	}
}

func isRateLimit(err error) bool {
	var api *telegram.APIError
	return errors.As(err, &api) && api.Code == 429
}

// retryPolicy classifies err and returns the wait before the next attempt.
func retryPolicy(err error, attempt int, base time.Duration) (time.Duration, bool) {
	var api *telegram.APIError
	if errors.As(err, &api) {
		if !api.Retryable() {
			return 0, false
		}
		if api.Code == 429 && api.RetryAfter > 0 {
			return api.RetryAfter, true
		}
		return backoff(attempt, base), true
	}
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return 0, false // A local file cannot be read; waiting does not help.
	}
	var ne net.Error
	if errors.As(err, &ne) || errors.Is(err, context.DeadlineExceeded) {
		return backoff(attempt, base), true
	}
	return 0, false
}

// backoff doubles base per attempt and adds up to 25% jitter.
func backoff(attempt int, base time.Duration) time.Duration {
	d := base << attempt
	return d + time.Duration(rand.Int64N(int64(d)/4+1))
}
