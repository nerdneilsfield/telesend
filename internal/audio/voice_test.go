package audio

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeFormatsPassThrough(t *testing.T) {
	for _, name := range []string{"a.ogg", "a.OGG", "a.opus", "a.mp3", "a.m4a"} {
		path, cleanup, err := PrepareVoice(context.Background(), name)
		if err != nil || path != name {
			t.Errorf("%s: path=%q err=%v", name, path, err)
		}
		cleanup()
	}
}

func TestCheckRemote(t *testing.T) {
	for url, wantErr := range map[string]bool{
		"https://x.example/a.ogg":       false,
		"https://x.example/a.MP3?t=1":   false,
		"https://x.example/voice":       false,
		"https://x.example/a.wav":       true,
		"https://x.example/a.flac?x=.m": true,
	} {
		if err := CheckRemote(url); (err != nil) != wantErr {
			t.Errorf("%s: err=%v, want error=%v", url, err, wantErr)
		}
	}
}

func TestWAVIsConvertedToOpus(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	dir := t.TempDir()
	wav := filepath.Join(dir, "tone.wav")
	gen := exec.Command(ffmpeg, "-loglevel", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=1", wav)
	if out, genErr := gen.CombinedOutput(); genErr != nil {
		t.Fatalf("make wav: %v %s", genErr, out)
	}

	path, cleanup, err := PrepareVoice(context.Background(), wav)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 || !strings.HasSuffix(path, ".ogg") {
		t.Fatalf("converted file %q: %v", path, err)
	}
	probe, err := exec.Command(ffmpeg, "-hide_banner", "-i", path).CombinedOutput()
	if err == nil || !strings.Contains(string(probe), "opus") {
		t.Errorf("the output is not Opus: %s", probe)
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("cleanup must remove the temporary file")
	}
}

func TestBrokenInputGivesErrorAndNoTempFile(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp) // MkdirTemp creates the conversion directory here.
	bad := filepath.Join(t.TempDir(), "bad.wav")
	if err := os.WriteFile(bad, []byte("not audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := PrepareVoice(context.Background(), bad); err == nil {
		t.Error("a file that is not audio must fail")
	}
	left, _ := filepath.Glob(filepath.Join(tmp, "telesend-voice-*"))
	if len(left) != 0 {
		t.Errorf("a failed conversion left %v behind", left)
	}
}
