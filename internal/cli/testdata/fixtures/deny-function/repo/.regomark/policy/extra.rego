package docs

# A function: OPA leaves it out of data.docs, so nothing would collect it
deny(doc) := {"rule": "HEADING", "path": doc.path, "line": 1, "message": "never"}
