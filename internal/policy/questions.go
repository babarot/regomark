package policy

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/open-policy-agent/opa/v1/ast"

	"github.com/babarot/regomark/internal/input"
)

// Question is one question a policy asks about a document: whether the
// blocks, sent as they are in the source, answer yes or no to ask.
type Question struct {
	ID     string        `json:"id"`
	Path   string        `json:"path"`
	Line   int           `json:"line"`
	Blocks []input.Block `json:"blocks"`
	Ask    string        `json:"ask"`
}

// questionFields are the fields of a question, all required.
var questionFields = []string{"ask", "blocks", "id", "line", "path"}

var questionID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Questions evaluates the questions the policy set asks about the input,
// whose judgments must be empty, and returns them sorted by path, id and
// line. A policy set that defines no questions asks none.
//
// Every question must be an object with exactly id (lowercase letters,
// digits and hyphens), path (a document of the input), line (from 1), blocks
// (one or more blocks of that document, as the input has them) and ask (not
// empty), and no two may share id, path and line.
func (s *Set) Questions(ctx context.Context, in *input.Input) ([]Question, error) {
	if len(in.Judgments) > 0 {
		return nil, errors.New("the input to ask about must have no judgments")
	}
	if err := s.checkSchemaVersion(ctx, in); err != nil {
		return nil, err
	}
	schemas, err := schemaOption()
	if err != nil {
		return nil, err
	}
	t, err := s.eval(ctx, in, fmt.Sprintf("data.%s.%s", s.entrypoint, questionsRule), schemas)
	if err != nil {
		return nil, err
	}
	qs := []Question{}
	if t == nil {
		return qs, nil
	}
	name := fmt.Sprintf("data.%s.%s", s.entrypoint, questionsRule)
	set, ok := t.Value.(ast.Set)
	if !ok {
		return nil, fmt.Errorf("%s: must be a set, not %s", name, ast.ValueName(t.Value))
	}
	docs := map[string][]input.Block{}
	for _, d := range in.Documents {
		docs[d.Path] = d.Blocks
	}
	var errs []error
	seen := map[[3]string]bool{}
	for _, e := range set.Slice() {
		q, err := question(e, docs)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %s: %w", name, describe(e), err))
			continue
		}
		key := [3]string{q.ID, q.Path, fmt.Sprint(q.Line)}
		if seen[key] {
			errs = append(errs, fmt.Errorf("%s: more than one question %q at %s:%d", name, q.ID, q.Path, q.Line))
			continue
		}
		seen[key] = true
		qs = append(qs, q)
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	slices.SortFunc(qs, func(a, b Question) int {
		return cmp.Or(strings.Compare(a.Path, b.Path), strings.Compare(a.ID, b.ID), cmp.Compare(a.Line, b.Line))
	})
	return qs, nil
}

// describe names a question in an error by its id, path and line when it
// has them, rather than printing the blocks it carries.
func describe(t *ast.Term) string {
	obj, ok := t.Value.(ast.Object)
	if !ok {
		return t.String()
	}
	id, path, line := obj.Get(ast.StringTerm("id")), obj.Get(ast.StringTerm("path")), obj.Get(ast.StringTerm("line"))
	if id == nil || path == nil || line == nil {
		return "a question"
	}
	return fmt.Sprintf("question %v at %v:%v", id, path, line)
}

func question(t *ast.Term, docs map[string][]input.Block) (Question, error) {
	obj, ok := t.Value.(ast.Object)
	if !ok {
		return Question{}, fmt.Errorf("a question must be an object with %s, not %s", strings.Join(questionFields, ", "), ast.ValueName(t.Value))
	}
	var extra, missing []string
	for _, k := range obj.Keys() {
		if s, ok := k.Value.(ast.String); !ok || !slices.Contains(questionFields, string(s)) {
			extra = append(extra, k.String())
		}
	}
	for _, f := range questionFields {
		if obj.Get(ast.StringTerm(f)) == nil {
			missing = append(missing, f)
		}
	}
	if len(extra) > 0 {
		return Question{}, fmt.Errorf("unknown field %s; a question has %s", strings.Join(extra, ", "), strings.Join(questionFields, ", "))
	}
	if len(missing) > 0 {
		return Question{}, fmt.Errorf("%s is missing", strings.Join(missing, ", "))
	}
	var q Question
	id, ok := obj.Get(ast.StringTerm("id")).Value.(ast.String)
	if !ok || !questionID.MatchString(string(id)) {
		return q, errors.New("id must be a string of lowercase letters, digits and hyphens, starting with a letter or a digit")
	}
	q.ID = string(id)
	path, ok := obj.Get(ast.StringTerm("path")).Value.(ast.String)
	if !ok {
		return q, errors.New("path must be a string")
	}
	q.Path = string(path)
	blocks, ok := docs[q.Path]
	if !ok {
		return q, fmt.Errorf("path %q is not a document of the input", q.Path)
	}
	n, ok := obj.Get(ast.StringTerm("line")).Value.(ast.Number)
	if !ok {
		return q, errors.New("line must be an integer from 1")
	}
	if q.Line, ok = n.Int(); !ok || q.Line < 1 {
		return q, errors.New("line must be an integer from 1")
	}
	ask, ok := obj.Get(ast.StringTerm("ask")).Value.(ast.String)
	if !ok || strings.TrimSpace(string(ask)) == "" {
		return q, errors.New("ask must be a non-empty string")
	}
	q.Ask = string(ask)
	arr, ok := obj.Get(ast.StringTerm("blocks")).Value.(*ast.Array)
	if !ok || arr.Len() == 0 {
		return q, errors.New("blocks must be a non-empty array of blocks of the document")
	}
	for i := range arr.Len() {
		b, err := block(arr.Elem(i))
		if err != nil || !slices.Contains(blocks, b) {
			// Only a block as the input has it carries lines that match the
			// source, which is what the judge is sent
			return q, fmt.Errorf("blocks[%d] is not a block of %s as the input has it", i, q.Path)
		}
		q.Blocks = append(q.Blocks, b)
	}
	return q, nil
}

// block decodes a block from its term, rejecting unknown fields.
func block(t *ast.Term) (input.Block, error) {
	v, err := ast.JSON(t.Value)
	if err != nil {
		return input.Block{}, err
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return input.Block{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var b input.Block
	if err := dec.Decode(&b); err != nil {
		return input.Block{}, err
	}
	return b, nil
}

// NotRun returns the judgments of questions that were not asked.
func NotRun(qs []Question) []input.Judgment {
	js := make([]input.Judgment, 0, len(qs))
	for _, q := range qs {
		js = append(js, input.Judgment{ID: q.ID, Path: q.Path, Line: q.Line, Status: input.StatusNotRun})
	}
	return js
}
