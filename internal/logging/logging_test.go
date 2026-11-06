package logging

import (
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestMaskCoreHidesSecrets(t *testing.T) {
	obs, logs := observer.New(zapcore.DebugLevel)
	log := zap.New(maskCore{Core: obs, secrets: Secrets("123456:ABCDEFGHIJ")}).With(zap.String("ctx", "id 123456:ABCDEFGHIJ"))

	log.Info("token 123456:ABCDEFGHIJ in message",
		zap.String("url", "https://x/bot123456:ABCDEFGHIJ/sendMessage"),
		zap.Error(errors.New("post bot123456:ABCDEFGHIJ failed")),
		zap.Any("raw", map[string]string{"k": "ABCDEFGHIJ"}),
	)

	e := logs.All()[0]
	all := e.Message
	for _, f := range e.Context {
		all += " " + f.String
		if f.Interface != nil {
			t.Errorf("field %q kept a raw value %v", f.Key, f.Interface)
		}
	}
	for _, secret := range []string{"ABCDEFGHIJ"} {
		if strings.Contains(all, secret) {
			t.Errorf("secret %q leaked: %s", secret, all)
		}
	}
}

func TestParseLevel(t *testing.T) {
	if l, err := ParseLevel("DEBUG"); err != nil || l != zapcore.DebugLevel {
		t.Errorf("got %v, %v", l, err)
	}
	if _, err := ParseLevel("trace"); err == nil {
		t.Error("unknown level must fail")
	}
}
