package docs

# No module defines package lib.nothing
deny contains {"rule": "HEADING", "path": "docs/doc.md", "line": 1, "message": "never"} if {
	data.lib.nothing.here == 1
}
