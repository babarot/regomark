package docs

import data.lib.h

# Misspelled: h.alowed is undefined, so this rule would never fire
deny contains {"rule": "HEADING", "path": "docs/doc.md", "line": 1, "message": "not allowed"} if {
	not "Proposed" in h.alowed
}
