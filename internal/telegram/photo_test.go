package telegram

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePNG(t *testing.T, w, h int) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "x.png")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, image.NewGray(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return p
}

// The limits are in the sendPhoto section of the Bot API documentation.
func TestPhotoFits(t *testing.T) {
	cases := []struct {
		name         string
		w, h         int
		ok           bool
		reasonHasStr string
	}{
		{"normal", 1920, 1080, true, ""},
		{"sides total exactly 10000", 9000, 1000, true, ""},
		{"sides total over 10000", 9001, 1000, false, "width plus height"},
		{"ratio exactly 20", 2000, 100, true, ""},
		{"ratio over 20", 2100, 100, false, "times longer"},
	}
	for _, c := range cases {
		ok, reason := PhotoFits(writePNG(t, c.w, c.h))
		if ok != c.ok || !strings.Contains(reason, c.reasonHasStr) {
			t.Errorf("%s: ok=%v reason=%q", c.name, ok, reason)
		}
	}
}

func TestPhotoFitsBySizeAndUnknownFormat(t *testing.T) {
	big := filepath.Join(t.TempDir(), "big.webp") // WebP is not decoded; the size check still applies.
	f, _ := os.Create(big)
	_ = f.Truncate(10_000_001)
	_ = f.Close()
	if ok, reason := PhotoFits(big); ok || !strings.Contains(reason, "10 MB") {
		t.Errorf("a file over 10 MB: ok=%v reason=%q", ok, reason)
	}
	small := filepath.Join(t.TempDir(), "small.webp")
	if err := os.WriteFile(small, []byte("not decodable"), 0o600); err != nil {
		t.Fatal(err)
	}
	if ok, _ := PhotoFits(small); !ok {
		t.Error("an unknown format under 10 MB is left to Telegram")
	}
	if ok, _ := PhotoFits(filepath.Join(t.TempDir(), "missing.png")); !ok {
		t.Error("a missing file is reported by the upload, not by the photo check")
	}
}
