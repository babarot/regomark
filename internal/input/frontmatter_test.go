package input

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestParseFrontMatter(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		meta   string   // the front matter as JSON
		blocks [][2]int // line and end line of each block
	}{
		{"none", "# h\n\ntext\n", `{}`, [][2]int{{1, 1}, {3, 3}}},
		{
			"title and description",
			"---\ntitle: Monitoring\ndescription: Where alerts go\n---\n\n# Monitoring\n\nBody.\n",
			`{"title":"Monitoring","description":"Where alerts go"}`,
			[][2]int{{6, 6}, {8, 8}},
		},
		{"dots closing", "---\ntitle: a\n...\n# h\n", `{"title":"a"}`, [][2]int{{4, 4}}},
		{"crlf", "---\r\ntitle: a\r\n---\r\n\r\n# h\r\n", `{"title":"a"}`, [][2]int{{5, 5}}},
		{"bom", "\xef\xbb\xbf---\ntitle: a\n---\n# h\n", `{"title":"a"}`, [][2]int{{4, 4}}},
		{"empty", "---\n---\n# h\n", `{}`, [][2]int{{3, 3}}},
		{"only comments", "---\n# a comment\n---\n# h\n", `{}`, [][2]int{{4, 4}}},
		{"at the end", "---\ntitle: a\n---", `{"title":"a"}`, nil},
		{"closing with no newline", "---\ntitle: a\n---\n\ntext", `{"title":"a"}`, [][2]int{{5, 5}}},
		{
			"nested values",
			"---\ndate: 2026-11-19\nat: 2026-11-19T10:00:00.5+09:00\nspaced: 2026-11-19 10:00:00\ntagged: !!timestamp 2026-11-19T10:00:00Z\nquoted: \"2026-11-19\"\ntags: [a, b]\nauthor: {name: x, 1: y}\ncount: 3\ndraft: false\nnothing: null\n---\n",
			`{"date":"2026-11-19","at":"2026-11-19T10:00:00.5+09:00","spaced":"2026-11-19T10:00:00Z","tagged":"2026-11-19T10:00:00Z","quoted":"2026-11-19","tags":["a","b"],"author":{"name":"x","1":"y"},"count":3,"draft":false,"nothing":null}`,
			nil,
		},
		{"non-string key", "---\n1: a\n---\n", `{"1":"a"}`, nil},
		// Without a closing line, the opening one is a rule and the rest a
		// setext heading, as without the feature
		{"unclosed", "---\ntitle: a\n\n# h\n", `{}`, [][2]int{{1, 1}, {2, 2}, {4, 4}}},
		{"not on line 1", "\n---\ntitle: a\n---\n", `{}`, [][2]int{{2, 2}, {3, 4}}},
		{"opening with trailing space", "--- \ntitle: a\n---\n", `{}`, [][2]int{{1, 1}, {2, 3}}},
		{"closing with trailing text", "---\ntitle: a\n--- x\n", `{}`, [][2]int{{1, 1}, {2, 3}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks, _, meta, err := Parse([]byte(tt.src))
			if err != nil {
				t.Fatal(err)
			}
			var want map[string]any
			if err := json.Unmarshal([]byte(tt.meta), &want); err != nil {
				t.Fatal(err)
			}
			b, err := json.Marshal(meta)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("meta = %s, want %s", b, tt.meta)
			}
			var lines [][2]int
			for _, b := range blocks {
				lines = append(lines, [2]int{b.Line, b.EndLine})
			}
			if !reflect.DeepEqual(lines, tt.blocks) {
				t.Errorf("blocks = %v, want %v", lines, tt.blocks)
			}
		})
	}
}

func TestParseFrontMatterLinks(t *testing.T) {
	src := "---\n" +
		"title: \"[not](a.md)\"\n" +
		"---\n" +
		"\n" +
		"See [b](b.md) and [r].\n" + // 5
		"\n" +
		"[r]: r.md\n"
	blocks, links, _, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	wantBlocks := []Block{{Kind: "paragraph", Line: 5, EndLine: 5, Text: "See b and r."}}
	if !reflect.DeepEqual(blocks, wantBlocks) {
		t.Errorf("blocks:\n got %+v\nwant %+v", blocks, wantBlocks)
	}
	wantLinks := []Link{{Dest: "b.md", Text: "b", Line: 5}, {Dest: "r.md", Text: "r", Line: 5}}
	if !reflect.DeepEqual(links, wantLinks) {
		t.Errorf("links:\n got %+v\nwant %+v", links, wantLinks)
	}
}

func TestParseFrontMatterError(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"invalid YAML", "---\ntitle: a\n  b: c\n---\n", "line 3"},
		{"sequence", "---\n- a\n- b\n---\n", "line 2: front matter is a sequence, not a mapping"},
		{"scalar", "---\njust text\n---\n", "line 2: front matter is a scalar, not a mapping"},
		{"not a number", "---\nx: [.nan]\n---\n", "line 2: front matter: x: 0: NaN has no JSON form"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := Parse([]byte(tt.src))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want one containing %q", err, tt.want)
			}
		})
	}
}
