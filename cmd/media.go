package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dengqi/telesend/internal/audio"
	"github.com/dengqi/telesend/internal/dispatch"
	"github.com/dengqi/telesend/internal/telegram"
)

// albumSpec describes a command that sends photos, videos or documents.
type albumSpec struct {
	use, short, long, example, need string
	aliases                         []string
	// kind forces the media type. Empty means: detect it from each file extension.
	kind telegram.Kind
}

func newPhotoCmd() *cobra.Command {
	return newAlbumCmd(albumSpec{
		use: "photo SRC...", aliases: []string{"image"}, kind: telegram.KindPhoto,
		short: "Send one photo or albums of photos",
		long: `Send photos. SRC is a local path or an http(s) URL.

One photo goes out as a single message. Several photos go out as albums.
The caption appears on the first photo.

Telegram recompresses photos. To keep the original quality, send the image
with 'telesend file'.`,
		example: `  telesend photo cat.jpg
  telesend photo a.jpg b.jpg c.jpg --caption "Trip"
  telesend photo ./shots/*.png`,
		need: "at least one photo path or URL",
	})
}

func newVideoCmd() *cobra.Command {
	return newAlbumCmd(albumSpec{
		use: "video SRC...", kind: telegram.KindVideo,
		short: "Send one video or albums of videos",
		long: `Send videos. SRC is a local path or an http(s) URL.

One video goes out as a single message. Several videos go out as albums.
The caption appears on the first video. Telegram plays MP4 (H.264) files inline.`,
		example: `  telesend video demo.mp4
  telesend video a.mp4 b.mp4 --caption "Demos"`,
		need: "at least one video path or URL",
	})
}

func newMediaCmd() *cobra.Command {
	return newAlbumCmd(albumSpec{
		use: "media SRC...", aliases: []string{"album"},
		short: "Send photos and videos together as albums",
		long: `Send photos and videos in one album. SRC is a local path or an http(s) URL.
telesend tells photos from videos by the file extension.

Photo extensions: .jpg .jpeg .png .webp .bmp
Video extensions: .mp4 .mov .mkv .webm .avi .m4v

Many files make several albums, in the order given. The caption appears on
the first item.`,
		example: `  telesend media a.jpg b.mp4 c.png --caption "Release 2"
  telesend media ./out/*`,
		need: "at least one photo or video path or URL",
	})
}

func newFileCmd() *cobra.Command {
	return newAlbumCmd(albumSpec{
		use: "file SRC...", aliases: []string{"document"}, kind: telegram.KindDocument,
		short: "Send files as documents",
		long: `Send each SRC as a document, without recompression. SRC is a local path or
an http(s) URL.

One file goes out as a single message. Several files go out as document
albums. The caption appears on the first file.`,
		example: `  telesend file report.pdf
  telesend file a.zip b.zip --caption "Release build"
  telesend file ./logs/*.txt`,
		need: "at least one file path or URL",
	})
}

func newAlbumCmd(sp albumSpec) *cobra.Command {
	var capt captionOpts
	c := &cobra.Command{
		Use: sp.use, Aliases: sp.aliases, Short: sp.short, Long: sp.long, Example: sp.example,
		Args: minArgs(1, sp.need),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSend(cmd, func(_ context.Context, s session) ([]dispatch.Job, func(), error) {
				opts, err := capt.options(s.cfg.ChatID)
				if err != nil {
					return nil, nil, err
				}
				media, err := parseAll(args, sp.kind)
				if err != nil {
					return nil, nil, err
				}
				return albumJobs(s, media, args, opts), nil, nil
			})
		},
	}
	capt.addFlags(c)
	return c
}

// albumJobs groups media into albums of s.cfg.AlbumSize items. One leftover item goes out alone.
// Only the first job carries the caption.
func albumJobs(s session, media []telegram.Media, names []string, opts telegram.Options) []dispatch.Job {
	var jobs []dispatch.Job
	// An album of n items counts as n messages, so it cannot exceed --rpm.
	// With --rpm 1 no album fits, and each file goes out alone.
	size := min(s.cfg.AlbumSize, s.cfg.RPM)
	if size < 2 {
		size = 1
	}
	for i := 0; i < len(media); i += size {
		end := min(i+size, len(media))
		chunk := media[i:end]
		var bytes int64
		for _, m := range chunk {
			bytes += fileSize(m)
		}
		o := opts
		if i > 0 {
			o.Caption, o.Markdown = "", false
		}
		if len(chunk) == 1 {
			jobs = append(jobs, dispatch.Job{Name: fmt.Sprintf("%s %s", kindLabel(chunk[0].Kind), names[i]), Size: bytes, Run: func(ctx context.Context) ([]int, error) {
				return s.client.SendMedia(ctx, chunk[0], o)
			}})
			continue
		}
		name := fmt.Sprintf("album of %d", len(chunk))
		if len(media) > size {
			name = fmt.Sprintf("album %d-%d of %d", i+1, end, len(media))
		}
		jobs = append(jobs, dispatch.Job{Name: name, Weight: len(chunk), Size: bytes, Run: func(ctx context.Context) ([]int, error) {
			return s.client.SendMediaGroup(ctx, chunk, o)
		}})
	}
	return jobs
}

func newVoiceCmd() *cobra.Command {
	var capt captionOpts
	c := &cobra.Command{
		Use:   "voice SRC",
		Short: "Send an audio file as a voice message",
		Long: `Send one audio file as a playable voice message. SRC is a local path or an
http(s) URL.

.ogg (Opus), .mp3 and .m4a files go out as they are. For any other local
format (.wav, .flac, .aac ...) telesend converts the file with ffmpeg.`,
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
				if m.URL != "" {
					if err = audio.CheckRemote(m.URL); err != nil {
						return nil, nil, err
					}
				}
				if m.Path != "" {
					m.Path, cleanup, err = audio.PrepareVoice(ctx, m.Path)
					if err != nil {
						return nil, nil, err
					}
				}
				job := dispatch.Job{Name: "voice " + args[0], Size: fileSize(m), Run: func(ctx context.Context) ([]int, error) {
					return s.client.SendVoice(ctx, m, opts)
				}}
				return []dispatch.Job{job}, cleanup, nil
			})
		},
	}
	capt.addFlags(c)
	return c
}

// parseAll classifies each argument as a URL or a local file, checks local files,
// and sets the media kind. An empty kind means: detect it from the extension.
func parseAll(args []string, kind telegram.Kind) ([]telegram.Media, error) {
	media := make([]telegram.Media, len(args))
	for i, a := range args {
		m := telegram.ParseMedia(a)
		if err := checkLocal(m); err != nil {
			return nil, err
		}
		m.Kind = kind
		if kind == "" {
			k, err := telegram.DetectKind(m)
			if err != nil {
				return nil, err
			}
			m.Kind = k
		}
		media[i] = m
	}
	return media, nil
}

// kindLabel is the word a user knows for a media kind.
func kindLabel(k telegram.Kind) string {
	if k == telegram.KindDocument {
		return "file"
	}
	return string(k)
}
