package input

import (
	"bytes"
	"strings"
	"testing"
)

func valid() *Input {
	return &Input{SchemaVersion: SchemaVersion, Root: ".", Documents: []Document{{
		Path: "a.md", Name: "a.md",
		Blocks: []Block{{Kind: "heading", Level: 1, Line: 1, Text: "A"}},
		Links:  []Link{{Dest: "b.md", Text: "b", Line: 1, Target: "b.md", Kind: KindMissing}},
	}}}
}

func TestSchemaHasNoReferences(t *testing.T) {
	// OPA's type checker gets the schema as it is
	if bytes.Contains(Schema(), []byte(`"$ref"`)) {
		t.Errorf("the schema has references:\n%s", Schema())
	}
}

func TestValidate(t *testing.T) {
	if err := Validate(valid()); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		modify func(*Input)
		want   string
	}{
		{"nil documents", func(in *Input) { in.Documents = nil }, "/documents"},
		{"nil blocks", func(in *Input) { in.Documents[0].Blocks = nil }, "/documents/0/blocks"},
		{"unknown kind", func(in *Input) { in.Documents[0].Blocks[0].Kind = "headline" }, "/documents/0/blocks/0/kind"},
		{"line 0", func(in *Input) { in.Documents[0].Links[0].Line = 0 }, "/documents/0/links/0/line"},
		{"unknown link kind", func(in *Input) { in.Documents[0].Links[0].Kind = "" }, "/documents/0/links/0/kind"},
		{"schema version", func(in *Input) { in.SchemaVersion = 2 }, "/schema_version"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := valid()
			tt.modify(in)
			err := Validate(in)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want one at %s", err, tt.want)
			}
		})
	}
}
