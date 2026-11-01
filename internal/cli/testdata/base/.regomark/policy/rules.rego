package docs

import data.lib.links

# Every heading, so that a test sees which headings the input has
warn contains {"rule": "HEADING", "path": doc.path, "line": b.line, "message": sprintf("h%d %s", [b.level, b.text])} if {
	some doc in input.documents
	some b in doc.blocks
	b.kind == "heading"
}

# Every link that resolves, with what it resolved to
warn contains {"rule": "LINK", "path": doc.path, "line": l.line, "message": sprintf("%s %s -> %s", [l.kind, l.dest, l.target])} if {
	some doc in input.documents
	some l in doc.links
	not links.broken(l)
}

deny contains {"rule": "BROKEN", "path": doc.path, "line": l.line, "message": sprintf("%s %s -> %s", [l.kind, l.dest, l.target])} if {
	some doc in input.documents
	some l in doc.links
	links.broken(l)
}
