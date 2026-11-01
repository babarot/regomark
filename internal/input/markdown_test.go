package input

import (
	"reflect"
	"testing"
)

func TestParseBlocks(t *testing.T) {
	src := "# ATX `code`\n" + // 1
		"\n" +
		"```sh\n" + // 3
		"# not a heading\n" +
		"```\n" +
		"\n" +
		"Setext\n" + // 7
		"------\n" +
		"\n" +
		"one **two**\n" + // 10
		"three\\\n" +
		"four\n" +
		"\n" +
		"1. a\n" + // 14
		"2. b\n" +
		"\n" +
		"> q\n" + // 17
		"\n" +
		"***\n" + // 19
		"\n" +
		"<p>\n" + // 21
		"hi\n" +
		"</p>\n" +
		"\n" +
		"| h1 | h2 |\n" + // 25
		"|----|----|\n" +
		"| c1 | c2 |\n" +
		"\n" +
		"[ref]: x.md\n"
	blocks, links, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	want := []Block{
		{Kind: "heading", Level: 1, Line: 1, Text: "ATX `code`"},
		{Kind: "code", Line: 3, Text: "# not a heading\n", Lang: "sh"},
		{Kind: "heading", Level: 2, Line: 7, Text: "Setext"},
		{Kind: "paragraph", Line: 10, Text: "one two\nthree\nfour"},
		{Kind: "list", Line: 14, Text: "a\nb"},
		{Kind: "quote", Line: 17, Text: "q"},
		{Kind: "rule", Line: 19},
		{Kind: "html", Line: 21, Text: "<p>\nhi\n</p>\n"},
		{Kind: "table", Line: 25, Text: "h1 | h2\nc1 | c2"},
	}
	if !reflect.DeepEqual(blocks, want) {
		t.Errorf("blocks:\n got %+v\nwant %+v", blocks, want)
	}
	if len(links) != 0 {
		t.Errorf("links = %+v, want none: a definition alone is not a link", links)
	}
}

func TestParseLinks(t *testing.T) {
	src := "See [inline](a.md \"title\") and\n" + // 1
		"[full][r], [collapsed][], [shortcut]\n" + // 2
		"<https://example.com> and <me@example.com>.\n" + // 3
		"\n" +
		"- [`code` *link*](b.md)\n" + // 5
		"\n" +
		"![image](c.png)\n" + // 7
		"\n" +
		"[r]: r.md\n" +
		"[collapsed]: <c d.md>\n" +
		"[shortcut]: s.md#frag\n"
	_, links, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	want := []Link{
		{Dest: "a.md", Text: "inline", Line: 1},
		{Dest: "r.md", Text: "full", Line: 2},
		{Dest: "c d.md", Text: "collapsed", Line: 2},
		{Dest: "s.md#frag", Text: "shortcut", Line: 2},
		{Dest: "https://example.com", Text: "https://example.com", Line: 3},
		{Dest: "mailto:me@example.com", Text: "me@example.com", Line: 3},
		{Dest: "b.md", Text: "`code` link", Line: 5},
	}
	if !reflect.DeepEqual(links, want) {
		t.Errorf("links:\n got %+v\nwant %+v", links, want)
	}
}

func TestParseEmpty(t *testing.T) {
	blocks, links, err := Parse(nil)
	if err != nil {
		t.Fatal(err)
	}
	if blocks == nil || links == nil || len(blocks) != 0 || len(links) != 0 {
		t.Errorf("Parse(nil) = %#v, %#v; want empty, non-nil slices", blocks, links)
	}
}
