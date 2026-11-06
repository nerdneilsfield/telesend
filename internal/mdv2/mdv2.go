// Package mdv2 turns CommonMark text into Telegram MarkdownV2.
//
// Telegram rejects a message when a reserved character is not escaped.
// Convert parses the input and escapes every reserved character in plain text.
// It maps the CommonMark constructs that MarkdownV2 can show and writes
// the rest as plain text.
//
// Telegram counts the characters of a message after it parses the markup.
// Convert measures the same way, so an escape does not count as a character.
package mdv2

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

const (
	reserved = "_*[]()~`>#+-=|{}.!\\"
	// maxLang caps the language label of a fenced code block.
	maxLang = 20
)

var parser = goldmark.New(goldmark.WithExtensions(extension.Strikethrough)).Parser()

// Escape escapes every MarkdownV2 reserved character in s.
func Escape(s string) string {
	return escapeSet(s, reserved)
}

func escapeSet(s, set string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(set, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// VisibleLen counts the characters of s as Telegram counts them. A backslash
// that escapes a character does not count. Entity markers such as * and _ do
// count, which overstates the length by a few characters.
func VisibleLen(s string) int {
	n := 0
	escaped := false
	for _, r := range s {
		if r == '\\' && !escaped {
			escaped = true
			continue
		}
		escaped = false
		n++
	}
	return n
}

// piece is a part of a message. sep joins it to the piece before it in the same message.
type piece struct {
	text string
	sep  string
}

// Convert renders src as MarkdownV2 messages. Each message has at most limit
// characters. Convert splits a paragraph between inline elements, a list between
// items, and a code block between lines. It returns an error for a heading, a
// list item, a quote, or an inline element that is longer than limit.
func Convert(src string, limit int) ([]string, error) {
	data := []byte(src)
	doc := parser.Parse(text.NewReader(data))
	r := &renderer{src: data, limit: limit}

	var pieces []piece
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		parts, err := r.block(n)
		if err != nil {
			return nil, err
		}
		pieces = append(pieces, parts...)
	}
	return pack(pieces, limit, VisibleLen, true), nil
}

// pack joins pieces into messages of at most limit characters, as size measures them.
// With trim, it removes the spaces and line breaks around each message.
// Code blocks need trim off, because their spaces are content.
func pack(pieces []piece, limit int, size func(string) int, trim bool) []string {
	var out []string
	cur, started := "", false
	flush := func() {
		if trim {
			cur = strings.Trim(cur, " \n")
		}
		if cur != "" || !trim {
			out = append(out, cur)
		}
	}
	for _, p := range pieces {
		switch {
		case !started:
			cur, started = p.text, true
		case size(cur+p.sep+p.text) <= limit:
			cur += p.sep + p.text
		default:
			flush()
			cur = p.text
		}
	}
	if started {
		flush()
	}
	return out
}

type renderer struct {
	src   []byte
	limit int
}

func (r *renderer) tooLong(what string) error {
	return fmt.Errorf("%s is longer than %d characters; shorten it or raise the limit", what, r.limit)
}

// block renders one top-level block into pieces that each fit the limit.
func (r *renderer) block(n ast.Node) ([]piece, error) {
	const blockSep = "\n\n"
	switch n := n.(type) {
	case *ast.FencedCodeBlock, *ast.CodeBlock:
		return r.code(n)
	case *ast.Paragraph, *ast.TextBlock:
		return r.paragraph(n)
	case *ast.List:
		items, err := r.listItems(n, 0)
		if err != nil {
			return nil, err
		}
		out := make([]piece, len(items))
		for i, it := range items {
			if VisibleLen(it) > r.limit {
				return nil, r.tooLong("a list item")
			}
			out[i] = piece{text: it, sep: "\n"}
		}
		if len(out) > 0 {
			out[0].sep = blockSep
		}
		return out, nil
	}
	s, err := r.blockText(n, 0)
	if err != nil {
		return nil, err
	}
	s = strings.TrimRight(s, "\n")
	if VisibleLen(s) > r.limit {
		return nil, r.tooLong("a heading, quote or other block")
	}
	return []piece{{text: s, sep: blockSep}}, nil
}

// paragraph packs the inline elements of a paragraph. An element is never cut,
// so no bold, link or code marker is left open at the end of a message.
func (r *renderer) paragraph(n ast.Node) ([]piece, error) {
	var atoms []piece
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			chunks := splitWords(string(t.Value(r.src)), r.limit)
			for i, ch := range chunks {
				s := Escape(ch)
				if i == len(chunks)-1 && (t.HardLineBreak() || t.SoftLineBreak()) {
					s += "\n"
				}
				atoms = append(atoms, piece{text: s})
			}
			continue
		}
		s := r.inlineNode(c)
		if VisibleLen(s) > r.limit {
			return nil, r.tooLong("a link, code span or emphasis")
		}
		atoms = append(atoms, piece{text: s})
	}
	parts := pack(atoms, r.limit, VisibleLen, true)
	out := make([]piece, len(parts))
	for i, p := range parts {
		out[i] = piece{text: p}
	}
	if len(out) > 0 {
		out[0].sep = "\n\n"
	}
	return out, nil
}

// splitWords cuts s into chunks of at most limit characters, at spaces when it can.
func splitWords(s string, limit int) []string {
	rs := []rune(s)
	var out []string
	for len(rs) > limit {
		cut := limit
		if i := lastSpace(rs[:limit]); i > 0 {
			cut = i + 1
		}
		out = append(out, string(rs[:cut]))
		rs = rs[cut:]
	}
	return append(out, string(rs))
}

func lastSpace(rs []rune) int {
	for i := len(rs) - 1; i >= 0; i-- {
		if rs[i] == ' ' || rs[i] == '\n' {
			return i
		}
	}
	return -1
}

var errCodeLimit = errors.New("the text limit is too small for a code block")

// code splits a code block into fenced pieces that each fit the limit.
func (r *renderer) code(n ast.Node) ([]piece, error) {
	lang := ""
	if f, ok := n.(*ast.FencedCodeBlock); ok {
		l := []rune(string(f.Language(r.src)))
		lang = escapeSet(string(l[:min(len(l), maxLang)]), "`\\")
	}
	head, tail := "```"+lang+"\n", "\n```"
	budget := r.limit - VisibleLen(head+tail)
	if budget < 1 {
		return nil, errCodeLimit
	}

	var lines []piece
	segs := n.Lines()
	for i := 0; i < segs.Len(); i++ {
		seg := segs.At(i)
		raw := []rune(strings.TrimRight(string(seg.Value(r.src)), "\r\n"))
		// Cut the raw line first, then escape. An escape never splits.
		for len(raw) > budget {
			lines = append(lines, piece{text: escapeSet(string(raw[:budget]), "`\\"), sep: "\n"})
			raw = raw[budget:]
		}
		lines = append(lines, piece{text: escapeSet(string(raw), "`\\"), sep: "\n"})
	}
	var out []piece
	for _, body := range pack(lines, budget, VisibleLen, false) {
		out = append(out, piece{text: head + body + tail, sep: "\n"})
	}
	if len(out) == 0 {
		out = []piece{{text: head + tail}}
	}
	out[0].sep = "\n\n"
	return out, nil
}

func (r *renderer) blockText(n ast.Node, depth int) (string, error) {
	switch n := n.(type) {
	case *ast.Heading:
		return "*" + r.inline(n) + "*", nil
	case *ast.Paragraph, *ast.TextBlock:
		return r.inline(n), nil
	case *ast.ThematicBreak:
		return "———", nil
	case *ast.Blockquote:
		inner, err := r.children(n, depth)
		if err != nil {
			return "", err
		}
		lines := strings.Split(strings.TrimRight(inner, "\n"), "\n")
		for i, l := range lines {
			lines[i] = ">" + l
		}
		return strings.Join(lines, "\n"), nil
	case *ast.List:
		items, err := r.listItems(n, depth)
		return strings.Join(items, "\n"), err
	case *ast.HTMLBlock:
		var b strings.Builder
		for i := 0; i < n.Lines().Len(); i++ {
			seg := n.Lines().At(i)
			b.Write(seg.Value(r.src))
		}
		return Escape(strings.TrimRight(b.String(), "\n")), nil
	case *ast.FencedCodeBlock, *ast.CodeBlock:
		// A code block inside a quote or list item stays whole.
		parts, err := r.code(n)
		if err != nil {
			return "", err
		}
		var s []string
		for _, p := range parts {
			s = append(s, p.text)
		}
		return strings.Join(s, "\n"), nil
	}
	return r.children(n, depth)
}

func (r *renderer) children(n ast.Node, depth int) (string, error) {
	var parts []string
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		s, err := r.blockText(c, depth)
		if err != nil {
			return "", err
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "\n\n"), nil
}

// listItems renders each item of l, with its marker and its nested lists.
func (r *renderer) listItems(l *ast.List, depth int) ([]string, error) {
	var items []string
	num := l.Start
	indent := strings.Repeat("  ", depth)
	for item := l.FirstChild(); item != nil; item = item.NextSibling() {
		marker := "•"
		if l.IsOrdered() {
			marker = strconv.Itoa(num) + "\\."
			num++
		}
		var body []string
		for c := item.FirstChild(); c != nil; c = c.NextSibling() {
			s, err := r.blockText(c, depth+1)
			if err != nil {
				return nil, err
			}
			body = append(body, s)
		}
		items = append(items, indent+marker+" "+strings.Join(body, "\n"))
	}
	return items, nil
}

func (r *renderer) inline(n ast.Node) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		b.WriteString(r.inlineNode(c))
	}
	return b.String()
}

// inlineNode renders one inline node with its children.
func (r *renderer) inlineNode(c ast.Node) string {
	switch c := c.(type) {
	case *ast.Text:
		s := Escape(string(c.Value(r.src)))
		if c.HardLineBreak() || c.SoftLineBreak() {
			s += "\n"
		}
		return s
	case *ast.String:
		return Escape(string(c.Value))
	case *ast.CodeSpan:
		return "`" + escapeSet(string(c.Text(r.src)), "`\\") + "`" //nolint:staticcheck // Text is the only accessor for code span content.
	case *ast.Emphasis:
		mark := "_"
		if c.Level == 2 {
			mark = "*"
		}
		return mark + r.inline(c) + mark
	case *extast.Strikethrough:
		return "~" + r.inline(c) + "~"
	case *ast.Link:
		return "[" + r.inline(c) + "](" + escapeSet(string(c.Destination), ")\\") + ")"
	case *ast.Image:
		return "[" + r.inline(c) + "](" + escapeSet(string(c.Destination), ")\\") + ")"
	case *ast.AutoLink:
		u := string(c.URL(r.src))
		return "[" + Escape(u) + "](" + escapeSet(u, ")\\") + ")"
	case *ast.RawHTML:
		var b strings.Builder
		for i := 0; i < c.Segments.Len(); i++ {
			seg := c.Segments.At(i)
			b.WriteString(Escape(string(seg.Value(r.src))))
		}
		return b.String()
	}
	return r.inline(c)
}
