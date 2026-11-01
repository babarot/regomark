package docs

import data.lib.links

# A default yields no violation, so it does not make this module a policy
default deny := set()

denies contains {"rule": "BROKEN", "path": doc.path, "line": l.line, "message": l.dest} if {
	some doc in input.documents
	some l in doc.links
	links.broken(l)
}
