// Package config resolves runtime settings. A flag typed on the command line
// wins over an environment variable. An environment variable wins over a default.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/pflag"
)

// Environment variable names.
const (
	EnvToken   = "TELESEND_BOT_TOKEN"
	EnvBaseURL = "TELESEND_BASE_URL"
	EnvChatID  = "TELESEND_CHAT_ID"

	EnvAlbumSize = "TELESEND_ALBUM_SIZE"
	EnvTextLimit = "TELESEND_TEXT_LIMIT"
)

// Defaults and limits.
const (
	DefaultBaseURL     = "https://api.telegram.org"
	DefaultConcurrency = 1
	// DefaultRPM keeps a bot under the Telegram group limit of 20 messages per minute.
	DefaultRPM     = 20
	DefaultRetries = 3
	// DefaultAlbumSize and DefaultTextLimit are the Telegram maximums.
	DefaultAlbumSize = 10
	DefaultTextLimit = 4096
	DefaultTimeout   = 2 * time.Minute

	// MaxConcurrency bounds the worker pool.
	MaxConcurrency = 16
	// MaxRPM is the Telegram broadcast limit of about 30 messages per second.
	MaxRPM = 1800
)

// Config holds the resolved settings.
type Config struct {
	Token       string
	BaseURL     string
	ChatID      string
	Concurrency int
	RPM         int
	Retries     int
	Timeout     time.Duration
	// AlbumSize is the number of items in one album.
	AlbumSize int
	// TextLimit is the number of characters in one text message.
	TextLimit int
}

// AddFlags registers the setting flags on fs.
func AddFlags(fs *pflag.FlagSet) {
	fs.String("token", "", "bot token (env "+EnvToken+")")
	fs.String("base-url", DefaultBaseURL, "Telegram API base URL (env "+EnvBaseURL+")")
	fs.StringP("to", "t", "", "recipient chat ID or @channel name (env "+EnvChatID+")")
	fs.Int("album-size", DefaultAlbumSize, "items per album, 2 to 10 (env "+EnvAlbumSize+")")
	fs.Int("text-limit", DefaultTextLimit, "characters per text message, 1 to 4096 (env "+EnvTextLimit+")")
	fs.Int("concurrency", DefaultConcurrency, "parallel requests; above 1 the send order is not kept")
	fs.Int("rpm", DefaultRPM, "maximum requests per minute")
	fs.Int("retries", DefaultRetries, "retries after a 429, a 5xx response or a network error")
	fs.Duration("timeout", DefaultTimeout, "time limit for one request attempt")
}

// pick returns the flag value if typed, else the environment value, else the flag default.
func pick(fs *pflag.FlagSet, getenv func(string) string, flag, env string) string {
	if fs.Changed(flag) {
		v, _ := fs.GetString(flag)
		return v
	}
	if v := getenv(env); v != "" {
		return v
	}
	v, _ := fs.GetString(flag)
	return v
}

// Token returns the bot token without validating the other settings.
// The logger uses it to mask the token before the full settings exist.
func Token(fs *pflag.FlagSet, getenv func(string) string) string {
	return strings.TrimSpace(pick(fs, getenv, "token", EnvToken))
}

// Resolve builds a Config from fs and the environment lookup getenv.
func Resolve(fs *pflag.FlagSet, getenv func(string) string) (Config, error) {
	c := Config{
		Token:   Token(fs, getenv),
		BaseURL: strings.TrimRight(strings.TrimSpace(pick(fs, getenv, "base-url", EnvBaseURL)), "/"),
		ChatID:  strings.TrimSpace(pick(fs, getenv, "to", EnvChatID)),
	}
	var errs []error
	var err error
	if c.AlbumSize, err = pickInt(fs, getenv, "album-size", EnvAlbumSize); err != nil {
		errs = append(errs, err)
	}
	if c.TextLimit, err = pickInt(fs, getenv, "text-limit", EnvTextLimit); err != nil {
		errs = append(errs, err)
	}
	c.Concurrency, _ = fs.GetInt("concurrency")
	c.RPM, _ = fs.GetInt("rpm")
	c.Retries, _ = fs.GetInt("retries")
	c.Timeout, _ = fs.GetDuration("timeout")
	return c, errors.Join(append(errs, c.validate())...)
}

// pickInt is pick for an integer setting. A bad environment value names the variable.
func pickInt(fs *pflag.FlagSet, getenv func(string) string, flag, env string) (int, error) {
	if !fs.Changed(flag) {
		if v := getenv(env); v != "" {
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil {
				def, _ := fs.GetInt(flag)
				return def, fmt.Errorf("%s=%q is not a number", env, v)
			}
			return n, nil
		}
	}
	n, _ := fs.GetInt(flag)
	return n, nil
}

func (c Config) validate() error {
	var errs []error
	if c.Token == "" {
		errs = append(errs, fmt.Errorf("no bot token: pass --token or set %s", EnvToken))
	}
	if c.ChatID == "" {
		errs = append(errs, fmt.Errorf("no recipient: pass --to or set %s", EnvChatID))
	}
	if u, err := url.Parse(c.BaseURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		errs = append(errs, fmt.Errorf("invalid base URL %q: use http:// or https://", c.BaseURL))
	}
	if c.Concurrency < 1 || c.Concurrency > MaxConcurrency {
		errs = append(errs, fmt.Errorf("--concurrency must be 1 to %d", MaxConcurrency))
	}
	if c.RPM < 1 || c.RPM > MaxRPM {
		errs = append(errs, fmt.Errorf("--rpm must be 1 to %d", MaxRPM))
	}
	if c.AlbumSize < 2 || c.AlbumSize > 10 {
		errs = append(errs, errors.New("--album-size must be 2 to 10"))
	}
	if c.TextLimit < 1 || c.TextLimit > 4096 {
		errs = append(errs, errors.New("--text-limit must be 1 to 4096"))
	}
	if c.Retries < 0 {
		errs = append(errs, errors.New("--retries must be 0 or more"))
	}
	if c.Timeout <= 0 {
		errs = append(errs, errors.New("--timeout must be positive"))
	}
	return errors.Join(errs...)
}
