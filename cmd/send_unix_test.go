//go:build unix

package cmd

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestFIFOIsRejectedWithoutHanging(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "in.fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skip(err)
	}
	m := newMock(t)
	done := make(chan error, 1)
	go func() {
		_, err := run(t, okEnv(m), "file", fifo, "--timeout", "1s")
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Errorf("want a regular-file error, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("telesend hung on a FIFO")
	}
}
