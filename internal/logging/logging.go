// Package logging builds the CLI logger. Logs go to stderr so stdout stays
// free for command output. The logger replaces every secret it is given.
package logging

import (
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Mask replaces a secret in logs and errors.
const Mask = "<token>"

// minSecretLen keeps a short fragment from masking ordinary words.
const minSecretLen = 6

// ParseLevel converts a level name to a zap level.
func ParseLevel(name string) (zapcore.Level, error) {
	switch strings.ToLower(name) {
	case "debug":
		return zapcore.DebugLevel, nil
	case "info":
		return zapcore.InfoLevel, nil
	case "warn", "warning":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	}
	return 0, fmt.Errorf("invalid log level %q: use debug, info, warn or error", name)
}

// Secrets returns the strings to mask for a bot token: the whole token and its
// secret half (the part after the colon).
func Secrets(token string) []string {
	var out []string
	if len(token) >= minSecretLen {
		out = append(out, token)
	}
	if _, secret, ok := strings.Cut(token, ":"); ok && len(secret) >= minSecretLen {
		out = append(out, secret)
	}
	return out
}

// New returns a console logger at level. It masks every string in secrets.
func New(level zapcore.Level, secrets ...string) *zap.Logger {
	encoderCfg := zap.NewDevelopmentEncoderConfig()
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderCfg),
		zapcore.Lock(os.Stderr),
		level,
	)
	return zap.New(maskCore{Core: core, secrets: secrets})
}

// maskCore removes secrets from the message and from the fields of each entry.
type maskCore struct {
	zapcore.Core
	secrets []string
}

func (m maskCore) mask(s string) string { return MaskString(s, m.secrets...) }

// MaskString replaces every secret in s with Mask.
func MaskString(s string, secrets ...string) string {
	for _, secret := range secrets {
		s = strings.ReplaceAll(s, secret, Mask)
	}
	return s
}

func (m maskCore) With(fields []zapcore.Field) zapcore.Core {
	return maskCore{Core: m.Core.With(m.maskFields(fields)), secrets: m.secrets}
}

func (m maskCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if m.Enabled(ent.Level) {
		return ce.AddCore(ent, m)
	}
	return ce
}

func (m maskCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	ent.Message = m.mask(ent.Message)
	return m.Core.Write(ent, m.maskFields(fields))
}

// maskFields turns every field that can hold text into a masked string field.
func (m maskCore) maskFields(fields []zapcore.Field) []zapcore.Field {
	out := make([]zapcore.Field, len(fields))
	for i, f := range fields {
		switch f.Type {
		case zapcore.StringType:
			f.String = m.mask(f.String)
		case zapcore.ErrorType:
			if err, ok := f.Interface.(error); ok {
				f = zap.String(f.Key, m.mask(err.Error()))
			}
		case zapcore.ByteStringType:
			f = zap.String(f.Key, m.mask(string(f.Interface.([]byte))))
		case zapcore.ReflectType, zapcore.StringerType:
			f = zap.String(f.Key, m.mask(fmt.Sprint(f.Interface)))
		}
		out[i] = f
	}
	return out
}
