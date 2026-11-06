// Package telegram is a thin Bot API client. It covers the send methods
// that telesend needs and reports errors in a form the retry loop can classify.
package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Client calls the Bot API.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
	// Log receives one debug line per request. It may be nil.
	Log *zap.Logger
}

// APIError is an error reply from the Bot API or from a proxy in front of it.
type APIError struct {
	Code        int
	Description string
	// RetryAfter is the wait that Telegram asks for after a 429. Zero when absent.
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	return fmt.Sprintf("telegram: %d %s", e.Code, e.Description)
}

// TransportError is a failure to read a reply after the server accepted the request.
type TransportError struct{ Err error }

func (e *TransportError) Error() string { return "telegram: reading the reply: " + e.Err.Error() }
func (e *TransportError) Unwrap() error { return e.Err }

// Retryable reports whether the same request can succeed later.
func (e *APIError) Retryable() bool {
	return e.Code == http.StatusTooManyRequests || e.Code >= 500
}

// Kind is the Bot API media type of an album item.
type Kind string

// Media kinds that can go in an album. Telegram groups documents only with documents.
const (
	KindPhoto    Kind = "photo"
	KindVideo    Kind = "video"
	KindDocument Kind = "document"
)

// Media names one file to send. Set exactly one of Path and URL.
// Kind matters for albums and for SendMedia.
type Media struct {
	Path string
	URL  string
	Kind Kind
}

var kindByExt = map[string]Kind{
	".jpg": KindPhoto, ".jpeg": KindPhoto, ".png": KindPhoto, ".webp": KindPhoto, ".bmp": KindPhoto,
	".mp4": KindVideo, ".mov": KindVideo, ".mkv": KindVideo, ".webm": KindVideo, ".avi": KindVideo, ".m4v": KindVideo,
}

// DetectKind returns the media kind from the file extension of a path or URL.
func DetectKind(m Media) (Kind, error) {
	name := m.Path
	if m.URL != "" {
		u, err := url.Parse(m.URL)
		if err != nil {
			return "", err
		}
		name = u.Path
	}
	if k, ok := kindByExt[strings.ToLower(filepath.Ext(name))]; ok {
		return k, nil
	}
	return "", fmt.Errorf("cannot tell if %q is a photo or a video; use 'telesend photo' or 'telesend video'", name)
}

// ParseMedia treats an http(s) address as a remote file and anything else as a local path.
func ParseMedia(s string) Media {
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return Media{URL: s}
	}
	return Media{Path: s}
}

type upload struct {
	field string
	path  string
}

// call posts a multipart form and returns the "result" member of the reply.
func (c *Client) call(ctx context.Context, method string, fields map[string]string, files []upload) (json.RawMessage, error) {
	boundary := multipart.NewWriter(io.Discard).Boundary()
	size, err := formSize(boundary, fields, files)
	if err != nil {
		return nil, err
	}
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	if err = mw.SetBoundary(boundary); err != nil {
		return nil, err
	}
	progress := progressFrom(ctx)
	if progress != nil {
		progress(0)
	}
	go func() { _ = pw.CloseWithError(writeForm(mw, fields, files, progress)) }()

	endpoint := c.BaseURL + "/bot" + c.Token + "/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, pr)
	if err != nil {
		_ = pr.Close()
		return nil, c.redact(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	// A known length avoids chunked encoding, which not every Bot API server accepts.
	req.ContentLength = size

	began := time.Now()
	resp, err := c.HTTP.Do(req)
	if err != nil {
		_ = pr.Close()
		return nil, c.redact(err)
	}
	if c.Log != nil {
		c.Log.Debug("telegram response", zap.String("method", method),
			zap.Int("status", resp.StatusCode), zap.Int64("request_bytes", size),
			zap.Duration("took", time.Since(began)))
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, &TransportError{Err: c.redact(err)}
	}
	var reply struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Code        int             `json:"error_code"`
		Description string          `json:"description"`
		Parameters  struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
	}
	if jsonErr := json.Unmarshal(body, &reply); jsonErr != nil {
		// A proxy or gateway answered with a non-JSON body.
		return nil, &APIError{Code: resp.StatusCode, Description: http.StatusText(resp.StatusCode)}
	}
	if !reply.OK {
		code := reply.Code
		if code == 0 {
			code = resp.StatusCode
		}
		return nil, &APIError{
			Code: code,
			// A gateway can echo the request path, which holds the token.
			Description: strings.ReplaceAll(reply.Description, c.Token, "<token>"),
			RetryAfter:  time.Duration(reply.Parameters.RetryAfter) * time.Second,
		}
	}
	return reply.Result, nil
}

// formSize computes the exact body length: the multipart framing plus the file sizes.
func formSize(boundary string, fields map[string]string, files []upload) (int64, error) {
	var cw countWriter
	mw := multipart.NewWriter(&cw)
	if err := mw.SetBoundary(boundary); err != nil {
		return 0, err
	}
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			return 0, err
		}
	}
	total := int64(0)
	for _, f := range files {
		info, err := os.Stat(f.path)
		if err != nil {
			return 0, err
		}
		if _, err := mw.CreateFormFile(f.field, filepath.Base(f.path)); err != nil {
			return 0, err
		}
		total += info.Size()
	}
	if err := mw.Close(); err != nil {
		return 0, err
	}
	return cw.n + total, nil
}

type countWriter struct{ n int64 }

func (c *countWriter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return len(p), nil
}

func writeForm(mw *multipart.Writer, fields map[string]string, files []upload, progress func(int64)) error {
	var done int64
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			return err
		}
	}
	for _, f := range files {
		n, err := writeFile(mw, f, done, progress)
		if err != nil {
			return err
		}
		done += n
	}
	return mw.Close()
}

// writeFile copies one file into the form. It reports done plus the bytes copied so far.
func writeFile(mw *multipart.Writer, f upload, done int64, progress func(int64)) (int64, error) {
	file, err := os.Open(f.path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = file.Close() }()
	part, err := mw.CreateFormFile(f.field, filepath.Base(f.path))
	if err != nil {
		return 0, err
	}
	var src io.Reader = file
	if progress != nil {
		src = &countReader{r: file, fn: func(n int64) { progress(done + n) }}
	}
	return io.Copy(part, src)
}

// redact removes the bot token from a transport error. The token is part of the URL path.
func (c *Client) redact(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		ue.URL = strings.ReplaceAll(ue.URL, c.Token, "<token>")
	}
	return err
}
