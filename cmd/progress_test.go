package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nerdneilsfield/telesend/internal/dispatch"
)

func TestProgressCountsBytesAndResetsOnRetry(t *testing.T) {
	var buf bytes.Buffer
	jobs := []dispatch.Job{{Name: "a", Size: 1000}, {Name: "b", Size: 3000}}
	p := newProgress(jobs, true, &buf)

	p.onProgress(0, 600)
	p.onProgress(1, 1000)
	if got := p.sum(); got != 1600 {
		t.Fatalf("sum %d, want 1600", got)
	}
	// A retry starts again at 0. The total must go back, not count the bytes twice.
	p.onProgress(1, 0)
	if got := p.sum(); got != 600 {
		t.Fatalf("after a retry reset, sum %d, want 600", got)
	}
	p.onDone(0, dispatch.Result{Name: "a"})
	p.onDone(1, dispatch.Result{Name: "b"})
	if got := p.sum(); got != 4000 {
		t.Fatalf("after both jobs, sum %d, want 4000", got)
	}
	p.finish()
	if out := buf.String(); !strings.Contains(out, "100%") || !strings.Contains(out, "2/2 sent") {
		t.Errorf("the bar did not show completion: %q", out)
	}
}

func TestProgressCountsJobsForText(t *testing.T) {
	var buf bytes.Buffer
	p := newProgress([]dispatch.Job{{Name: "t1"}, {Name: "t2"}}, true, &buf)
	if p.bytesMode {
		t.Fatal("jobs without files must count jobs, not bytes")
	}
	p.onDone(0, dispatch.Result{Name: "t1"})
	p.withCleared(func() {})
	p.onDone(1, dispatch.Result{Name: "t2"})
	p.finish()
	if !strings.Contains(buf.String(), "2/2") {
		t.Errorf("bar output: %q", buf.String())
	}
}

func TestProgressIsOffWhenDisabled(t *testing.T) {
	if p := newProgress([]dispatch.Job{{Name: "a"}}, false, nil); p != nil {
		t.Error("a disabled bar must be nil")
	}
	var p *progress
	p.onProgress(0, 1) // A nil bar must accept every call.
	p.onDone(0, dispatch.Result{})
	p.withCleared(func() {})
	p.finish()
}

func TestVersionFlag(t *testing.T) {
	out, err := run(t, nil, "--version")
	if err != nil || !strings.Contains(out, "telesend version") {
		t.Errorf("out=%q err=%v", out, err)
	}
}
