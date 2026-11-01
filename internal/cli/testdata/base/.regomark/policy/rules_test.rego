package docs_test

import data.docs

# Not a policy: regomark must not load it, or it would break the package rules
test_heading if {
	count(docs.warn) == 1 with input as {"documents": [{"path": "a.md", "blocks": [{"kind": "heading", "level": 1, "line": 1, "text": "A"}], "links": []}]}
}
