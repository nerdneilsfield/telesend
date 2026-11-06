package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/dengqi/telesend/internal/config"
	"github.com/dengqi/telesend/internal/dispatch"
	"github.com/dengqi/telesend/internal/mdv2"
	"github.com/dengqi/telesend/internal/telegram"
)

const backoffBase = time.Second

// session carries what a command needs to build its jobs.
type session struct {
	cfg    config.Config
	client *telegram.Client
}

// builder turns the command input into jobs. cleanup may be nil.
type builder func(ctx context.Context, s session) (jobs []dispatch.Job, cleanup func(), err error)

// runSend resolves settings, builds the jobs, runs them and prints the results.
func runSend(cmd *cobra.Command, build builder) error {
	cfg, err := config.Resolve(cmd.Flags(), os.Getenv)
	if err != nil {
		return usageErr(cmd, err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = cfg.Concurrency
	s := session{cfg: cfg, client: &telegram.Client{
		BaseURL: cfg.BaseURL, Token: cfg.Token, HTTP: &http.Client{Transport: transport},
	}}

	jobs, cleanup, err := build(cmd.Context(), s)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return err
	}

	log := loggerFrom(cmd)
	results := dispatch.Run(cmd.Context(), jobs, dispatch.Options{
		Concurrency: cfg.Concurrency,
		RPM:         cfg.RPM,
		Retries:     cfg.Retries,
		Timeout:     cfg.Timeout,
		BackoffBase: backoffBase,
		OnRetry: func(job string, attempt int, wait time.Duration, err error) {
			log.Warn("retrying", zap.String("job", job), zap.Int("retry", attempt),
				zap.Duration("wait", wait), zap.Error(err))
		},
	})

	asJSON, _ := cmd.Flags().GetBool("json")
	if err := report(cmd, results, asJSON); err != nil {
		return err
	}
	return failure(results)
}

type jsonResult struct {
	Name       string `json:"name"`
	OK         bool   `json:"ok"`
	MessageIDs []int  `json:"message_ids,omitempty"`
	Attempts   int    `json:"attempts"`
	Skipped    bool   `json:"skipped,omitempty"`
	Error      string `json:"error,omitempty"`
}

func report(cmd *cobra.Command, results []dispatch.Result, asJSON bool) error {
	out := cmd.OutOrStdout()
	if asJSON {
		list := make([]jsonResult, len(results))
		for i, r := range results {
			list[i] = jsonResult{Name: r.Name, OK: r.Err == nil && !r.Skipped, MessageIDs: r.IDs, Attempts: r.Attempts, Skipped: r.Skipped}
			if r.Err != nil {
				list[i].Error = r.Err.Error()
			}
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(list)
	}
	for _, r := range results {
		if r.Err == nil && !r.Skipped {
			if _, err := fmt.Fprintf(out, "sent %s (message %s)\n", r.Name, telegram.String(r.IDs)); err != nil {
				return err
			}
		}
	}
	return nil
}

// failure summarizes failed and skipped jobs. It returns nil when all jobs were sent.
func failure(results []dispatch.Result) error {
	var errs []error
	skipped, canceled := 0, false
	for _, r := range results {
		switch {
		case r.Skipped:
			skipped++
		case errors.Is(r.Err, context.Canceled):
			canceled = true
		case r.Err != nil:
			errs = append(errs, fmt.Errorf("%s: %w", r.Name, r.Err))
		}
	}
	if len(errs) == 0 && !canceled && skipped == 0 {
		return nil
	}
	if len(errs) == 0 && canceled {
		errs = append(errs, errors.New("interrupted"))
	}
	if skipped > 0 {
		errs = append(errs, fmt.Errorf("%d job(s) not sent", skipped))
	}
	return errors.Join(errs...)
}

// captionOpts are the flags shared by the media commands.
type captionOpts struct {
	text     string
	markdown bool
}

func (c *captionOpts) addFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&c.text, "caption", "c", "", "text shown under the media (up to 1024 characters)")
	cmd.Flags().BoolVarP(&c.markdown, "markdown", "m", false, "treat the caption as Markdown")
}

// options returns the send options for chat, with the caption already converted.
func (c captionOpts) options(chat string) (telegram.Options, error) {
	o := telegram.Options{ChatID: chat}
	if c.text == "" {
		return o, nil
	}
	if !c.markdown {
		if n := utf8.RuneCountInString(c.text); n > telegram.MaxCaption {
			return o, fmt.Errorf("caption has %d characters; the limit is %d", n, telegram.MaxCaption)
		}
		o.Caption = c.text
		return o, nil
	}
	parts, err := mdv2.Convert(c.text, telegram.MaxCaption)
	if err != nil {
		return o, fmt.Errorf("caption: %w", err)
	}
	if len(parts) != 1 {
		return o, fmt.Errorf("caption is longer than %d characters after conversion", telegram.MaxCaption)
	}
	o.Caption, o.Markdown = parts[0], true
	return o, nil
}

// checkLocal fails early when a local file is missing, so no request is wasted on it.
func checkLocal(m telegram.Media) error {
	if m.URL != "" {
		return nil
	}
	info, err := os.Stat(m.Path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory; pass a file", m.Path)
	}
	return nil
}
