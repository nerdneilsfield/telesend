package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strconv"
)

// Limits from the Bot API.
const (
	MaxText        = 4096
	MaxCaption     = 1024
	MaxGroupMedia  = 10
	minGroupMedia  = 2
	parseModeV2    = "MarkdownV2"
	attachFieldFmt = "file%d"
)

// Options are the fields shared by every send method.
type Options struct {
	ChatID  string
	Caption string
	// Markdown marks Caption (or the text) as MarkdownV2 that is already escaped.
	Markdown bool
	// Silent sends the message without a notification sound.
	Silent bool
	// Thread is the forum topic ID in a supergroup. Zero means no topic.
	Thread int
}

// base returns the fields that every send method takes.
func (o Options) base() map[string]string {
	f := map[string]string{"chat_id": o.ChatID}
	if o.Silent {
		f["disable_notification"] = "true"
	}
	if o.Thread > 0 {
		f["message_thread_id"] = strconv.Itoa(o.Thread)
	}
	return f
}

func (o Options) fields() map[string]string {
	f := o.base()
	if o.Caption != "" {
		f["caption"] = o.Caption
		if o.Markdown {
			f["parse_mode"] = parseModeV2
		}
	}
	return f
}

// SendMessage sends one text message. With o.Markdown, text is escaped MarkdownV2.
func (c *Client) SendMessage(ctx context.Context, text string, o Options) ([]int, error) {
	f := o.base()
	f["text"] = text
	if o.Markdown {
		f["parse_mode"] = parseModeV2
	}
	return c.sendOne(ctx, "sendMessage", f, nil)
}

// SendDocument sends a file as a document.
func (c *Client) SendDocument(ctx context.Context, m Media, o Options) ([]int, error) {
	return c.sendMedia(ctx, "sendDocument", "document", m, o, nil)
}

// SendPhoto sends one photo.
func (c *Client) SendPhoto(ctx context.Context, m Media, o Options) ([]int, error) {
	return c.sendMedia(ctx, "sendPhoto", "photo", m, o, nil)
}

// SendVideo sends one video that plays while it downloads.
func (c *Client) SendVideo(ctx context.Context, m Media, o Options) ([]int, error) {
	return c.sendMedia(ctx, "sendVideo", "video", m, o, map[string]string{"supports_streaming": "true"})
}

// SendMedia sends one photo, video or document, chosen by m.Kind.
func (c *Client) SendMedia(ctx context.Context, m Media, o Options) ([]int, error) {
	switch m.Kind {
	case KindVideo:
		return c.SendVideo(ctx, m, o)
	case KindDocument:
		return c.SendDocument(ctx, m, o)
	default:
		return c.SendPhoto(ctx, m, o)
	}
}

// SendVoice sends a voice message. Local files must be OGG/Opus, MP3 or M4A.
func (c *Client) SendVoice(ctx context.Context, m Media, o Options) ([]int, error) {
	return c.sendMedia(ctx, "sendVoice", "voice", m, o, nil)
}

// SendMediaGroup sends 2 to 10 items as one album. Each Media needs a Kind.
// Photos and videos can mix. Documents must not mix with other kinds.
// The caption sits on the first item.
func (c *Client) SendMediaGroup(ctx context.Context, media []Media, o Options) ([]int, error) {
	if n := len(media); n < minGroupMedia || n > MaxGroupMedia {
		return nil, fmt.Errorf("media group needs %d to %d items, got %d", minGroupMedia, MaxGroupMedia, n)
	}
	type item struct {
		Type              string `json:"type"`
		Media             string `json:"media"`
		Caption           string `json:"caption,omitempty"`
		ParseMode         string `json:"parse_mode,omitempty"`
		SupportsStreaming bool   `json:"supports_streaming,omitempty"`
	}
	items := make([]item, len(media))
	var files []upload
	for i, p := range media {
		if p.Kind == "" {
			return nil, fmt.Errorf("media group item %d has no kind", i+1)
		}
		if (p.Kind == KindDocument) != (media[0].Kind == KindDocument) {
			return nil, errors.New("a media group cannot mix documents with photos or videos")
		}
		items[i] = item{Type: string(p.Kind), SupportsStreaming: p.Kind == KindVideo}
		if p.URL != "" {
			items[i].Media = p.URL
		} else {
			field := fmt.Sprintf(attachFieldFmt, i)
			items[i].Media = "attach://" + field
			files = append(files, upload{field: field, path: p.Path})
		}
	}
	if o.Caption != "" {
		items[0].Caption = o.Caption
		if o.Markdown {
			items[0].ParseMode = parseModeV2
		}
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	f := o.base()
	f["media"] = string(raw)
	return c.sendOne(ctx, "sendMediaGroup", f, files)
}

func (c *Client) sendMedia(ctx context.Context, method, field string, m Media, o Options, extra map[string]string) ([]int, error) {
	f := o.fields()
	maps.Copy(f, extra)
	var files []upload
	if m.URL != "" {
		f[field] = m.URL
	} else {
		f[field] = "attach://" + field
		files = []upload{{field: field, path: m.Path}}
	}
	return c.sendOne(ctx, method, f, files)
}

// sendOne returns the IDs of the messages that Telegram created.
func (c *Client) sendOne(ctx context.Context, method string, fields map[string]string, files []upload) ([]int, error) {
	raw, err := c.call(ctx, method, fields, files)
	if err != nil {
		return nil, err
	}
	var many []struct {
		ID int `json:"message_id"`
	}
	if json.Unmarshal(raw, &many) != nil {
		var one struct {
			ID int `json:"message_id"`
		}
		if err := json.Unmarshal(raw, &one); err != nil {
			return nil, fmt.Errorf("unreadable %s reply: %w", method, err)
		}
		many = append(many, one)
	}
	ids := make([]int, len(many))
	for i, m := range many {
		ids[i] = m.ID
	}
	return ids, nil
}

// String formats message IDs for output.
func String(ids []int) string {
	s := ""
	for i, id := range ids {
		if i > 0 {
			s += ","
		}
		s += strconv.Itoa(id)
	}
	return s
}
