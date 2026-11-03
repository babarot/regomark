package docs

# Asks about the block under each "## Decision"
questions contains {"id": "spec-detail", "path": d.path, "line": h.line, "blocks": [d.blocks[i + 1]], "ask": "Does it name functions?"} if {
	some d in input.documents
	some i, h in d.blocks
	h.kind == "heading"
	h.text == "Decision"
}
