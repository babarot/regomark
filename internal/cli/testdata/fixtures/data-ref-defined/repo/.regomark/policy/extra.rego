package docs

import data.lib.h

# Through the full path with a variable, and the whole package at once
warn contains {"rule": "HEADING", "path": "docs/doc.md", "line": 1, "message": sprintf("allowed %s", [x])} if {
	data.lib.h.allowed[x]
	count(h) == 1
	data.lib.h.allowed["Accepted"]
	data.lib
}
