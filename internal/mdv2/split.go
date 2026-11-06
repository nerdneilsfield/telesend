package mdv2

import (
	"strings"
	"unicode/utf8"
)

// Split cuts plain text into pieces of at most limit characters.
// It breaks at line ends. It cuts inside a line only when one line exceeds limit.
func Split(s string, limit int) []string {
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		for utf8.RuneCountInString(l) > limit {
			cut := len(string([]rune(l)[:limit]))
			lines = append(lines, l[:cut])
			l = l[cut:]
		}
		lines = append(lines, l)
	}
	pieces := make([]piece, len(lines))
	for i, l := range lines {
		pieces[i] = piece{text: l, sep: "\n"}
	}
	return packRaw(pieces, limit)
}

// packRaw is pack for plain text: it keeps spaces and counts runes.
func packRaw(pieces []piece, limit int) []string {
	var out []string
	cur, started := "", false
	for _, p := range pieces {
		switch {
		case !started:
			cur, started = p.text, true
		case utf8.RuneCountInString(cur+p.sep+p.text) <= limit:
			cur += p.sep + p.text
		default:
			out = append(out, cur)
			cur = p.text
		}
	}
	if started {
		out = append(out, cur)
	}
	return out
}
