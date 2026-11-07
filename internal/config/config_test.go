package config

import (
	"testing"

	"github.com/spf13/pflag"
)

func resolve(t *testing.T, env map[string]string, args ...string) (Config, error) {
	t.Helper()
	fs := pflag.NewFlagSet("t", pflag.ContinueOnError)
	AddFlags(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}
	return Resolve(fs, func(k string) string { return env[k] })
}

func TestFlagOverridesEnv(t *testing.T) {
	env := map[string]string{EnvToken: "env-token", EnvChatID: "111", EnvBaseURL: "http://env.example"}
	c, err := resolve(t, env, "--token", "flag-token", "--to", "222")
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != "flag-token" || c.ChatID != "222" {
		t.Errorf("flags must win: got token=%q chat=%q", c.Token, c.ChatID)
	}
	if c.BaseURL != "http://env.example" {
		t.Errorf("env must beat the default: got %q", c.BaseURL)
	}
}

func TestDefaultsAndMissing(t *testing.T) {
	c, err := resolve(t, map[string]string{EnvToken: "t", EnvChatID: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if c.BaseURL != DefaultBaseURL || c.RPM != DefaultRPM {
		t.Errorf("unexpected defaults: %+v", c)
	}
	if _, err := resolve(t, nil); err == nil {
		t.Error("missing token and chat must fail")
	}
	if _, err := resolve(t, map[string]string{EnvToken: "t", EnvChatID: "1"}, "--rpm", "0"); err == nil {
		t.Error("rpm 0 must fail")
	}
}

func TestSizeSettings(t *testing.T) {
	base := map[string]string{EnvToken: "t", EnvChatID: "1"}
	c, err := resolve(t, base)
	if err != nil || c.AlbumSize != 10 || c.TextLimit != 4096 {
		t.Fatalf("defaults: %+v %v", c, err)
	}

	env := map[string]string{EnvToken: "t", EnvChatID: "1", EnvAlbumSize: "6", EnvTextLimit: "2000"}
	c, err = resolve(t, env)
	if err != nil || c.AlbumSize != 6 || c.TextLimit != 2000 {
		t.Fatalf("env: %+v %v", c, err)
	}
	c, err = resolve(t, env, "--album-size", "4", "--text-limit", "500")
	if err != nil || c.AlbumSize != 4 || c.TextLimit != 500 {
		t.Fatalf("flag must beat env: %+v %v", c, err)
	}

	for _, bad := range []map[string]string{
		{EnvToken: "t", EnvChatID: "1", EnvAlbumSize: "11"},
		{EnvToken: "t", EnvChatID: "1", EnvAlbumSize: "1"},
		{EnvToken: "t", EnvChatID: "1", EnvTextLimit: "5000"},
		{EnvToken: "t", EnvChatID: "1", EnvTextLimit: "many"},
	} {
		if _, err := resolve(t, bad); err == nil {
			t.Errorf("env %v must fail", bad)
		}
	}
}

func TestSilentAndThread(t *testing.T) {
	base := map[string]string{EnvToken: "t", EnvChatID: "1"}
	c, err := resolve(t, base, "--silent", "--thread", "7")
	if err != nil || !c.Silent || c.Thread != 7 {
		t.Errorf("%+v %v", c, err)
	}
	c, err = resolve(t, base)
	if err != nil || c.Silent || c.Thread != 0 {
		t.Errorf("defaults: %+v %v", c, err)
	}
}
