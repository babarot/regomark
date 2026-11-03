package judge

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/babarot/regomark/internal/input"
	"github.com/babarot/regomark/internal/policy"
)

// Questions is what regomark questions prints: every question with the
// prompt judge.command would get, for a judge that is not a command run per
// question, such as an agent in a CI job.
type Questions struct {
	PromptVersion int                `json:"prompt_version"`
	AnswerSchema  json.RawMessage    `json:"answer_schema"`
	Questions     []ExportedQuestion `json:"questions"`
}

// ExportedQuestion is one question as written out. An answer names it by
// Key, which changes whenever what it sends does.
type ExportedQuestion struct {
	Key    string `json:"key"`
	ID     string `json:"id"`
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Prompt string `json:"prompt"`
}

// Export returns the questions about the documents in sources, as written
// out.
func Export(qs []policy.Question, sources map[string][]byte) (*Questions, error) {
	out := &Questions{PromptVersion: PromptVersion, AnswerSchema: json.RawMessage(AnswerSchema), Questions: []ExportedQuestion{}}
	for _, q := range qs {
		src, ok := sources[q.Path]
		if !ok {
			return nil, fmt.Errorf("no source for %s", q.Path)
		}
		prompt, _, err := Prompt(q, src)
		if err != nil {
			return nil, err
		}
		out.Questions = append(out.Questions, ExportedQuestion{Key: Key(prompt), ID: q.ID, Path: q.Path, Line: q.Line, Prompt: prompt})
	}
	return out, nil
}

// FileStats counts what reading a file of answers found.
type FileStats struct {
	Questions int // questions asked about
	Answered  int // answered by the file
	Failed    int // not answered, or answered with something that is not an answer
	Unmatched int // answers in the file that match no question
}

// FromFile answers the questions from data, a file of answers named name:
//
//	{"answers": [{"key": "sha256:...", "verdict": "yes", "reason": "...", "evidence_line": 16}]}
//
// An answer applies to the question with its key, which comes from what the
// question sends: if a document or a policy has changed since the questions
// were written out, the old answer matches nothing. A question without an
// answer is provider_failed, and an answer that is not shaped as one
// invalid_answer. A file that is not shaped as one, or that answers a
// question twice, is an error.
func FromFile(qs []policy.Question, sources map[string][]byte, data []byte, name string) ([]input.Judgment, FileStats, error) {
	var file struct {
		Answers []json.RawMessage `json:"answers"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&file); err != nil {
		return nil, FileStats{}, fmt.Errorf("%s: %w", name, err)
	}
	if file.Answers == nil {
		return nil, FileStats{}, fmt.Errorf("%s: no answers; the file is {\"answers\": [...]}", name)
	}
	byKey := map[string]json.RawMessage{}
	for i, raw := range file.Answers {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return nil, FileStats{}, fmt.Errorf("%s: answers[%d] is not an object", name, i)
		}
		var key string
		if err := json.Unmarshal(fields["key"], &key); err != nil || key == "" {
			return nil, FileStats{}, fmt.Errorf("%s: answers[%d] has no key", name, i)
		}
		if _, dup := byKey[key]; dup {
			return nil, FileStats{}, fmt.Errorf("%s: more than one answer for %s", name, key)
		}
		delete(fields, "key")
		rest, err := json.Marshal(fields)
		if err != nil {
			return nil, FileStats{}, err
		}
		byKey[key] = rest
	}

	stats := FileStats{Questions: len(qs)}
	judgments := make([]input.Judgment, len(qs))
	used := map[string]bool{}
	for i, q := range qs {
		judgments[i] = input.Judgment{ID: q.ID, Path: q.Path, Line: q.Line}
		src, ok := sources[q.Path]
		if !ok {
			return nil, FileStats{}, fmt.Errorf("no source for %s", q.Path)
		}
		prompt, sent, err := Prompt(q, src)
		if err != nil {
			return nil, FileStats{}, err
		}
		key := Key(prompt)
		raw, ok := byKey[key]
		if !ok {
			judgments[i].Status, judgments[i].Error = input.StatusFailed, "no answer in "+name
			stats.Failed++
			continue
		}
		used[key] = true
		a, err := checkAnswer(raw, sent)
		if err != nil {
			judgments[i].Status, judgments[i].Error = input.StatusInvalidAnswer, err.Error()
			stats.Failed++
			continue
		}
		setAnswer(&judgments[i], a)
		stats.Answered++
	}
	stats.Unmatched = len(byKey) - len(used)
	return judgments, stats, nil
}
