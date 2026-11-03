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
		{Kind: "heading", Level: 1, Line: 1, EndLine: 1, Text: "ATX `code`"},
		{Kind: "code", Line: 3, EndLine: 5, Text: "# not a heading\n", Lang: "sh"},
		{Kind: "heading", Level: 2, Line: 7, EndLine: 8, Text: "Setext"},
		{Kind: "paragraph", Line: 10, EndLine: 12, Text: "one two\nthree\nfour"},
		{Kind: "list", Line: 14, EndLine: 15, Text: "a\nb"},
		{Kind: "quote", Line: 17, EndLine: 17, Text: "q"},
		{Kind: "rule", Line: 19, EndLine: 19},
		{Kind: "html", Line: 21, EndLine: 23, Text: "<p>\nhi\n</p>\n"},
		// Ends before the reference definition, which is not a block
		{Kind: "table", Line: 25, EndLine: 27, Text: "h1 | h2\nc1 | c2"},
	}
	if !reflect.DeepEqual(blocks, want) {
		t.Errorf("blocks:\n got %+v\nwant %+v", blocks, want)
	}
	if len(links) != 0 {
		t.Errorf("links = %+v, want none: a definition alone is not a link", links)
	}
}

func TestParseEndLine(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want [][2]int // line and end line of each block
	}{
		{"loose list", "- a\n\n- b\n  more\n\n\npara\n", [][2]int{{1, 4}, {7, 7}}},
		{"unclosed fence", "```\ncode\n\n", [][2]int{{1, 2}}},
		{"no final newline", "# h\n\ntext", [][2]int{{1, 1}, {3, 3}}},
		{"blank lines at the end", "text\n\n\n", [][2]int{{1, 1}}},
		{"lazy continuation", "> quote\nlazy\n\nnext\n", [][2]int{{1, 2}, {4, 4}}},
		{"indented code", "    a\n\n    b\n\npara\n", [][2]int{{1, 3}, {5, 5}}},
		{"nested", "> - a\n>\n>   ```\n>   x\n>   ```\n", [][2]int{{1, 5}}},
		{"definition between", "para\n\n[r]: x.md\n\n# h\n", [][2]int{{1, 1}, {5, 5}}},
		{"crlf", "a\r\nb\r\n\r\nc\r\n", [][2]int{{1, 2}, {4, 4}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks, _, err := Parse([]byte(tt.src))
			if err != nil {
				t.Fatal(err)
			}
			var got [][2]int
			for _, b := range blocks {
				got = append(got, [2]int{b.Line, b.EndLine})
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("lines = %v, want %v", got, tt.want)
			}
		})
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
