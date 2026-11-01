package docs

warn contains {"rule": "HEADING", "path": d.path, "line": 1, "message": "printed"} if {
	some d in input.documents
	print("checking", d.path)
}
