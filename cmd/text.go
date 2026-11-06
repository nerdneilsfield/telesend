package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dengqi/telesend/internal/dispatch"
	"github.com/dengqi/telesend/internal/mdv2"
	"github.com/dengqi/telesend/internal/telegram"
)

func newTextCmd() *cobra.Command {
	var markdown bool
	c := &cobra.Command{
		Use:   "text [TEXT...]",
		Short: "Send a text message",
		Long: `Send a text message.

Pass the text as arguments, or pipe it on stdin. A text longer than 4096
characters is split at line ends into several messages.

With --markdown, telesend converts CommonMark (headings, **bold**, *italic*,
~~strike~~, lists, quotes, links, code) to Telegram MarkdownV2 and escapes
every reserved character.`,
		Example: `  telesend text "Build finished"
  telesend text --markdown "# Report\n\n- **ok**: 12\n- failed: 0"
  git log -5 --oneline | telesend text`,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := readText(cmd, args)
			if err != nil {
				return usageErr(cmd, err)
			}
			return runSend(cmd, func(_ context.Context, s session) ([]dispatch.Job, func(), error) {
				var pieces []string
				if markdown {
					pieces, err = mdv2.Convert(body, telegram.MaxText)
					if err != nil {
						return nil, nil, err
					}
				} else {
					pieces = mdv2.Split(body, telegram.MaxText)
				}
				jobs := make([]dispatch.Job, len(pieces))
				for i, p := range pieces {
					name := "text"
					if len(pieces) > 1 {
						name = fmt.Sprintf("text %d/%d", i+1, len(pieces))
					}
					jobs[i] = dispatch.Job{Name: name, Run: func(ctx context.Context) ([]int, error) {
						return s.client.SendMessage(ctx, s.cfg.ChatID, p, markdown)
					}}
				}
				return jobs, nil, nil
			})
		},
	}
	c.Flags().BoolVarP(&markdown, "markdown", "m", false, "treat the text as Markdown")
	return c
}

// readText takes the text from args, or from stdin when no args are given.
func readText(cmd *cobra.Command, args []string) (string, error) {
	if len(args) > 0 && (len(args) != 1 || args[0] != "-") {
		return strings.Join(args, " "), nil
	}
	in := cmd.InOrStdin()
	if f, ok := in.(*os.File); ok && len(args) == 0 {
		if info, err := f.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
			return "", fmt.Errorf("text needs a message argument or piped stdin")
		}
	}
	b, err := io.ReadAll(in)
	if err != nil {
		return "", err
	}
	body := strings.TrimRight(string(b), "\r\n")
	if strings.TrimSpace(body) == "" {
		return "", fmt.Errorf("the text is empty")
	}
	return body, nil
}
