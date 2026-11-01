// Package input builds what the policies receive as input: the Markdown
// documents as blocks and links, each with its line, and every link resolved
// against the repository.
package input

// SchemaVersion is the version of the input this regomark produces. A policy
// set declares the version it was written for, and regomark refuses to
// evaluate it against any other.
const SchemaVersion = 1

// Input is the whole input of the policies.
type Input struct {
	// SchemaVersion is the version of this structure.
	SchemaVersion int `json:"schema_version" jsonschema:"enum=1"`
	// Root is the repository root that every path is relative to. It is
	// always ".", so the input does not depend on where the repository is.
	Root string `json:"root" jsonschema:"enum=."`
	// Documents are the Markdown documents, sorted by path.
	Documents []Document `json:"documents"`
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
