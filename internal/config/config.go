// Package config resolves runtime settings. A flag typed on the command line
// wins over an environment variable. An environment variable wins over a default.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/pflag"
)

// Environment variable names.
const (
	EnvToken   = "TELESEND_BOT_TOKEN"
	EnvBaseURL = "TELESEND_BASE_URL"
	EnvChatID  = "TELESEND_CHAT_ID"
)

// Defaults and limits.
const (
	DefaultBaseURL     = "https://api.telegram.org"
	DefaultConcurrency = 1
	// DefaultRPM keeps a bot under the Telegram group limit of 20 messages per minute.
	DefaultRPM     = 20
	DefaultRetries = 3
	DefaultTimeout = 2 * time.Minute

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
}

// AddFlags registers the setting flags on fs.
func AddFlags(fs *pflag.FlagSet) {
	fs.String("token", "", "bot token (env "+EnvToken+")")
	fs.String("base-url", DefaultBaseURL, "Telegram API base URL (env "+EnvBaseURL+")")
	fs.StringP("to", "t", "", "recipient chat ID or @channel name (env "+EnvChatID+")")
	fs.Int("concurrency", DefaultConcurrency, "parallel requests; above 1 the send order is not kept")
	fs.Int("rpm", DefaultRPM, "maximum requests per minute")
	fs.Int("retries", DefaultRetries, "retries after a 429, a 5xx response or a network error")
	fs.Duration("timeout", DefaultTimeout, "time limit for one request attempt")
}

// Resolve builds a Config from fs and the environment lookup getenv.
func Resolve(fs *pflag.FlagSet, getenv func(string) string) (Config, error) {
	str := func(flag, env string) string {
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
	c := Config{
		Token:   strings.TrimSpace(str("token", EnvToken)),
		BaseURL: strings.TrimRight(strings.TrimSpace(str("base-url", EnvBaseURL)), "/"),
		ChatID:  strings.TrimSpace(str("to", EnvChatID)),
	}
	c.Concurrency, _ = fs.GetInt("concurrency")
	c.RPM, _ = fs.GetInt("rpm")
	c.Retries, _ = fs.GetInt("retries")
	c.Timeout, _ = fs.GetDuration("timeout")
	return c, c.validate()
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
	if c.Retries < 0 {
		errs = append(errs, errors.New("--retries must be 0 or more"))
	}
	if c.Timeout <= 0 {
		errs = append(errs, errors.New("--timeout must be positive"))
	}
	return errors.Join(errs...)
}
