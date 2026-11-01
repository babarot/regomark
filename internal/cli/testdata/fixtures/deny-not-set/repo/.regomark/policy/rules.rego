package docs

import data.lib.links

# An array, not the set that deny contains ... makes
deny := [{"rule": "BROKEN", "path": "docs/doc.md", "line": 1, "message": "an array"}] if links.broken({"kind": "missing"})
