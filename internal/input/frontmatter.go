package input

import (
	"bytes"
	"fmt"
	"math"
	"time"

	"go.yaml.in/yaml/v3"
)

// bom is the UTF-8 byte order mark, which some editors write at the start of
// a file.
var bom = []byte("\xef\xbb\xbf")

// splitFrontMatter finds YAML front matter at the start of a document and
// reads it. Front matter starts with a first line that is exactly "---" and
// ends at the next line that is exactly "---" or "..."; without the closing
// line there is none. It returns the source with every line of the front
// matter blanked, so that the lines after it keep their numbers, and the front
// matter as an object, empty when there is none.
func splitFrontMatter(src []byte) ([]byte, map[string]any, error) {
	meta := map[string]any{}
	rest := bytes.TrimPrefix(src, bom)
	first, rest, ok := cutLine(rest)
	if !ok || !bytes.Equal(first, []byte("---")) {
		return src, meta, nil
	}
	// The YAML starts with an empty line in place of the opening one, so that
	// the lines its errors name are the lines of the file
	yamlSrc := []byte("\n")
	lines := 1
	for {
		line, after, ok := cutLine(rest)
		if !ok {
			return src, meta, nil // never closed: not front matter
		}
		lines++
		rest = after
		if bytes.Equal(line, []byte("---")) || bytes.Equal(line, []byte("...")) {
			break
		}
		yamlSrc = append(append(yamlSrc, line...), '\n')
	}
	body := append(bytes.Repeat([]byte("\n"), lines), rest...)
	if len(rest) == 0 && !bytes.HasSuffix(src, []byte("\n")) {
		body = body[:len(body)-1] // the closing line was the last, with no newline
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(yamlSrc, &doc); err != nil {
		return nil, nil, fmt.Errorf("front matter: %w", err)
	}
	if len(doc.Content) == 0 {
		return body, meta, nil // nothing but comments, or nothing at all
	}
	n := doc.Content[0]
	if n.Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("line %d: front matter is %s, not a mapping", n.Line, describe(n))
	}
	timestampsAsText(n)
	var v any
	if err := n.Decode(&v); err != nil {
		return nil, nil, fmt.Errorf("front matter: %w", err)
	}
	v, err := jsonValue(v)
	if err != nil {
		return nil, nil, fmt.Errorf("line %d: front matter: %w", n.Line, err)
	}
	meta, ok = v.(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("line %d: front matter is not a mapping", n.Line)
	}
	return body, meta, nil
}

// cutLine cuts the first line off s, without its line ending ("\n" or
// "\r\n"); the last line may have none. It reports false when s is empty.
func cutLine(s []byte) (line, rest []byte, ok bool) {
	if len(s) == 0 {
		return nil, nil, false
	}
	line, rest, _ = bytes.Cut(s, []byte("\n"))
	return bytes.TrimSuffix(line, []byte("\r")), rest, true
}

// timestampsAsText makes every timestamp in the YAML a string: a date as it
// is written, and a date with a time in RFC 3339. JSON has no timestamps, and
// a date read as midnight UTC would not read as the date that was written.
func timestampsAsText(n *yaml.Node) {
	if n.Kind == yaml.ScalarNode && n.ShortTag() == "!!timestamp" {
		var t time.Time
		if err := n.Decode(&t); err == nil {
			if _, err := time.Parse(time.DateOnly, n.Value); err != nil {
				n.Value = t.Format(time.RFC3339Nano)
			}
			n.Tag, n.Style = "!!str", 0
		}
	}
	for _, c := range n.Content {
		timestampsAsText(c)
	}
}

// describe names the kind of a YAML node that is not a mapping.
func describe(n *yaml.Node) string {
	switch n.Kind {
	case yaml.SequenceNode:
		return "a sequence"
	case yaml.ScalarNode:
		return "a scalar"
	case yaml.AliasNode:
		return "an alias"
	default:
		return "not a mapping"
	}
}

// jsonValue converts a decoded YAML value into one that marshals to JSON as
// it reads: maps get string keys, and timestamps become RFC 3339 strings.
func jsonValue(v any) (any, error) {
	switch x := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(x))
		for k, e := range x {
			e, err := jsonValue(e)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", k, err)
			}
			m[k] = e
		}
		return m, nil
	case map[any]any:
		m := make(map[string]any, len(x))
		for k, e := range x {
			key := fmt.Sprint(k)
			e, err := jsonValue(e)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", key, err)
			}
			m[key] = e
		}
		return m, nil
	case []any:
		s := make([]any, len(x))
		for i, e := range x {
			e, err := jsonValue(e)
			if err != nil {
				return nil, fmt.Errorf("%d: %w", i, err)
			}
			s[i] = e
		}
		return s, nil
	case time.Time:
		return x.Format(time.RFC3339Nano), nil
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return nil, fmt.Errorf("%v has no JSON form", x)
		}
		return x, nil
	default:
		return x, nil
	}
}
