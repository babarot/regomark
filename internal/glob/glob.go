// Package glob matches root-relative, slash-separated paths against
// patterns: path.Match patterns in which a segment that is exactly ** matches
// any number of directories, none included.
package glob

import (
	"path"
	"strings"
)

// HasMeta reports whether s has any of the characters that make it a
// pattern rather than a path.
func HasMeta(s string) bool {
	return strings.ContainsAny(s, "*?[")
}

// Validate reports whether pattern is malformed, returning
// path.ErrBadPattern if it is.
func Validate(pattern string) error {
	if !recursive(pattern) {
		_, err := path.Match(pattern, "")
		return err
	}
	for seg := range strings.SplitSeq(pattern, "/") {
		if _, err := path.Match(seg, ""); err != nil {
			return err
		}
	}
	return nil
}

// Match reports whether name matches pattern. A malformed pattern matches
// nothing. A pattern without a ** segment matches exactly as path.Match does.
func Match(pattern, name string) bool {
	if !recursive(pattern) {
		ok, _ := path.Match(pattern, name)
		return ok
	}
	return matchSegments(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

// recursive reports whether pattern has a ** segment.
func recursive(pattern string) bool {
	for seg := range strings.SplitSeq(pattern, "/") {
		if seg == "**" {
			return true
		}
	}
	return false
}

func matchSegments(pat, name []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			for len(pat) > 0 && pat[0] == "**" {
				pat = pat[1:]
			}
			if len(pat) == 0 {
				return true
			}
			for i := range len(name) + 1 {
				if matchSegments(pat, name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		if ok, _ := path.Match(pat[0], name[0]); !ok {
			return false
		}
		pat, name = pat[1:], name[1:]
	}
	return len(name) == 0
}
