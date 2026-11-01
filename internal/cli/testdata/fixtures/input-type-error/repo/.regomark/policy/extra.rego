package docs

# A misspelled input field: the schema turns it into a type error
warn contains {"rule": "HEADING", "path": d.path, "line": 1, "message": "never"} if {
	some d in input.documents
	d.titel == "x"
}
