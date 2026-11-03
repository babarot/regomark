package docs

schema_version := 2

rule_ids := {"HEADING", "LINK", "BROKEN"}

# Misspelled: a set nothing collects, in a file exempt by its name alone
denies contains {"rule": "HEADING", "path": "docs/doc.md", "line": 1, "message": "lost"} if true
