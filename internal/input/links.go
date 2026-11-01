package input

import (
	"net/url"
	"path"
	"regexp"
	"strings"
)

// scheme matches a URI scheme at the start of a destination (RFC 3986).
var scheme = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)

// resolve returns the root-relative path a link destination in the document
// at docPath points to, and whether the link is external. The target of a
// link out of the repository starts with "../".
//
// The fragment and the query are dropped first, so that an encoded "#" or
// "?" stays part of the path, and the rest is percent-decoded. A destination
// that starts with "/" is relative to the repository root, any other one to
// the directory of the document.
func resolve(docPath, dest string) (target string, external bool) {
	if scheme.MatchString(dest) || strings.HasPrefix(dest, "#") {
		return "", true
	}
	p := dest
	if i := strings.IndexAny(p, "#?"); i >= 0 {
		p = p[:i]
	}
	if u, err := url.PathUnescape(p); err == nil {
		p = u
	}
	if p == "" {
		// An empty destination, or only a query: the document itself
		return docPath, false
	}
	if strings.HasPrefix(p, "/") {
		return path.Clean(strings.TrimLeft(p, "/")), false
	}
	return path.Clean(path.Join(path.Dir(docPath), p)), false
}

// outside reports whether a resolved target is out of the repository.
func outside(target string) bool {
	return target == ".." || strings.HasPrefix(target, "../")
}
