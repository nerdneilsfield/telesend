package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nerdneilsfield/telesend/internal/dispatch"
	"github.com/nerdneilsfield/telesend/internal/mdv2"
)

func newTextCmd() *cobra.Command {
	var markdown bool
	var file string
	c := &cobra.Command{
		Use:   "text [TEXT...]",
		Short: "Send a text message",
		Long: `Send a text message.

Pass the text as arguments, read it from a file with --file, or pipe it on
stdin. A long text is split into several messages, which arrive in order.
--file sends the content of the file as the message. To attach the file, use
'telesend file'.

With --markdown, telesend formats headings, **bold**, *italic*, ~~strike~~,
lists, quotes, links and code.`,
		Example: `  telesend text "Build finished"
  telesend text --file notes.md --markdown
  telesend text --markdown "# Report\n\n- **ok**: 12\n- failed: 0"
  git log -5 --oneline | telesend text`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSend(cmd, func(ctx context.Context, s session) ([]dispatch.Job, func(), error) {
				body, err := readText(ctx, cmd, args, file)
				if err != nil {
					return nil, nil, usageErr(cmd, err)
				}
				var pieces []string
				if markdown {
					pieces, err = mdv2.Convert(body, s.cfg.TextLimit)
					if err != nil {
						return nil, nil, err
					}
				} else {
					pieces = mdv2.Split(body, s.cfg.TextLimit)
				}
				jobs := make([]dispatch.Job, len(pieces))
				for i, p := range pieces {
					name := "text"
					if len(pieces) > 1 {
						name = fmt.Sprintf("text %d/%d", i+1, len(pieces))
					}
					jobs[i] = dispatch.Job{Name: name, Run: func(ctx context.Context) ([]int, error) {
						o := s.options()
						o.Markdown = markdown
						return s.client.SendMessage(ctx, p, o)
					}}
				}
				return jobs, nil, nil
			})
		},
	}
	c.Flags().BoolVarP(&markdown, "markdown", "m", false, "treat the text as Markdown")
	c.Flags().StringVarP(&file, "file", "f", "", "read the message text from this file ('-' for stdin)")
	return c
}

// readText takes the text from --file, from args, or from stdin when neither is given.
// A signal ends the wait for stdin.
func readText(ctx context.Context, cmd *cobra.Command, args []string, file string) (string, error) {
	if file != "" && len(args) > 0 {
		return "", fmt.Errorf("give the text as arguments or with --file, not both")
	}
	var raw string
	switch {
	case file != "" && file != "-":
		b, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		raw = string(b)
	case len(args) > 0 && (len(args) != 1 || args[0] != "-"):
		raw = strings.Join(args, " ")
	default:
		in := cmd.InOrStdin()
		if f, ok := in.(*os.File); ok && len(args) == 0 && file == "" {
			if info, statErr := f.Stat(); statErr == nil && info.Mode()&os.ModeCharDevice != 0 {
				return "", fmt.Errorf("text needs a message argument, --file, or piped stdin")
			}
		}
		b, err := readAll(ctx, in)
		if err != nil {
			return "", err
		}
		raw = string(b)
	}
	body := strings.TrimRight(raw, "\r\n")
	if strings.TrimSpace(body) == "" {
		return "", fmt.Errorf("the text is empty")
	}
	return body, nil
}

// readAll reads r to the end, or returns when ctx ends. A read that is still blocked
// keeps its goroutine until the process exits, which happens right after the error.
func readAll(ctx context.Context, r io.Reader) ([]byte, error) {
	type result struct {
		b   []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		b, err := io.ReadAll(r)
		done <- result{b, err}
	}()
	select {
	case res := <-done:
		return res.b, res.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
