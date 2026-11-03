package docs

import data.lib.links

deny contains {"rule": "BROKEN", "path": doc.path, "line": l.line, "message": l.dest} if {
	some doc in input.documents
	some l in doc.links
	links.broken(l)
}

# Wrong: deny must not depend on the judgments
deny contains {"rule": "SPEC", "path": j.path, "line": j.line, "message": "judged"} if {
	some j in input.judgments
}
