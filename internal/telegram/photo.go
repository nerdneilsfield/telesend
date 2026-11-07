package telegram

import (
	"fmt"
	"image"
	_ "image/gif" // Register the decoders that DecodeConfig uses.
	_ "image/jpeg"
	_ "image/png"
	"os"
)

// Limits of sendPhoto, from the Bot API documentation.
const (
	maxPhotoBytes = 10_000_000
	maxPhotoSides = 10_000 // Width plus height.
	maxPhotoRatio = 20
)

// PhotoFits reports whether Telegram accepts the local file as a photo. When it does
// not, the reason names the limit. A format that this package cannot read (such as
// WebP) is checked by size only, and Telegram decides the rest.
func PhotoFits(path string) (ok bool, reason string) {
	info, err := os.Stat(path)
	if err != nil {
		return true, "" // A missing file is reported by the upload, not here.
	}
	if info.Size() > maxPhotoBytes {
		return false, fmt.Sprintf("it is %.1f MB and a photo can have 10 MB", float64(info.Size())/1e6)
	}
	f, err := os.Open(path)
	if err != nil {
		return true, ""
	}
	defer func() { _ = f.Close() }()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return true, ""
	}
	w, h := cfg.Width, cfg.Height
	if w+h > maxPhotoSides {
		return false, fmt.Sprintf("its width plus height is %d and a photo can have %d", w+h, maxPhotoSides)
	}
	if lo := min(w, h); lo > 0 && max(w, h) > maxPhotoRatio*lo {
		return false, fmt.Sprintf("it is %dx%d and a photo can be at most %d times longer than wide", w, h, maxPhotoRatio)
	}
	return true, ""
}
