package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// call is one request that reached the mock Telegram server.
type call struct {
	method string
	form   map[string]string
	files  map[string]string // form field -> file name
}

type mock struct {
	*httptest.Server
	mu    sync.Mutex
	calls []call
	// fail429 makes the first sendMessage answer 429 with retry_after 1.
	fail429 bool
}

func newMock(t *testing.T) *mock {
	t.Helper()
	m := &mock{}
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if r.ContentLength <= 0 {
			http.Error(w, "length required", http.StatusLengthRequired)
			return
		}
		c := call{method: r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:], form: map[string]string{}, files: map[string]string{}}
		for k, v := range r.MultipartForm.Value {
			c.form[k] = v[0]
		}
		for k, v := range r.MultipartForm.File {
			c.files[k] = v[0].Filename
		}
		m.mu.Lock()
		first := len(m.calls) == 0
		m.calls = append(m.calls, c)
		m.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if m.fail429 && first {
			w.WriteHeader(429)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 1","parameters":{"retry_after":1}}`))
			return
		}
		if c.method == "sendMediaGroup" {
			_, _ = w.Write([]byte(`{"ok":true,"result":[{"message_id":10},{"message_id":11}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":5}}`))
	}))
	t.Cleanup(m.Close)
	return m
}

func run(t *testing.T, env map[string]string, args ...string) (string, error) {
	t.Helper()
	for _, k := range []string{"TELESEND_BOT_TOKEN", "TELESEND_CHAT_ID", "TELESEND_BASE_URL"} {
		t.Setenv(k, env[k])
	}
	var out bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestTextFlagsOverrideEnv(t *testing.T) {
	m := newMock(t)
	env := map[string]string{"TELESEND_BOT_TOKEN": "tok", "TELESEND_CHAT_ID": "111", "TELESEND_BASE_URL": "http://127.0.0.1:1"}
	out, err := run(t, env, "text", "hello", "--to", "222", "--base-url", m.URL, "--rpm", "600")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.calls) != 1 || m.calls[0].form["chat_id"] != "222" || m.calls[0].form["text"] != "hello" {
		t.Fatalf("calls: %+v", m.calls)
	}
	if !strings.Contains(out, "message 5") {
		t.Errorf("output: %q", out)
	}
}

func TestMarkdownIsEscaped(t *testing.T) {
	m := newMock(t)
	env := map[string]string{"TELESEND_BOT_TOKEN": "tok", "TELESEND_CHAT_ID": "1", "TELESEND_BASE_URL": m.URL}
	if _, err := run(t, env, "text", "-m", "**done** v1.2!", "--rpm", "600"); err != nil {
		t.Fatal(err)
	}
	f := m.calls[0].form
	if f["text"] != `*done* v1\.2\!` || f["parse_mode"] != "MarkdownV2" {
		t.Errorf("form: %+v", f)
	}
}

func TestPhotosBuildMediaGroup(t *testing.T) {
	m := newMock(t)
	dir := t.TempDir()
	var paths []string
	for _, n := range []string{"a.jpg", "b.jpg"} {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	env := map[string]string{"TELESEND_BOT_TOKEN": "tok", "TELESEND_CHAT_ID": "1", "TELESEND_BASE_URL": m.URL}
	args := append([]string{"photo"}, paths...)
	args = append(args, "-c", "Trip", "--rpm", "600")
	if _, err := run(t, env, args...); err != nil {
		t.Fatal(err)
	}
	c := m.calls[0]
	if c.method != "sendMediaGroup" || c.files["file0"] != "a.jpg" || c.files["file1"] != "b.jpg" {
		t.Fatalf("call: %+v", c)
	}
	var items []map[string]string
	if err := json.Unmarshal([]byte(c.form["media"]), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0]["media"] != "attach://file0" || items[0]["caption"] != "Trip" || items[1]["caption"] != "" {
		t.Errorf("media: %v", items)
	}
}

func TestSinglePhotoFileAndVoice(t *testing.T) {
	m := newMock(t)
	dir := t.TempDir()
	jpg, pdf, ogg := filepath.Join(dir, "a.jpg"), filepath.Join(dir, "r.pdf"), filepath.Join(dir, "v.ogg")
	for _, p := range []string{jpg, pdf, ogg} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	env := map[string]string{"TELESEND_BOT_TOKEN": "tok", "TELESEND_CHAT_ID": "1", "TELESEND_BASE_URL": m.URL}
	for _, args := range [][]string{{"photo", jpg}, {"file", pdf}, {"voice", ogg}, {"photo", "https://example.com/x.png"}} {
		if _, err := run(t, env, append(args, "--rpm", "600")...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	want := []string{"sendPhoto", "sendDocument", "sendVoice", "sendPhoto"}
	for i, w := range want {
		if m.calls[i].method != w {
			t.Errorf("call %d: got %s, want %s", i, m.calls[i].method, w)
		}
	}
	if m.calls[3].form["photo"] != "https://example.com/x.png" || len(m.calls[3].files) != 0 {
		t.Errorf("a URL must be passed through: %+v", m.calls[3])
	}
}

func TestRetryOn429(t *testing.T) {
	m := newMock(t)
	m.fail429 = true
	env := map[string]string{"TELESEND_BOT_TOKEN": "tok", "TELESEND_CHAT_ID": "1", "TELESEND_BASE_URL": m.URL}
	if _, err := run(t, env, "text", "hi", "--rpm", "600"); err != nil {
		t.Fatal(err)
	}
	if len(m.calls) != 2 {
		t.Fatalf("want 1 retry, got %d calls", len(m.calls))
	}
}

func TestInvalidInputShowsUsage(t *testing.T) {
	_, err := run(t, nil, "photo")
	if err == nil || !strings.Contains(err.Error(), "Usage:") || !strings.Contains(err.Error(), "Examples:") {
		t.Errorf("missing-arg error lacks usage: %v", err)
	}
	_, err = run(t, nil, "text", "hi")
	if err == nil || !strings.Contains(err.Error(), "TELESEND_BOT_TOKEN") || !strings.Contains(err.Error(), "Usage:") {
		t.Errorf("missing-token error: %v", err)
	}
	_, err = run(t, nil, "text", "--nope")
	if err == nil || !strings.Contains(err.Error(), "Usage:") {
		t.Errorf("bad-flag error: %v", err)
	}
}

func TestTokenNeverAppearsInErrors(t *testing.T) {
	env := map[string]string{"TELESEND_BOT_TOKEN": "SECRET:tok", "TELESEND_CHAT_ID": "1", "TELESEND_BASE_URL": "http://127.0.0.1:1"}
	_, err := run(t, env, "text", "hi", "--retries", "0", "--rpm", "600")
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Errorf("error must exist and hide the token: %v", err)
	}
}
