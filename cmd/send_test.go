package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nerdneilsfield/telesend/internal/config"
	"github.com/nerdneilsfield/telesend/internal/dispatch"
	"github.com/nerdneilsfield/telesend/internal/telegram"
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
	// failBody, when set, is the JSON error body with status 400 for every request.
	failBody string
	// truncateFirst makes the first reply stop in the middle of the body.
	truncateFirst bool
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
		if m.truncateFirst && first {
			conn, buf, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_, _ = buf.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 100\r\nContent-Type: application/json\r\n\r\n{\"ok\":")
				_ = buf.Flush()
				_ = conn.Close()
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if m.failBody != "" {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(m.failBody))
			return
		}
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
	for _, k := range []string{"TELESEND_BOT_TOKEN", "TELESEND_CHAT_ID", "TELESEND_BASE_URL", "TELESEND_ALBUM_SIZE", "TELESEND_TEXT_LIMIT"} {
		t.Setenv(k, env[k])
	}
	var out bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := executeRoot(context.Background(), root)
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

func touch(t *testing.T, dir string, names ...string) []string {
	t.Helper()
	var paths []string
	for _, n := range names {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	return paths
}

func okEnv(m *mock) map[string]string {
	return map[string]string{"TELESEND_BOT_TOKEN": "tok", "TELESEND_CHAT_ID": "1", "TELESEND_BASE_URL": m.URL}
}

func mediaTypes(t *testing.T, c call) (types []string, captions []string) {
	t.Helper()
	var items []map[string]any
	if err := json.Unmarshal([]byte(c.form["media"]), &items); err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		types = append(types, it["type"].(string))
		c, _ := it["caption"].(string)
		captions = append(captions, c)
	}
	return types, captions
}

func TestVideoAndMixedAlbum(t *testing.T) {
	m := newMock(t)
	files := touch(t, t.TempDir(), "a.jpg", "b.MP4", "c.png", "d.txt")

	if _, err := run(t, okEnv(m), "video", files[1], "--rpm", "1800"); err != nil {
		t.Fatal(err)
	}
	if c := m.calls[0]; c.method != "sendVideo" || c.form["supports_streaming"] != "true" || c.files["video"] != "b.MP4" {
		t.Errorf("sendVideo call: %+v", c)
	}

	if _, err := run(t, okEnv(m), "media", files[0], files[1], files[2], "-c", "Mixed", "--rpm", "1800"); err != nil {
		t.Fatal(err)
	}
	types, caps := mediaTypes(t, m.calls[1])
	if strings.Join(types, ",") != "photo,video,photo" || caps[0] != "Mixed" || caps[1] != "" {
		t.Errorf("types=%v captions=%v", types, caps)
	}

	_, err := run(t, okEnv(m), "media", files[3], "--rpm", "1800")
	if err == nil || !strings.Contains(err.Error(), "photo or a video") {
		t.Errorf("unknown extension must fail with advice: %v", err)
	}
	if len(m.calls) != 2 {
		t.Errorf("a rejected input must send nothing, got %d calls", len(m.calls))
	}
}

func TestManyFilesAreGroupedByTen(t *testing.T) {
	cases := []struct {
		n    int
		want []string // method per call
	}{
		{25, []string{"sendMediaGroup", "sendMediaGroup", "sendMediaGroup"}},
		{21, []string{"sendMediaGroup", "sendMediaGroup", "sendPhoto"}},
	}
	for _, tc := range cases {
		m := newMock(t)
		names := make([]string, tc.n)
		for i := range names {
			names[i] = fmt.Sprintf("p%02d.jpg", i)
		}
		args := append([]string{"photo"}, touch(t, t.TempDir(), names...)...)
		args = append(args, "-c", "Set", "--rpm", "1800")
		if _, err := run(t, okEnv(m), args...); err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, c := range m.calls {
			got = append(got, c.method)
		}
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("n=%d: calls %v, want %v", tc.n, got, tc.want)
		}
		if _, caps := mediaTypes(t, m.calls[1]); caps[0] != "" {
			t.Errorf("n=%d: only the first album carries the caption", tc.n)
		}
		if len(m.calls[0].files) != 10 {
			t.Errorf("n=%d: first album uploads %d files, want 10", tc.n, len(m.calls[0].files))
		}
	}
}

func TestSeveralFilesGoOutAsDocumentAlbum(t *testing.T) {
	m := newMock(t)
	files := touch(t, t.TempDir(), "a.pdf", "b.zip", "c.txt")
	if _, err := run(t, okEnv(m), append(append([]string{"file"}, files...), "--rpm", "1800")...); err != nil {
		t.Fatal(err)
	}
	types, _ := mediaTypes(t, m.calls[0])
	if strings.Join(types, ",") != "document,document,document" {
		t.Errorf("types: %v", types)
	}
}

func TestTextFromFileIsSplitInOrder(t *testing.T) {
	m := newMock(t)
	var lines []string
	for i := range 1500 {
		lines = append(lines, fmt.Sprintf("line %04d", i))
	}
	body := strings.Join(lines, "\n")
	path := touch(t, t.TempDir(), "msg.txt")[0]
	if err := os.WriteFile(path, []byte(body+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, okEnv(m), "text", "--file", path, "--rpm", "1800"); err != nil {
		t.Fatal(err)
	}
	if len(m.calls) < 3 {
		t.Fatalf("expected a split into several messages, got %d", len(m.calls))
	}
	var parts []string
	for _, c := range m.calls {
		if n := len([]rune(c.form["text"])); n > 4096 {
			t.Errorf("message of %d characters exceeds 4096", n)
		}
		parts = append(parts, c.form["text"])
	}
	if strings.Join(parts, "\n") != body {
		t.Error("the messages joined do not equal the file content")
	}
	if _, err := run(t, okEnv(m), "text", "x", "--file", path); err == nil || !strings.Contains(err.Error(), "not both") {
		t.Errorf("args plus --file must fail: %v", err)
	}
}

func TestLogFlags(t *testing.T) {
	m := newMock(t)
	for _, args := range [][]string{{"-v"}, {"--verbose"}, {"--log-level", "info"}} {
		if _, err := run(t, okEnv(m), append([]string{"text", "hi", "--rpm", "1800"}, args...)...); err != nil {
			t.Errorf("%v: %v", args, err)
		}
	}
	_, err := run(t, okEnv(m), "text", "hi", "--log-level", "loud")
	if err == nil || !strings.Contains(err.Error(), "invalid log level") {
		t.Errorf("bad level: %v", err)
	}
}

func TestAlbumSizeFromFlagAndEnv(t *testing.T) {
	names := make([]string, 7)
	for i := range names {
		names[i] = fmt.Sprintf("p%d.jpg", i)
	}
	files := touch(t, t.TempDir(), names...)
	methods := func(m *mock) string {
		var got []string
		for _, c := range m.calls {
			got = append(got, c.method)
		}
		return strings.Join(got, ",")
	}

	m := newMock(t)
	if _, err := run(t, okEnv(m), append(append([]string{"photo"}, files...), "--album-size", "3", "--rpm", "1800")...); err != nil {
		t.Fatal(err)
	}
	if got := methods(m); got != "sendMediaGroup,sendMediaGroup,sendPhoto" {
		t.Errorf("--album-size 3: %s", got)
	}
	if n := len(m.calls[0].files); n != 3 {
		t.Errorf("first album has %d files, want 3", n)
	}

	m = newMock(t)
	env := okEnv(m)
	env["TELESEND_ALBUM_SIZE"] = "4"
	if _, err := run(t, env, append(append([]string{"photo"}, files...), "--rpm", "1800")...); err != nil {
		t.Fatal(err)
	}
	if got := methods(m); got != "sendMediaGroup,sendMediaGroup" {
		t.Errorf("env 4: %s", got)
	}

	_, err := run(t, okEnv(m), "photo", files[0], "--album-size", "11")
	if err == nil || !strings.Contains(err.Error(), "--album-size") {
		t.Errorf("album size 11 must fail: %v", err)
	}
}

func TestTextLimitFromFlagAndEnv(t *testing.T) {
	body := strings.Repeat("0123456789\n", 30) // 330 characters in 30 lines
	for name, tc := range map[string]struct {
		env  string
		args []string
	}{"flag": {"", []string{"--text-limit", "100"}}, "env": {"100", nil}} {
		m := newMock(t)
		env := okEnv(m)
		if tc.env != "" {
			env["TELESEND_TEXT_LIMIT"] = tc.env
		}
		if _, err := run(t, env, append([]string{"text", strings.TrimSuffix(body, "\n"), "--rpm", "1800"}, tc.args...)...); err != nil {
			t.Fatal(err)
		}
		if len(m.calls) < 4 {
			t.Errorf("%s: want a split into at least 4 messages, got %d", name, len(m.calls))
		}
		for _, c := range m.calls {
			if n := len([]rune(c.form["text"])); n > 100 {
				t.Errorf("%s: message of %d characters", name, n)
			}
		}
	}
}

func TestOutputModes(t *testing.T) {
	m := newMock(t)
	file := touch(t, t.TempDir(), "a.pdf")[0]

	human, err := run(t, okEnv(m), "file", file, "--rpm", "1800")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"sent file ", "message 5", "Sent 1 of 1 in", "Average request time"} {
		if !strings.Contains(human, want) {
			t.Errorf("human output lacks %q:\n%s", want, human)
		}
	}

	out, err := run(t, okEnv(m), "file", file, "--json", "--rpm", "1800")
	if err != nil {
		t.Fatal(err)
	}
	var rep struct {
		OK      bool `json:"ok"`
		Sent    int  `json:"sent"`
		Results []struct {
			Name       string `json:"name"`
			OK         bool   `json:"ok"`
			MessageIDs []int  `json:"message_ids"`
			Attempts   int    `json:"attempts"`
			Bytes      int64  `json:"bytes"`
		} `json:"results"`
		Stats struct {
			Requests int   `json:"requests"`
			Bytes    int64 `json:"bytes"`
		} `json:"stats"`
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("--json output is not one JSON object: %v\n%s", err, out)
	}
	if !rep.OK || rep.Sent != 1 || len(rep.Results) != 1 || rep.Results[0].MessageIDs[0] != 5 ||
		rep.Results[0].Bytes != 1 || rep.Stats.Requests != 1 || rep.Stats.Bytes != 1 {
		t.Errorf("report: %+v", rep)
	}
}

func TestJSONReportsFailure(t *testing.T) {
	m := newMock(t)
	m.failBody = `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`
	out, err := run(t, okEnv(m), "text", "a", "--json", "--rpm", "1800")
	if err == nil {
		t.Fatal("want an error")
	}
	var rep struct {
		OK      bool `json:"ok"`
		Failed  int  `json:"failed"`
		Results []struct {
			Error string `json:"error"`
		} `json:"results"`
	}
	if jerr := json.Unmarshal([]byte(out), &rep); jerr != nil || rep.OK || rep.Failed != 1 || !strings.Contains(rep.Results[0].Error, "chat not found") {
		t.Errorf("report: %+v err %v\n%s", rep, jerr, out)
	}
}

func TestTokenInAPIErrorDescriptionIsMasked(t *testing.T) {
	m := newMock(t)
	m.failBody = `{"ok":false,"error_code":400,"description":"bad path /botSECRET:TOKENVALUE/sendMessage"}`
	env := okEnv(m)
	env["TELESEND_BOT_TOKEN"] = "SECRET:TOKENVALUE"
	out, err := run(t, env, "text", "a", "--json", "--rpm", "1800")
	if err == nil || strings.Contains(err.Error(), "TOKENVALUE") || strings.Contains(out, "TOKENVALUE") {
		t.Errorf("token leaked: err=%v out=%s", err, out)
	}
}

func TestTruncatedReplyIsRetried(t *testing.T) {
	m := newMock(t)
	m.truncateFirst = true
	if _, err := run(t, okEnv(m), "text", "hi", "--rpm", "1800"); err != nil {
		t.Fatalf("a reply cut in the middle must be retried: %v", err)
	}
	if len(m.calls) != 2 {
		t.Errorf("want 2 requests, got %d", len(m.calls))
	}
}

func TestRemoteVoiceNeedingConversionIsRejected(t *testing.T) {
	m := newMock(t)
	_, err := run(t, okEnv(m), "voice", "https://example.com/note.wav")
	if err == nil || !strings.Contains(err.Error(), "converts local files only") || len(m.calls) != 0 {
		t.Errorf("err=%v calls=%d", err, len(m.calls))
	}
	if _, err := run(t, okEnv(m), "voice", "https://example.com/note.ogg", "--rpm", "1800"); err != nil {
		t.Errorf("a remote .ogg must pass: %v", err)
	}
}

func TestSetupErrorsGiveJSONFailureObject(t *testing.T) {
	m := newMock(t)
	file := touch(t, t.TempDir(), "a.pdf")[0]
	for name, args := range map[string][]string{
		"bad setting":  {"text", "hi", "--json", "--rpm", "0"},
		"missing file": {"file", file + ".nope", "--json"},
	} {
		out, err := run(t, okEnv(m), args...)
		if err == nil {
			t.Fatalf("%s: want an error", name)
		}
		var rep struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		if jerr := json.Unmarshal([]byte(out), &rep); jerr != nil || rep.OK || rep.Error == "" {
			t.Errorf("%s: stdout is not a failure object: %v\n%s", name, jerr, out)
		}
	}
	if len(m.calls) != 0 {
		t.Errorf("a setup error must send nothing, got %d calls", len(m.calls))
	}
}

func TestTokenInJobNameIsMasked(t *testing.T) {
	m := newMock(t)
	m.failBody = `{"ok":false,"error_code":403,"description":"Forbidden"}`
	env := okEnv(m)
	env["TELESEND_BOT_TOKEN"] = "SECRET:TOKENVALUE"
	url := "https://api.telegram.org/file/botSECRET:TOKENVALUE/doc.pdf"
	out, err := run(t, env, "file", url, "--json", "--rpm", "1800")
	if err == nil || strings.Contains(err.Error(), "TOKENVALUE") || strings.Contains(out, "TOKENVALUE") {
		t.Errorf("token leaked: err=%v out=%s", err, out)
	}
}

func TestSentLinesComeBeforeTheNextSend(t *testing.T) {
	// The first album must print its line while the second one waits for its slot.
	stampedLines = nil
	m := newMock(t)
	files := touch(t, t.TempDir(), "a.jpg", "b.jpg", "c.jpg", "d.jpg")
	var out bytes.Buffer
	t.Setenv("TELESEND_BOT_TOKEN", "tok")
	t.Setenv("TELESEND_CHAT_ID", "1")
	t.Setenv("TELESEND_BASE_URL", m.URL)
	root := NewRootCmd()
	root.SetOut(&lineStamp{w: &out})
	root.SetErr(&out)
	root.SetArgs(append(append([]string{"photo"}, files...), "--album-size", "2", "--rpm", "60"))
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	stamps := stampedLines
	if len(stamps) < 3 {
		t.Fatalf("want two sent lines and a summary, got %v", stamps)
	}
	// rpm 60 is 1 s per message, so an album of 2 holds the next send back for 2 s.
	if gap := stamps[1].Sub(stamps[0]); gap < 1500*time.Millisecond {
		t.Errorf("the second line came %v after the first; lines are printed late", gap)
	}
}

var stampedLines []time.Time

// lineStamp records when each line is written.
type lineStamp struct{ w io.Writer }

func (l *lineStamp) Write(p []byte) (int, error) {
	for range bytes.Count(p, []byte("\n")) {
		stampedLines = append(stampedLines, time.Now())
	}
	return l.w.Write(p)
}

func TestAlbumSizeNeverExceedsRPM(t *testing.T) {
	media := make([]telegram.Media, 10)
	names := make([]string, 10)
	for i := range media {
		media[i] = telegram.Media{Path: fmt.Sprintf("p%d.jpg", i), Kind: telegram.KindPhoto}
		names[i] = media[i].Path
	}
	for _, tc := range []struct{ rpm, albumSize, jobs, weight int }{
		{rpm: 5, albumSize: 10, jobs: 2, weight: 5},
		{rpm: 20, albumSize: 10, jobs: 1, weight: 10},
		{rpm: 1, albumSize: 10, jobs: 10, weight: 1},
	} {
		s := session{cfg: config.Config{RPM: tc.rpm, AlbumSize: tc.albumSize}}
		jobs := albumJobs(s, media, names, telegram.Options{})
		if len(jobs) != tc.jobs {
			t.Errorf("rpm %d: %d jobs, want %d", tc.rpm, len(jobs), tc.jobs)
			continue
		}
		for _, j := range jobs {
			if j.Weight > tc.rpm || (tc.jobs < 10 && j.Weight != tc.weight) {
				t.Errorf("rpm %d: job weight %d", tc.rpm, j.Weight)
			}
		}
	}
}

func TestTextFileErrorGivesJSONAndHidesToken(t *testing.T) {
	m := newMock(t)
	env := okEnv(m)
	env["TELESEND_BOT_TOKEN"] = "SECRET:TOKENVALUE"
	out, err := run(t, env, "text", "--file", "/tmp/dir-SECRET:TOKENVALUE/missing.txt", "--json")
	if err == nil || strings.Contains(err.Error(), "TOKENVALUE") || strings.Contains(out, "TOKENVALUE") {
		t.Fatalf("token leaked: err=%v out=%s", err, out)
	}
	var rep struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if jerr := json.Unmarshal([]byte(out), &rep); jerr != nil || rep.OK || rep.Error == "" {
		t.Errorf("stdout is not a failure object: %v\n%s", jerr, out)
	}
}

// flakyWriter fails its first write.
type flakyWriter struct {
	failed bool
	buf    bytes.Buffer
}

func (f *flakyWriter) Write(p []byte) (int, error) {
	if !f.failed {
		f.failed = true
		return 0, errors.New("disk full")
	}
	return f.buf.Write(p)
}

func TestWriteErrorOfSentLineIsReturned(t *testing.T) {
	m := newMock(t)
	t.Setenv("TELESEND_BOT_TOKEN", "tok")
	t.Setenv("TELESEND_CHAT_ID", "1")
	t.Setenv("TELESEND_BASE_URL", m.URL)
	w := &flakyWriter{}
	root := NewRootCmd()
	root.SetOut(w)
	root.SetErr(w)
	root.SetArgs([]string{"text", "hi", "--rpm", "1800"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Errorf("a lost output line must not end as success: %v", err)
	}
}

func TestTokenTypedAsWrongArgumentIsMasked(t *testing.T) {
	m := newMock(t)
	env := okEnv(m)
	env["TELESEND_BOT_TOKEN"] = "123456:SECRETSECRET"
	_, err := run(t, env, "text", "hi", "--log-level", "123456:SECRETSECRET")
	if err == nil || strings.Contains(err.Error(), "SECRETSECRET") {
		t.Errorf("want an error without the token: %v", err)
	}
	_, err = run(t, env, "text", "hi", "--timeout", "123456:SECRETSECRET")
	if err == nil || strings.Contains(err.Error(), "SECRETSECRET") {
		t.Errorf("want an error without the token: %v", err)
	}
}

func TestTextCanStartWithADash(t *testing.T) {
	m := newMock(t)
	if _, err := run(t, okEnv(m), "text", "--rpm", "1800", "--", "--help"); err != nil {
		t.Fatal(err)
	}
	if len(m.calls) != 1 || m.calls[0].form["text"] != "--help" {
		t.Errorf("calls: %+v", m.calls)
	}
}

func TestBlankTextIsRejectedOnEveryPath(t *testing.T) {
	m := newMock(t)
	for _, args := range [][]string{
		{"text", "   "},
		{"text", "--markdown", "   ", "--json"},
	} {
		_, err := run(t, okEnv(m), append(args, "--rpm", "1800")...)
		if err == nil {
			t.Errorf("%v: blank text must fail, not report success", args)
		}
	}
	if len(m.calls) != 0 {
		t.Errorf("nothing may be sent, got %d calls", len(m.calls))
	}
}

func TestSplitNeverSendsAnEmptyMessage(t *testing.T) {
	m := newMock(t)
	if _, err := run(t, okEnv(m), "text", "--text-limit", "1", "--rpm", "1800", "a\n\nb"); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range m.calls {
		got = append(got, c.form["text"])
	}
	if strings.Join(got, "|") != "a|b" {
		t.Errorf("messages: %q", got)
	}
}

func TestReadingStdinEndsOnSignal(t *testing.T) {
	pr, pw, err := os.Pipe() // The writer stays open, so stdin never reaches EOF.
	if err != nil {
		t.Fatal(err)
	}
	defer pw.Close()
	defer pr.Close()
	cmd := newTextCmd()
	cmd.SetIn(pr)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := readText(ctx, cmd, nil, "-")
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("want context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("readText kept waiting for stdin after the signal")
	}
}

func TestLateSignalDoesNotTurnSuccessIntoFailure(t *testing.T) {
	sent := []dispatch.Result{{Name: "a"}, {Name: "b"}}
	if err := failure(sent, true); err != nil {
		t.Errorf("every job was sent; a late signal must not fail the run: %v", err)
	}
	mixed := []dispatch.Result{{Name: "a"}, {Name: "b", Err: context.Canceled}, {Name: "c", Skipped: true}}
	err := failure(mixed, true)
	if err == nil || !strings.Contains(err.Error(), "interrupted") || !strings.Contains(err.Error(), "2 job(s) not sent") {
		t.Errorf("an interrupted run: %v", err)
	}
}
