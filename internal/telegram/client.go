// Package telegram is a thin Bot API client. It covers the five send methods
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
)

// Client calls the Bot API.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
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

// Retryable reports whether the same request can succeed later.
func (e *APIError) Retryable() bool {
	return e.Code == http.StatusTooManyRequests || e.Code >= 500
}

// Media names one file to send. Set exactly one of Path and URL.
type Media struct {
	Path string
	URL  string
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
	go func() { _ = pw.CloseWithError(writeForm(mw, fields, files)) }()

	endpoint := c.BaseURL + "/bot" + c.Token + "/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, pr)
	if err != nil {
		_ = pr.Close()
		return nil, c.redact(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	// A known length avoids chunked encoding, which not every Bot API server accepts.
	req.ContentLength = size

	resp, err := c.HTTP.Do(req)
	if err != nil {
		_ = pr.Close()
		return nil, c.redact(err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, c.redact(err)
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
			Code:        code,
			Description: reply.Description,
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

func writeForm(mw *multipart.Writer, fields map[string]string, files []upload) error {
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			return err
		}
	}
	for _, f := range files {
		if err := writeFile(mw, f); err != nil {
			return err
		}
	}
	return mw.Close()
}

func writeFile(mw *multipart.Writer, f upload) error {
	file, err := os.Open(f.path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	part, err := mw.CreateFormFile(f.field, filepath.Base(f.path))
	if err != nil {
		return err
	}
	_, err = io.Copy(part, file)
	return err
}

// redact removes the bot token from a transport error. The token is part of the URL path.
func (c *Client) redact(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		ue.URL = strings.ReplaceAll(ue.URL, c.Token, "<token>")
	}
	return err
}
