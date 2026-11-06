package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUploadReportsFileBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = r.MultipartReader() // The body is read by the handler below.
		buf := make([]byte, 1<<16)
		for {
			if _, err := r.Body.Read(buf); err != nil {
				break
			}
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.bin"), filepath.Join(dir, "b.bin")
	if err := os.WriteFile(a, make([]byte, 300_000), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, make([]byte, 200_000), 0o600); err != nil {
		t.Fatal(err)
	}

	var seen []int64
	ctx := WithProgress(context.Background(), func(n int64) { seen = append(seen, n) })
	c := &Client{BaseURL: srv.URL, Token: "t", HTTP: srv.Client()}
	if _, err := c.SendMediaGroup(ctx, []Media{{Path: a, Kind: KindDocument}, {Path: b, Kind: KindDocument}}, Options{ChatID: "1"}); err != nil {
		t.Fatal(err)
	}
	if len(seen) < 3 || seen[0] != 0 {
		t.Fatalf("progress must start at 0 and report steps: %v", seen)
	}
	for i := 1; i < len(seen); i++ {
		if seen[i] < seen[i-1] {
			t.Fatalf("progress went backwards: %v", seen)
		}
	}
	if last := seen[len(seen)-1]; last != 500_000 {
		t.Errorf("last report %d, want 500000 file bytes", last)
	}
}
