package cmd

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/schollz/progressbar/v3"
	"golang.org/x/term"

	"github.com/dengqi/telesend/internal/dispatch"
)

// progress draws one bar for the whole run on stderr. It counts uploaded bytes
// when the jobs upload files, and finished jobs otherwise. The bar shows the
// speed and the time left. A nil *progress does nothing.
type progress struct {
	mu        sync.Mutex
	bar       *progressbar.ProgressBar
	bytesMode bool
	sizes     []int64
	sent      []int64 // Bytes sent by the current attempt of each job.
	done      int
}

// stderrIsTerminal reports whether the bar can draw on stderr.
func stderrIsTerminal() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
}

// newProgress returns nil when enabled is false or when there is nothing to send.
func newProgress(jobs []dispatch.Job, enabled bool) *progress {
	if !enabled || len(jobs) == 0 {
		return nil
	}
	p := &progress{sizes: make([]int64, len(jobs)), sent: make([]int64, len(jobs))}
	var total int64
	for i, j := range jobs {
		p.sizes[i] = j.Size
		total += j.Size
	}
	opts := []progressbar.Option{
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetWidth(24),
		progressbar.OptionThrottle(100 * time.Millisecond),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionSetElapsedTime(true),
		progressbar.OptionSetDescription(fmt.Sprintf("0/%d sent", len(jobs))),
	}
	if total > 0 {
		p.bytesMode = true
		opts = append(opts, progressbar.OptionShowBytes(true))
	} else {
		total = int64(len(jobs))
		opts = append(opts, progressbar.OptionShowCount())
	}
	p.bar = progressbar.NewOptions64(total, opts...)
	return p
}

// onProgress records the bytes that the current attempt of job i has sent.
func (p *progress) onProgress(i int, sent int64) {
	if p == nil || !p.bytesMode {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sent[i] = sent
	_ = p.bar.Set64(p.sum())
}

// onDone records the final result of job i.
func (p *progress) onDone(i int, r dispatch.Result) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if r.Err == nil && !r.Skipped {
		p.done++
		p.bar.Describe(fmt.Sprintf("%d/%d sent", p.done, len(p.sizes)))
		if p.bytesMode {
			p.sent[i] = p.sizes[i]
			_ = p.bar.Set64(p.sum())
		} else {
			_ = p.bar.Add(1)
		}
	}
}

func (p *progress) sum() int64 {
	var n int64
	for _, s := range p.sent {
		n += s
	}
	return n
}

// finish ends the bar line, so later output starts on a new line.
func (p *progress) finish() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	_ = p.bar.Finish()
	fmt.Fprintln(os.Stderr)
}

// pause clears the bar, so a line can print in its place.
func (p *progress) pause() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	_ = p.bar.Clear()
}

// resume draws the bar again after a line was printed.
func (p *progress) resume() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	_ = p.bar.RenderBlank()
}
