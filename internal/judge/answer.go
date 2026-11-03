package judge

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Answer is a judge's answer to one question, checked.
type Answer struct {
	Verdict      string `json:"verdict"`
	Reason       string `json:"reason"`
	EvidenceLine int    `json:"evidence_line"`
}

// errFailed marks what the command itself reported as a failure, as opposed
// to an answer that is not shaped as one.
var errFailed = errors.New("the command reported a failure")

// decodeOutput finds the answer in what the command printed: either the
// answer itself, or the result object of claude -p --output-format json,
// whose structured_output, or else its result, holds the answer.
func decodeOutput(out []byte) ([]byte, error) {
	out = bytes.TrimSpace(out)
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(out, &obj); err != nil {
		return nil, errors.New("the output is not a JSON object")
	}
	var typ string
	if raw, ok := obj["type"]; !ok || json.Unmarshal(raw, &typ) != nil || typ != "result" {
		return out, nil
	}
	var isError bool
	if raw, ok := obj["is_error"]; ok {
		_ = json.Unmarshal(raw, &isError)
	}
	var result string
	if raw, ok := obj["result"]; ok {
		_ = json.Unmarshal(raw, &result)
	}
	if isError {
		return nil, fmt.Errorf("%w: %s", errFailed, tail(result, 500))
	}
	if raw, ok := obj["structured_output"]; ok && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return raw, nil
	}
	if result == "" {
		return nil, errors.New("the result object has neither structured_output nor result")
	}
	return []byte(stripFence(result)), nil
}

// stripFence drops a Markdown code fence around a JSON answer, which a model
// asked for JSON alone may still add.
func stripFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[i+1:] // the info string, such as json
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "```"))
}

// checkAnswer decodes an answer and checks it against AnswerSchema and the
// lines that were sent: a yes must rest on one of them. A no has its
// evidence line set to 0, whatever it said.
func checkAnswer(raw []byte, sent []int) (Answer, error) {
	var a struct {
		Verdict      *string `json:"verdict"`
		Reason       *string `json:"reason"`
		EvidenceLine *int    `json:"evidence_line"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return Answer{}, fmt.Errorf("the answer does not match the schema: %w", err)
	}
	if dec.More() {
		return Answer{}, errors.New("the answer is followed by more JSON")
	}
	switch {
	case a.Verdict == nil || a.Reason == nil || a.EvidenceLine == nil:
		return Answer{}, errors.New("the answer must have verdict, reason and evidence_line")
	case *a.Verdict != "yes" && *a.Verdict != "no":
		return Answer{}, fmt.Errorf("verdict is %q, not yes or no", *a.Verdict)
	case strings.TrimSpace(*a.Reason) == "":
		return Answer{}, errors.New("reason is empty")
	case *a.EvidenceLine < 0:
		return Answer{}, fmt.Errorf("evidence_line is %d, below 0", *a.EvidenceLine)
	}
	ans := Answer{Verdict: *a.Verdict, Reason: strings.TrimSpace(*a.Reason), EvidenceLine: *a.EvidenceLine}
	if ans.Verdict == "no" {
		ans.EvidenceLine = 0
	} else if !slices.Contains(sent, ans.EvidenceLine) {
		return Answer{}, fmt.Errorf("evidence_line %d is not a line that was sent", ans.EvidenceLine)
	}
	return ans, nil
}

// tail returns the last n bytes of s, at most, without splitting a UTF-8
// character.
func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	s = s[len(s)-n:]
	for len(s) > 0 && s[0]&0xC0 == 0x80 {
		s = s[1:]
	}
	return "..." + s
}
