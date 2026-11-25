package docs

# Asks about the block under each "## Decision"
questions contains {"id": "spec-detail", "path": d.path, "line": h.line, "blocks": [d.blocks[i + 1]], "ask": "Does it name functions?"} if {
	some d in input.documents
	some i, h in d.blocks
	h.kind == "heading"
	h.text == "Decision"
}

warn contains {"rule": "SPEC", "path": j.path, "line": j.evidence_line, "message": j.reason} if {
	some j in input.judgments
	j.status == "ok"
	j.verdict == "yes"
}

# Without --judge every question is not run
warn contains {"rule": "NOT_RUN", "path": j.path, "line": j.line, "message": sprintf("%s was not asked", [j.id])} if {
	some j in input.judgments
	j.status == "not_run"
}

warn contains {"rule": "FAILED", "path": j.path, "line": j.line, "message": j.error} if {
	some j in input.judgments
	j.status in {"provider_failed", "invalid_answer"}
}
