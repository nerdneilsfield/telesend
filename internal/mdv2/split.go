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
	return pack(lines, "\n", limit)
}
