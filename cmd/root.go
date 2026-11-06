// Package cmd defines the telesend command tree.
package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/dengqi/telesend/internal/config"
	"github.com/dengqi/telesend/internal/logging"
)

type loggerKey struct{}

const rootLong = `Send text, files, photos and voice messages to a Telegram chat from a bot.

Settings come from flags first, then from environment variables:

  --token     TELESEND_BOT_TOKEN   bot token from @BotFather
  --to, -t    TELESEND_CHAT_ID     recipient chat ID or @channel name
  --base-url  TELESEND_BASE_URL    API address (default https://api.telegram.org)

Requests are spaced evenly (--rpm) and run through a worker pool (--concurrency).
A 429 reply pauses all workers for the time Telegram asks.`

const rootExample = `  export TELESEND_BOT_TOKEN=123456:ABC... TELESEND_CHAT_ID=987654321
  telesend text "Build finished"
  telesend text --markdown "**Deploy** done: [log](https://example.com/log)"
  telesend photo a.jpg b.jpg --caption "Trip"
  telesend file report.pdf
  telesend voice note.wav`

// NewRootCmd builds the command tree.
func NewRootCmd() *cobra.Command {
	var verbose bool

	root := &cobra.Command{
		Use:           "telesend",
		Short:         "Send text, files, photos and voice to Telegram",
		Long:          rootLong,
		Example:       rootExample,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			ctx := context.WithValue(cmd.Context(), loggerKey{}, logging.New(verbose))
			cmd.SetContext(ctx)
		},
		PersistentPostRun: func(cmd *cobra.Command, _ []string) {
			_ = loggerFrom(cmd).Sync()
		},
	}

	pf := root.PersistentFlags()
	pf.BoolVarP(&verbose, "verbose", "v", false, "enable debug logging")
	pf.Bool("json", false, "print results as JSON")
	config.AddFlags(pf)

	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return usageErr(cmd, err)
	})

	root.AddCommand(newTextCmd(), newPhotoCmd(), newFileCmd(), newVoiceCmd(), newVersionCmd())
	return root
}

// Execute runs the root command with ctx, which is canceled on SIGINT/SIGTERM.
func Execute(ctx context.Context) error {
	return NewRootCmd().ExecuteContext(ctx)
}

func loggerFrom(cmd *cobra.Command) *zap.Logger {
	if l, ok := cmd.Context().Value(loggerKey{}).(*zap.Logger); ok {
		return l
	}
	return zap.NewNop()
}

// usageErr adds the command syntax and examples to err.
func usageErr(cmd *cobra.Command, err error) error {
	msg := fmt.Sprintf("%v\n\nUsage:\n  %s\n", err, cmd.UseLine())
	if cmd.Example != "" {
		msg += "\nExamples:\n" + cmd.Example + "\n"
	}
	return fmt.Errorf("%sRun '%s --help' for all flags", msg+"\n", cmd.CommandPath())
}

// minArgs returns an Args check that explains what is missing.
func minArgs(n int, what string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < n {
			return usageErr(cmd, fmt.Errorf("%s needs %s", cmd.Name(), what))
		}
		return nil
	}
}
