package docs

deny contains {"rule": "TITLE", "path": doc.path, "line": 1, "message": "front matter has no title"} if {
	some doc in input.documents
	not doc.meta.title
}
