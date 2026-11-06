// Package mdv2 turns CommonMark text into Telegram MarkdownV2.
//
// Telegram rejects a message when a reserved character is not escaped.
// Convert parses the input and escapes every reserved character in plain text.
// It maps the CommonMark constructs that MarkdownV2 can show and writes
// the rest as plain text.
package mdv2

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

const reserved = "_*[]()~`>#+-=|{}.!\\"

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

// Convert renders src as MarkdownV2 messages. Each message is at most limit
// characters. Convert splits between blocks, and between lines inside a code block.
func Convert(src string, limit int) ([]string, error) {
	data := []byte(src)
	doc := parser.Parse(text.NewReader(data))
	r := &renderer{src: data, limit: limit}

	var pieces []string
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		parts, err := r.block(n)
		if err != nil {
			return nil, err
		}
		pieces = append(pieces, parts...)
	}
	return pack(pieces, "\n\n", limit), nil
}

// pack joins pieces with sep into messages of at most limit characters.
func pack(pieces []string, sep string, limit int) []string {
	var out []string
	cur := ""
	for _, p := range pieces {
		switch {
		case cur == "":
			cur = p
		case utf8.RuneCountInString(cur)+utf8.RuneCountInString(sep)+utf8.RuneCountInString(p) <= limit:
			cur += sep + p
		default:
			out = append(out, cur)
			cur = p
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

type renderer struct {
	src   []byte
	limit int
}

// block renders one block node into one or more pieces that each fit the limit.
func (r *renderer) block(n ast.Node) ([]string, error) {
	switch n := n.(type) {
	case *ast.FencedCodeBlock, *ast.CodeBlock:
		return r.code(n), nil
	}
	s := strings.TrimRight(r.blockText(n, 0), "\n")
	if utf8.RuneCountInString(s) <= r.limit {
		return []string{s}, nil
	}
	lines := strings.Split(s, "\n")
	for _, l := range lines {
		if utf8.RuneCountInString(l) > r.limit {
			return nil, fmt.Errorf("a line is longer than %d characters; break it up", r.limit)
		}
	}
	return pack(lines, "\n", r.limit), nil
}

// code splits a code block into fenced pieces of at most limit characters.
func (r *renderer) code(n ast.Node) []string {
	lang := ""
	if f, ok := n.(*ast.FencedCodeBlock); ok {
		lang = escapeSet(string(f.Language(r.src)), "`\\")
	}
	head, tail := "```"+lang+"\n", "\n```"
	budget := r.limit - utf8.RuneCountInString(head+tail)

	var lines []string
	segs := n.Lines()
	for i := 0; i < segs.Len(); i++ {
		seg := segs.At(i)
		line := strings.TrimRight(string(seg.Value(r.src)), "\r\n")
		line = escapeSet(line, "`\\")
		// Cut a line that alone exceeds the budget.
		for utf8.RuneCountInString(line) > budget {
			cut := len(string([]rune(line)[:budget]))
			lines = append(lines, line[:cut])
			line = line[cut:]
		}
		lines = append(lines, line)
	}
	var out []string
	for _, body := range pack(lines, "\n", budget) {
		out = append(out, head+body+tail)
	}
	if len(out) == 0 {
		out = []string{head + tail}
	}
	return out
}

func (r *renderer) blockText(n ast.Node, depth int) string {
	switch n := n.(type) {
	case *ast.Heading:
		return "*" + r.inline(n) + "*"
	case *ast.Paragraph, *ast.TextBlock:
		return r.inline(n)
	case *ast.ThematicBreak:
		return "———"
	case *ast.Blockquote:
		inner := r.children(n, depth)
		lines := strings.Split(strings.TrimRight(inner, "\n"), "\n")
		for i, l := range lines {
			lines[i] = ">" + l
		}
		return strings.Join(lines, "\n")
	case *ast.List:
		return r.list(n, depth)
	case *ast.HTMLBlock:
		var b strings.Builder
		for i := 0; i < n.Lines().Len(); i++ {
			seg := n.Lines().At(i)
			b.Write(seg.Value(r.src))
		}
		return Escape(strings.TrimRight(b.String(), "\n"))
	case *ast.FencedCodeBlock, *ast.CodeBlock:
		return strings.Join(r.code(n), "\n")
	}
	return r.children(n, depth)
}

func (r *renderer) children(n ast.Node, depth int) string {
	var parts []string
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		parts = append(parts, r.blockText(c, depth))
	}
	return strings.Join(parts, "\n\n")
}

func (r *renderer) list(l *ast.List, depth int) string {
	var lines []string
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
			body = append(body, r.blockText(c, depth+1))
		}
		text := strings.Join(body, "\n")
		lines = append(lines, indent+marker+" "+text)
	}
	return strings.Join(lines, "\n")
}

func (r *renderer) inline(n ast.Node) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch c := c.(type) {
		case *ast.Text:
			b.WriteString(Escape(string(c.Value(r.src))))
			if c.HardLineBreak() || c.SoftLineBreak() {
				b.WriteByte('\n')
			}
		case *ast.String:
			b.WriteString(Escape(string(c.Value)))
		case *ast.CodeSpan:
			b.WriteString("`" + escapeSet(string(c.Text(r.src)), "`\\") + "`") //nolint:staticcheck // Text is the only accessor for code span content.
		case *ast.Emphasis:
			mark := "_"
			if c.Level == 2 {
				mark = "*"
			}
			b.WriteString(mark + r.inline(c) + mark)
		case *extast.Strikethrough:
			b.WriteString("~" + r.inline(c) + "~")
		case *ast.Link:
			b.WriteString("[" + r.inline(c) + "](" + escapeSet(string(c.Destination), ")\\") + ")")
		case *ast.Image:
			b.WriteString("[" + r.inline(c) + "](" + escapeSet(string(c.Destination), ")\\") + ")")
		case *ast.AutoLink:
			u := string(c.URL(r.src))
			b.WriteString("[" + Escape(u) + "](" + escapeSet(u, ")\\") + ")")
		case *ast.RawHTML:
			for i := 0; i < c.Segments.Len(); i++ {
				seg := c.Segments.At(i)
				b.WriteString(Escape(string(seg.Value(r.src))))
			}
		default:
			b.WriteString(r.inline(c))
		}
	}
	return b.String()
}
