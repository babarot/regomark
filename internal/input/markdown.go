package input

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// markdown is CommonMark with GitHub's tables, which are a block of their own.
var markdown = goldmark.New(goldmark.WithExtensions(extension.Table))

// Parse reads a Markdown document into its top-level blocks and its links.
// The links are not resolved: Target and Kind are left empty.
func Parse(src []byte) ([]Block, []Link, error) {
	p := parser{src: src, lineStarts: lineStarts(src)}
	root := markdown.Parser().Parse(text.NewReader(src))
	blocks := []Block{}
	links := []Link{}
	for n := root.FirstChild(); n != nil; n = n.NextSibling() {
		if _, ok := n.(*gast.LinkReferenceDefinition); ok {
			continue // its uses are links; the definition shows nothing
		}
		b, err := p.block(n)
		if err != nil {
			return nil, nil, err
		}
		blocks = append(blocks, b)
		links = append(links, p.links(n, b.Line)...)
	}
	return blocks, links, nil
}

type parser struct {
	src        []byte
	lineStarts []int
}

// lineStarts returns the offset each line starts at.
func lineStarts(src []byte) []int {
	starts := []int{0}
	for i, c := range src {
		if c == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// line returns the 1-based line of the offset.
func (p *parser) line(offset int) int {
	return sort.Search(len(p.lineStarts), func(i int) bool { return p.lineStarts[i] > offset })
}

func (p *parser) block(n gast.Node) (Block, error) {
	if n.Pos() < 0 {
		return Block{}, fmt.Errorf("a %s block has no position", n.Kind())
	}
	b := Block{Line: p.line(n.Pos())}
	switch v := n.(type) {
	case *gast.Heading:
		b.Kind, b.Level, b.Text = "heading", v.Level, p.plain(v)
	case *gast.Paragraph:
		b.Kind, b.Text = "paragraph", p.plain(v)
	case *gast.FencedCodeBlock:
		b.Kind, b.Text = "code", p.lines(v)
		if v.Info != nil {
			b.Lang = string(v.Language(p.src))
		}
	case *gast.CodeBlock:
		b.Kind, b.Text = "code", p.lines(v)
	case *gast.List:
		b.Kind, b.Text = "list", p.plain(v)
	case *gast.Blockquote:
		b.Kind, b.Text = "quote", p.plain(v)
	case *gast.HTMLBlock:
		b.Kind, b.Text = "html", p.lines(v)
		if v.HasClosure() {
			b.Text += string(v.ClosureLine.Value(p.src))
		}
	case *gast.ThematicBreak:
		b.Kind = "rule"
	case *east.Table:
		b.Kind, b.Text = "table", p.table(v)
	default:
		return Block{}, fmt.Errorf("line %d: unexpected %s block", b.Line, n.Kind())
	}
	return b, nil
}

// lines returns the lines of a code or HTML block as they are.
func (p *parser) lines(n gast.Node) string {
	var b strings.Builder
	for i := 0; i < n.Lines().Len(); i++ {
		s := n.Lines().At(i)
		b.Write(s.Value(p.src))
	}
	return b.String()
}

// table returns a row per line, the cells separated by " | ".
func (p *parser) table(t *east.Table) string {
	var rows []string
	for r := t.FirstChild(); r != nil; r = r.NextSibling() {
		var cells []string
		for c := r.FirstChild(); c != nil; c = c.NextSibling() {
			cells = append(cells, p.plain(c))
		}
		rows = append(rows, strings.Join(cells, " | "))
	}
	return strings.Join(rows, "\n")
}

// plain returns the plain text of a node: what a reader sees, with code
// spans in backquotes and a line break wherever the source breaks a line or
// a block ends.
func (p *parser) plain(n gast.Node) string {
	var b strings.Builder
	newline := func() {
		if s := b.String(); s != "" && !strings.HasSuffix(s, "\n") {
			b.WriteString("\n")
		}
	}
	_ = gast.Walk(n, func(c gast.Node, entering bool) (gast.WalkStatus, error) {
		if !entering {
			if c.Type() == gast.TypeBlock && c != n {
				newline()
			}
			return gast.WalkContinue, nil
		}
		switch v := c.(type) {
		case *gast.Text:
			b.Write(unescape(v.Segment.Value(p.src)))
			if v.SoftLineBreak() || v.HardLineBreak() {
				newline()
			}
		case *gast.String:
			b.Write(v.Value)
		case *gast.CodeSpan:
			b.WriteString("`")
			for g := v.FirstChild(); g != nil; g = g.NextSibling() {
				if t, ok := g.(*gast.Text); ok {
					b.Write(bytes.ReplaceAll(t.Segment.Value(p.src), []byte("\n"), []byte(" ")))
				}
			}
			b.WriteString("`")
			return gast.WalkSkipChildren, nil
		case *gast.AutoLink:
			b.Write(v.Label(p.src))
		}
		return gast.WalkContinue, nil
	})
	return strings.TrimSpace(b.String())
}

// unescape resolves backslash escapes and character references, as a
// renderer would.
func unescape(s []byte) []byte {
	s = util.UnescapePunctuations(s)
	s = util.ResolveNumericReferences(s)
	return util.ResolveEntityNames(s)
}

// links returns the links in a top-level block, reference-style ones
// included: the parser has already given them the destination of their
// definition.
func (p *parser) links(n gast.Node, fallback int) []Link {
	var links []Link
	lineOf := func(c gast.Node) int {
		if c.Pos() >= 0 {
			return p.line(c.Pos())
		}
		return fallback
	}
	_ = gast.Walk(n, func(c gast.Node, entering bool) (gast.WalkStatus, error) {
		if !entering {
			return gast.WalkContinue, nil
		}
		switch v := c.(type) {
		case *gast.Link:
			links = append(links, Link{Dest: string(v.Destination), Text: p.plain(v), Line: lineOf(v)})
		case *gast.AutoLink:
			dest := string(v.URL(p.src))
			if v.AutoLinkType == gast.AutoLinkEmail && !strings.HasPrefix(dest, "mailto:") {
				dest = "mailto:" + dest // as it is rendered, and so external
			}
			links = append(links, Link{Dest: dest, Text: string(v.Label(p.src)), Line: lineOf(v)})
		}
		return gast.WalkContinue, nil
	})
	return links
}
