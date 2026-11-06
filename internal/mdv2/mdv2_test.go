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
		if VisibleLen(m) > 20 {
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
		if !strings.HasPrefix(m, "```") || !strings.HasSuffix(m, "```") || VisibleLen(m) > 40 {
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

// unescapedCount counts the unescaped occurrences of c in s.
func unescapedCount(s string, c rune) int {
	n, esc := 0, false
	for _, r := range s {
		switch {
		case esc:
			esc = false
		case r == '\\':
			esc = true
		case r == c:
			n++
		}
	}
	return n
}

func TestEscapesDoNotCountAsCharacters(t *testing.T) {
	// 600 periods become 1200 characters of markup but are 600 characters in Telegram.
	got, err := Convert(strings.Repeat(".", 600), 1024)
	if err != nil || len(got) != 1 {
		t.Fatalf("a 600-character text must fit one 1024-character caption: %d messages, err %v", len(got), err)
	}
	got, err = Convert(strings.Repeat(".", 2049), 4096)
	if err != nil || len(got) != 1 {
		t.Fatalf("2049 periods must fit one message: %d messages, err %v", len(got), err)
	}
}

func TestSplitKeepsEmphasisBalanced(t *testing.T) {
	src := strings.Repeat("some words here **bold part** and more words. ", 40)
	got, err := Convert(src, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 5 {
		t.Fatalf("expected a split, got %d messages", len(got))
	}
	for _, m := range got {
		if unescapedCount(m, '*')%2 != 0 {
			t.Errorf("message has an open bold marker: %q", m)
		}
		if VisibleLen(m) > 100 {
			t.Errorf("message over limit: %q", m)
		}
	}
}

func TestInlineElementLongerThanLimitIsAnError(t *testing.T) {
	src := "**" + strings.Repeat("a", 3000) + "\n" + strings.Repeat("b", 3000) + "**"
	if _, err := Convert(src, 4096); err == nil {
		t.Error("a bold span of 6000 characters cannot fit 4096; want an error, not a broken split")
	}
}

func TestLongCodeLanguageDoesNotHang(t *testing.T) {
	src := "```" + strings.Repeat("a", 4088) + "\nx\n```"
	got, err := Convert(src, 4096)
	if err != nil || len(got) != 1 || VisibleLen(got[0]) > 4096 {
		t.Fatalf("got %d messages, err %v", len(got), err)
	}
	if _, err := Convert("```\nx\n```", 5); err == nil {
		t.Error("a limit of 5 cannot hold a code fence; want an error")
	}
}

func TestEscapeInCodeIsNotCutInHalf(t *testing.T) {
	src := "```\n" + strings.Repeat("`", 50) + "\n```"
	got, err := Convert(src, 30)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range got {
		body := strings.TrimSuffix(strings.TrimPrefix(m, "```\n"), "\n```")
		if strings.HasSuffix(body, "\\") && !strings.HasSuffix(body, "\\\\") {
			t.Errorf("a backslash escape is cut off: %q", m)
		}
	}
}

func TestListSplitsBetweenItems(t *testing.T) {
	src := strings.Repeat("- item with text\n", 30)
	got, err := Convert(src, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 4 {
		t.Fatalf("expected a split, got %d", len(got))
	}
	for _, m := range got {
		for _, l := range strings.Split(m, "\n") {
			if l != "• item with text" {
				t.Errorf("a list line was cut: %q in %q", l, m)
			}
		}
	}
}
