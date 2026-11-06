package telegram

import (
	"context"
	"encoding/json"
	"fmt"
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
}

func (o Options) fields() map[string]string {
	f := map[string]string{"chat_id": o.ChatID}
	if o.Caption != "" {
		f["caption"] = o.Caption
		if o.Markdown {
			f["parse_mode"] = parseModeV2
		}
	}
	return f
}

// SendMessage sends one text message. Markdown means text is escaped MarkdownV2.
func (c *Client) SendMessage(ctx context.Context, chatID, text string, markdown bool) ([]int, error) {
	f := map[string]string{"chat_id": chatID, "text": text}
	if markdown {
		f["parse_mode"] = parseModeV2
	}
	return c.sendOne(ctx, "sendMessage", f, nil)
}

// SendDocument sends a file as a document.
func (c *Client) SendDocument(ctx context.Context, m Media, o Options) ([]int, error) {
	return c.sendMedia(ctx, "sendDocument", "document", m, o)
}

// SendPhoto sends one photo.
func (c *Client) SendPhoto(ctx context.Context, m Media, o Options) ([]int, error) {
	return c.sendMedia(ctx, "sendPhoto", "photo", m, o)
}

// SendVoice sends a voice message. Local files must be OGG/Opus, MP3 or M4A.
func (c *Client) SendVoice(ctx context.Context, m Media, o Options) ([]int, error) {
	return c.sendMedia(ctx, "sendVoice", "voice", m, o)
}

// SendMediaGroup sends 2 to 10 photos as one album. The caption sits on the first photo.
func (c *Client) SendMediaGroup(ctx context.Context, photos []Media, o Options) ([]int, error) {
	if n := len(photos); n < minGroupMedia || n > MaxGroupMedia {
		return nil, fmt.Errorf("media group needs %d to %d photos, got %d", minGroupMedia, MaxGroupMedia, n)
	}
	type item struct {
		Type      string `json:"type"`
		Media     string `json:"media"`
		Caption   string `json:"caption,omitempty"`
		ParseMode string `json:"parse_mode,omitempty"`
	}
	items := make([]item, len(photos))
	var files []upload
	for i, p := range photos {
		items[i] = item{Type: "photo"}
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
	return c.sendOne(ctx, "sendMediaGroup", map[string]string{"chat_id": o.ChatID, "media": string(raw)}, files)
}

func (c *Client) sendMedia(ctx context.Context, method, field string, m Media, o Options) ([]int, error) {
	f := o.fields()
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
