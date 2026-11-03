// Package input builds what the policies receive as input: the Markdown
// documents as blocks and links, each with its line, every link resolved
// against the repository, and the answers to the policies' questions.
package input

// SchemaVersion is the version of the input this regomark produces. A policy
// set declares the version it was written for, and regomark refuses to
// evaluate it against any other.
const SchemaVersion = 2

// Input is the whole input of the policies.
type Input struct {
	// SchemaVersion is the version of this structure.
	SchemaVersion int `json:"schema_version" jsonschema:"enum=2"`
	// Root is the repository root that every path is relative to. It is
	// always ".", so the input does not depend on where the repository is.
	Root string `json:"root" jsonschema:"enum=."`
	// Documents are the Markdown documents, sorted by path.
	Documents []Document `json:"documents"`
	// Judgments are the answers to the questions the policies ask, sorted
	// by path, id and line. They are empty while the questions and deny are
	// evaluated, so that neither depends on them.
	Judgments []Judgment `json:"judgments"`

	// Sources are the documents as read, by path. They are not part of the
	// input the policies see; a judge is sent lines of them.
	Sources map[string][]byte `json:"-"`
}

// Document is one Markdown document.
type Document struct {
	// Path is the root-relative path of the document.
	Path string `json:"path" jsonschema:"minLength=1"`
	// Name is the file name of the document.
	Name string `json:"name" jsonschema:"minLength=1"`
	// Blocks are the top-level blocks in document order.
	Blocks []Block `json:"blocks"`
	// Links are all the links in the document, reference-style ones
	// included, in document order.
	Links []Link `json:"links"`
}

// Block is one top-level block of a document.
type Block struct {
	// Kind is what the block is.
	Kind string `json:"kind" jsonschema:"enum=heading,enum=paragraph,enum=code,enum=list,enum=quote,enum=html,enum=rule,enum=table"`
	// Level is the level of a heading (1 to 6), and 0 for any other block.
	Level int `json:"level" jsonschema:"minimum=0,maximum=6"`
	// Line is the line the block starts at, counting from 1. For a fenced
	// code block it is the line of the opening fence.
	Line int `json:"line" jsonschema:"minimum=1"`
	// EndLine is the last line of the block, its last child and a closing
	// fence included, and blank lines after it left out. It is Line for a
	// block of one line.
	EndLine int `json:"end_line" jsonschema:"minimum=1"`
	// Text is the plain text of the block, with code spans in backquotes and
	// the line breaks of the source kept. A code or HTML block has its
	// lines as they are; a rule has none.
	Text string `json:"text"`
	// Lang is the language of a fenced code block, and empty otherwise.
	Lang string `json:"lang"`
}

// Link is one link in a document.
type Link struct {
	// Dest is the destination as written, or as defined for a
	// reference-style link.
	Dest string `json:"dest"`
	// Text is the plain text of the link.
	Text string `json:"text"`
	// Line is the line the link starts at.
	Line int `json:"line" jsonschema:"minimum=1"`
	// Target is the root-relative path the link points to, without any
	// fragment or query. It is empty for an external link.
	Target string `json:"target"`
	// Kind is what the target is in the worktree: file, dir, missing (it does
	// not exist or git ignores it), outside (it is outside the repository)
	// or external (the destination has a scheme or is only a fragment).
	Kind string `json:"kind" jsonschema:"enum=file,enum=dir,enum=missing,enum=outside,enum=external"`
}

// Link kinds.
const (
	KindFile     = "file"
	KindDir      = "dir"
	KindMissing  = "missing"
	KindOutside  = "outside"
	KindExternal = "external"
)

// Judgment is the answer to one question a policy asks.
type Judgment struct {
	// ID, Path and Line are those of the question.
	ID   string `json:"id" jsonschema:"minLength=1"`
	Path string `json:"path" jsonschema:"minLength=1"`
	Line int    `json:"line" jsonschema:"minimum=1"`
	// Status is ok when the question was answered, not_run when it was not
	// asked, provider_failed when asking failed, and invalid_answer when the
	// answer was not shaped as one.
	Status string `json:"status" jsonschema:"enum=ok,enum=not_run,enum=provider_failed,enum=invalid_answer"`
	// Verdict is yes or no when Status is ok, and empty otherwise.
	Verdict string `json:"verdict" jsonschema:"enum=,enum=yes,enum=no"`
	// Reason is why, in a sentence, when Status is ok.
	Reason string `json:"reason"`
	// EvidenceLine is the line the answer rests on when Verdict is yes, and
	// 0 otherwise.
	EvidenceLine int `json:"evidence_line" jsonschema:"minimum=0"`
	// Error is what went wrong when Status is provider_failed or
	// invalid_answer, and empty otherwise.
	Error string `json:"error"`
}

// Judgment statuses.
const (
	StatusOK            = "ok"
	StatusNotRun        = "not_run"
	StatusFailed        = "provider_failed"
	StatusInvalidAnswer = "invalid_answer"
)
