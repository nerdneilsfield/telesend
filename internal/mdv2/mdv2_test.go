package mdv2

import (
	"strings"
	"testing"
)

// Expected strings follow the MarkdownV2 rules in the Telegram Bot API docs.
func TestConvert(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"escape reserved", "1.5 - a_b (c)!", `1\.5 \- a\_b \(c\)\!`},
		{"bold italic strike", "**b** *i* ~~s~~", `*b* _i_ ~s~`},
		{"heading to bold", "## Hi.", `*Hi\.*`},
		{"inline code keeps dots", "`a.b` and `x\\y`", "`a.b` and `x\\\\y`"},
		{"link url escapes paren", "[t](http://x.com/a_(b))", `[t](http://x.com/a_(b\))`},
		{"bullets", "- a\n- b.", "• a\n• b\\."},
		{"ordered", "1. a\n2. b", "1\\. a\n2\\. b"},
		{"quote", "> hi!", ">hi\\!"},
		{"code block", "```go\nx := a`b\n```", "```go\nx := a\\`b\n```"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Convert(c.in, 4096)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 || got[0] != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestConvertSplitsAtBlocks(t *testing.T) {
	src := strings.Repeat("para.\n\n", 10)
	got, err := Convert(src, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 3 {
		t.Fatalf("expected several messages, got %d", len(got))
	}
	for _, m := range got {
		if len([]rune(m)) > 20 {
			t.Errorf("message over limit: %q", m)
		}
	}
}

func TestConvertSplitsCodeBlockWithFences(t *testing.T) {
	src := "```\n" + strings.Repeat("line\n", 20) + "```"
	got, err := Convert(src, 40)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 2 {
		t.Fatalf("expected a split, got %d", len(got))
	}
	for _, m := range got {
		if !strings.HasPrefix(m, "```") || !strings.HasSuffix(m, "```") || len([]rune(m)) > 40 {
			t.Errorf("bad piece: %q", m)
		}
	}
}

func TestSplit(t *testing.T) {
	got := Split("aaa\nbbb\nccc", 7)
	if len(got) != 2 || got[0] != "aaa\nbbb" || got[1] != "ccc" {
		t.Errorf("got %q", got)
	}
	if got := Split("abcdef", 4); len(got) != 2 || got[0] != "abcd" {
		t.Errorf("long line: got %q", got)
	}
}
