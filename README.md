# regomark

Check Markdown documents with rego policies.

regomark reads a directory of Markdown documents into JSON that keeps the line of every block and link, resolves each link against the repository, and evaluates rego policies against that JSON with the OPA engine built in. The rules (which sections a document must have, how documents refer to each other, which links must resolve) are written in rego and tested with `opa test`; regomark itself knows nothing about any document format.

It is under development.

## Usage

```
regomark check [--config .regomark.yaml] [--format text|github|json]
regomark dump [--config .regomark.yaml]
regomark version   # or --version
```

- `check` evaluates the policies against the documents and prints the violations. It exits 0 when there is no deny, 1 when there is one, and 2 when the check cannot be done: an invalid configuration, no documents, no policy, a policy that breaks the rules below, or a violation that is not shaped as one.
- `dump` prints the input the policies receive, as JSON, for writing policy tests.
- `--format`: `text` prints `path:line: [rule] message` (a warn as `path:line: warning: [rule] message`), `github` prints workflow commands that GitHub Actions shows as annotations (`::error` for a deny, `::warning` for a warn), and `json` prints an array of violations.

regomark runs in a git repository. Every path, `--config` included, is relative to the repository root (`git rev-parse --show-toplevel`), so it gives the same result from any directory in the repository.

### Configuration

`.regomark.yaml` at the repository root:

```yaml
documents: docs/adr          # the directory of the Markdown documents (*.md right in it)
policy: .regomark/policy     # the directory of the rego policies
entrypoint: adr              # the package of deny and warn (data.adr)
skip_rules: []               # rule IDs whose violations are dropped
exclude: []                  # documents not to check, as path.Match patterns (docs/adr/000-*.md)
```

An unknown key, or a rule ID in `skip_rules` that the policy set does not declare, exits 2. Documents that git ignores are not checked, and one that cannot be read, such as a broken symbolic link, exits 2.

### Policies

regomark loads every `.rego` file under the policy directory except `*_test.rego`, and evaluates `data.<entrypoint>`. A symbolic link to a directory inside the policy directory is an error, since its modules would otherwise go unchecked. It adds no built-in functions, so `opa test`, `opa check` and Regal work on the policies as they are.

- A module outside `lib/` is in the entrypoint package itself (`package adr`; not `package adr.links`, whose rules nothing would collect), and defines `deny` or `warn`, unless it defines nothing but `schema_version` and `rule_ids`.
- A module in `lib/` is a library: another loaded module imports it, and it defines neither `deny` nor `warn`.
- At least one module defines `deny` or `warn`.
- Every reference into `data` (through an import or a full path) names something a loaded module defines. regomark loads no other data, so a misspelled library constant is an error rather than a rule that never fires.
- The policy set defines `schema_version`, equal to the version of the input below, and `rule_ids`, the set of its rule IDs.
- `deny` and `warn` are sets of objects with exactly `rule` (one of `rule_ids`), `path`, `line` (from 1) and `message`. Either may be left undefined.

```rego
package adr

schema_version := 1

rule_ids := {"ADR001"}

deny contains {"rule": "ADR001", "path": doc.path, "line": 1, "message": "the file name must be NNN-title.md"} if {
	some doc in input.documents
	not regex.match(`^\d{3}-[a-z0-9-]+\.md$`, doc.name)
}
```

The input is validated against a JSON Schema generated from regomark's types before the policies see it, and the same schema type-checks the policies, so a misspelled input field is an error rather than a rule that never fires.

### Input

```json
{
  "schema_version": 1,
  "root": ".",
  "documents": [
    {
      "path": "docs/adr/001-a.md",
      "name": "001-a.md",
      "blocks": [
        {"kind": "heading", "level": 1, "line": 1, "text": "ADR-001: A", "lang": ""}
      ],
      "links": [
        {"dest": "../../internal/a_test.go#L3", "text": "TestA", "line": 17,
         "target": "internal/a_test.go", "kind": "file"}
      ]
    }
  ]
}
```

- `blocks` are the top-level blocks in document order. `kind` is one of `heading`, `paragraph`, `code`, `list`, `quote`, `html`, `rule` and `table`; `line` is where the block starts (the opening fence for fenced code); `text` is plain text with code spans in backquotes and the line breaks of the source kept (code and HTML blocks as they are); `level` is set for headings and `lang` for fenced code.
- `links` are every link in the document, reference-style links and autolinks included. `target` is the root-relative path the link points to: the destination without its fragment and query, percent-decoded, relative to the document (or to the repository root when it starts with `/`), and cleaned. `kind` is `file` or `dir` for what exists in the worktree, `missing` for what does not or what git ignores (it would not exist in a fresh clone), `outside` for a target out of the repository, and `external` for a destination with a scheme or only a fragment (`target` is then empty). A path is matched with its case as written, so a case-insensitive file system does not hide a broken link.

## Development

```bash
mise run build   # ./regomark
mise run test    # gofmt, go vet and go test with the race detector
mise run lint    # golangci-lint and go mod tidy
mise run vuln    # govulncheck
```

## License

[MIT](LICENSE)
