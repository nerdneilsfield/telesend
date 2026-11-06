// Package audio prepares a voice file for Telegram.
package audio

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Telegram plays these formats as a voice message without conversion.
var native = map[string]bool{".ogg": true, ".oga": true, ".opus": true, ".mp3": true, ".m4a": true}

// CheckRemote rejects a URL whose file extension names a format that Telegram does not
// play as a voice message. telesend converts local files only.
func CheckRemote(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	ext := strings.ToLower(filepath.Ext(u.Path))
	if ext == "" || native[ext] {
		return nil
	}
	return fmt.Errorf("%s needs conversion to OGG/Opus, and telesend converts local files only; download it and pass the local path", rawURL)
}

// PrepareVoice returns a path that sendVoice accepts. A file in another format
// goes through ffmpeg to OGG/Opus. The returned cleanup removes any temporary file.
func PrepareVoice(ctx context.Context, path string) (string, func(), error) {
	noop := func() {}
	if native[strings.ToLower(filepath.Ext(path))] {
		return path, noop, nil
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", noop, fmt.Errorf("%s needs conversion to OGG/Opus, but ffmpeg is not installed; install ffmpeg or pass an .ogg, .mp3 or .m4a file", filepath.Base(path))
	}
	dir, err := os.MkdirTemp("", "telesend-voice-")
	if err != nil {
		return "", noop, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	out := filepath.Join(dir, "voice.ogg")
	cmd := exec.CommandContext(ctx, ffmpeg, "-nostdin", "-loglevel", "error", "-y",
		"-i", path, "-vn", "-c:a", "libopus", "-b:a", "32k", "-ar", "48000", "-ac", "1", out)
	if msg, err := cmd.CombinedOutput(); err != nil {
		cleanup()
		return "", noop, fmt.Errorf("ffmpeg failed on %s: %s", filepath.Base(path), strings.TrimSpace(string(msg)))
	}
	return out, cleanup, nil
}
