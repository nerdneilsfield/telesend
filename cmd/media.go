package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dengqi/telesend/internal/audio"
	"github.com/dengqi/telesend/internal/dispatch"
	"github.com/dengqi/telesend/internal/telegram"
)

func newPhotoCmd() *cobra.Command {
	var capt captionOpts
	c := &cobra.Command{
		Use:     "photo SRC...",
		Aliases: []string{"image"},
		Short:   "Send one photo or an album of photos",
		Long: `Send photos. SRC is a local path or an http(s) URL.

One photo goes out as a single message. Several photos go out as albums
of up to 10. The caption appears on the first photo.

Telegram recompresses photos and limits them to 10 MB. To keep the original
quality, send the image with 'telesend file'.`,
		Example: `  telesend photo cat.jpg
  telesend photo a.jpg b.jpg c.jpg --caption "Trip"
  telesend photo https://example.com/chart.png`,
		Args: minArgs(1, "at least one photo path or URL"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSend(cmd, func(_ context.Context, s session) ([]dispatch.Job, func(), error) {
				opts, err := capt.options(s.cfg.ChatID)
				if err != nil {
					return nil, nil, err
				}
				media, err := parseAll(args)
				if err != nil {
					return nil, nil, err
				}
				var jobs []dispatch.Job
				for i := 0; i < len(media); i += telegram.MaxGroupMedia {
					chunk := media[i:min(i+telegram.MaxGroupMedia, len(media))]
					o := opts
					if i > 0 {
						o.Caption, o.Markdown = "", false
					}
					name := fmt.Sprintf("photo %s", args[i])
					if len(chunk) == 1 {
						jobs = append(jobs, dispatch.Job{Name: name, Run: func(ctx context.Context) ([]int, error) {
							return s.client.SendPhoto(ctx, chunk[0], o)
						}})
						continue
					}
					if len(media) > telegram.MaxGroupMedia {
						name = fmt.Sprintf("album %d-%d", i+1, i+len(chunk))
					} else {
						name = fmt.Sprintf("album of %d photos", len(chunk))
					}
					jobs = append(jobs, dispatch.Job{Name: name, Run: func(ctx context.Context) ([]int, error) {
						return s.client.SendMediaGroup(ctx, chunk, o)
					}})
				}
				return jobs, nil, nil
			})
		},
	}
	capt.addFlags(c)
	return c
}

func newFileCmd() *cobra.Command {
	var capt captionOpts
	c := &cobra.Command{
		Use:     "file SRC...",
		Aliases: []string{"document"},
		Short:   "Send files as documents",
		Long: `Send each SRC as a document, without recompression. SRC is a local path
or an http(s) URL. Each file is one message and carries the caption.

Bots upload up to 50 MB per file through api.telegram.org.`,
		Example: `  telesend file report.pdf
  telesend file a.zip b.zip --caption "Release build"`,
		Args: minArgs(1, "at least one file path or URL"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSend(cmd, func(_ context.Context, s session) ([]dispatch.Job, func(), error) {
				opts, err := capt.options(s.cfg.ChatID)
				if err != nil {
					return nil, nil, err
				}
				media, err := parseAll(args)
				if err != nil {
					return nil, nil, err
				}
				jobs := make([]dispatch.Job, len(media))
				for i, m := range media {
					jobs[i] = dispatch.Job{Name: "file " + args[i], Run: func(ctx context.Context) ([]int, error) {
						return s.client.SendDocument(ctx, m, opts)
					}}
				}
				return jobs, nil, nil
			})
		},
	}
	capt.addFlags(c)
	return c
}

func newVoiceCmd() *cobra.Command {
	var capt captionOpts
	c := &cobra.Command{
		Use:   "voice SRC",
		Short: "Send an audio file as a voice message",
		Long: `Send one audio file as a playable voice message. SRC is a local path or an
http(s) URL.

Telegram plays .ogg (Opus), .mp3 and .m4a files directly. For any other format
(.wav, .flac, .aac ...) telesend converts the file to OGG/Opus with ffmpeg.
An .ogg file that is not Opus arrives as a document, not as a voice message.`,
		Example: `  telesend voice note.ogg
  telesend voice meeting.wav --caption "Standup"`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageErr(cmd, fmt.Errorf("voice needs exactly one audio path or URL"))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSend(cmd, func(ctx context.Context, s session) ([]dispatch.Job, func(), error) {
				opts, err := capt.options(s.cfg.ChatID)
				if err != nil {
					return nil, nil, err
				}
				m := telegram.ParseMedia(args[0])
				if err = checkLocal(m); err != nil {
					return nil, nil, err
				}
				cleanup := func() {}
				if m.Path != "" {
					m.Path, cleanup, err = audio.PrepareVoice(ctx, m.Path)
					if err != nil {
						return nil, nil, err
					}
				}
				job := dispatch.Job{Name: "voice " + args[0], Run: func(ctx context.Context) ([]int, error) {
					return s.client.SendVoice(ctx, m, opts)
				}}
				return []dispatch.Job{job}, cleanup, nil
			})
		},
	}
	capt.addFlags(c)
	return c
}

// parseAll classifies every argument as a URL or a local file and checks the local files.
func parseAll(args []string) ([]telegram.Media, error) {
	media := make([]telegram.Media, len(args))
	for i, a := range args {
		media[i] = telegram.ParseMedia(a)
		if err := checkLocal(media[i]); err != nil {
			return nil, err
		}
	}
	return media, nil
}
