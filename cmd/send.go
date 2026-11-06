package cmd

import (
	"context"
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
	log := loggerFrom(cmd)
	s := session{cfg: cfg, client: &telegram.Client{
		BaseURL: cfg.BaseURL, Token: cfg.Token, HTTP: &http.Client{Transport: transport}, Log: log,
	}}
	log.Debug("settings", zap.String("base_url", cfg.BaseURL), zap.String("chat", cfg.ChatID),
		zap.Int("rpm", cfg.RPM), zap.Int("concurrency", cfg.Concurrency),
		zap.Int("retries", cfg.Retries), zap.Duration("timeout", cfg.Timeout))

	jobs, cleanup, err := build(cmd.Context(), s)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return err
	}

	asJSON, _ := cmd.Flags().GetBool("json")
	bar := newProgress(jobs, !asJSON && stderrIsTerminal())

	log.Info("sending", zap.Int("jobs", len(jobs)))
	began := time.Now()
	results := dispatch.Run(cmd.Context(), jobs, dispatch.Options{
		Concurrency: cfg.Concurrency,
		RPM:         cfg.RPM,
		Retries:     cfg.Retries,
		Timeout:     cfg.Timeout,
		BackoffBase: backoffBase,
		OnProgress:  bar.onProgress,
		OnDone:      bar.onDone,
		OnAttempt: func(job string, attempt int, took time.Duration, err error) {
			log.Debug("attempt", zap.String("job", job), zap.Int("attempt", attempt),
				zap.Duration("took", took), zap.Bool("ok", err == nil))
		},
		OnRetry: func(job string, attempt int, wait time.Duration, err error) {
			log.Warn("retrying", zap.String("job", job), zap.Int("retry", attempt),
				zap.Duration("wait", wait), zap.Error(err))
		},
	})
	bar.finish()
	sum := summarize(results, time.Since(began))

	for _, r := range results {
		switch {
		case r.Skipped:
			log.Warn("not sent", zap.String("job", r.Name))
		case r.Err != nil:
			log.Error("failed", zap.String("job", r.Name), zap.Int("attempts", r.Attempts), zap.Error(r.Err))
		default:
			log.Info("sent", zap.String("job", r.Name), zap.Ints("message_ids", r.IDs), zap.Int("attempts", r.Attempts))
		}
	}

	write := writeHuman
	if asJSON {
		write = writeJSON
	}
	if err := write(cmd.OutOrStdout(), results, sum); err != nil {
		return err
	}
	return failure(results, cmd.Context().Err() != nil)
}

// failure summarizes failed and skipped jobs. It returns nil when all jobs were sent.
// interrupted is true when the user stopped the run with a signal.
func failure(results []dispatch.Result, interrupted bool) error {
	var errs []error
	skipped := 0
	for _, r := range results {
		switch {
		case r.Skipped:
			skipped++
		case interrupted && errors.Is(r.Err, context.Canceled):
			// The signal, not the job, caused this error.
		case r.Err != nil:
			errs = append(errs, fmt.Errorf("%s: %w", r.Name, r.Err))
		}
	}
	if interrupted {
		errs = append([]error{errors.New("interrupted")}, errs...)
	}
	if len(errs) == 0 && skipped == 0 {
		return nil
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

// fileSize returns the size of a local file. It returns 0 for a URL.
func fileSize(m telegram.Media) int64 {
	if m.Path == "" {
		return 0
	}
	info, err := os.Stat(m.Path)
	if err != nil {
		return 0
	}
	return info.Size()
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
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file; pass a file", m.Path)
	}
	return nil
}
