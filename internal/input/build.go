package input

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/babarot/regomark/internal/config"
	"github.com/babarot/regomark/internal/gitrepo"
)

// Build reads the documents the configuration names from the repository at
// root and resolves their links.
func Build(root string, cfg *config.Config) (*Input, error) {
	paths, err := documentPaths(root, cfg)
	if err != nil {
		return nil, err
	}
	in := &Input{SchemaVersion: SchemaVersion, Root: ".", Documents: []Document{}}
	for _, p := range paths {
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			return nil, err
		}
		blocks, links, err := Parse(src)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		in.Documents = append(in.Documents, Document{Path: p, Name: path.Base(p), Blocks: blocks, Links: links})
	}
	if err := resolveLinks(root, in.Documents); err != nil {
		return nil, err
	}
	return in, nil
}

// documentPaths returns the root-relative paths of the Markdown files right
// in the documents directory, sorted, leaving out the excluded ones and the
// ones git ignores.
func documentPaths(root string, cfg *config.Config) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(cfg.Documents)))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("documents: %s does not exist", cfg.Documents)
		}
		return nil, fmt.Errorf("documents: %w", err)
	}
	var paths []string
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		p := path.Join(cfg.Documents, e.Name())
		if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(p))); err != nil || !info.Mode().IsRegular() {
			continue // a directory, or a symbolic link to nothing
		}
		if !cfg.Excluded(p) {
			paths = append(paths, p)
		}
	}
	ignored, err := gitrepo.Ignored(root, paths)
	if err != nil {
		return nil, err
	}
	paths = slices.DeleteFunc(paths, func(p string) bool { return ignored[p] })
	if len(paths) == 0 {
		return nil, fmt.Errorf("documents: no Markdown documents to check in %s", cfg.Documents)
	}
	slices.Sort(paths)
	return paths, nil
}

// resolveLinks sets the target and the kind of every link.
func resolveLinks(root string, docs []Document) error {
	var local []string
	for i := range docs {
		for j := range docs[i].Links {
			l := &docs[i].Links[j]
			target, external := resolve(docs[i].Path, l.Dest)
			l.Target = target
			switch {
			case external:
				l.Kind = KindExternal
			case outside(target):
				l.Kind = KindOutside
			default:
				if target != "." {
					local = append(local, target)
				}
			}
		}
	}
	slices.Sort(local)
	local = slices.Compact(local)
	ignored, err := gitrepo.Ignored(root, local)
	if err != nil {
		return err
	}
	w := worktree{root: root, dirs: map[string][]string{}}
	for i := range docs {
		for j := range docs[i].Links {
			l := &docs[i].Links[j]
			if l.Kind != "" {
				continue
			}
			switch {
			case ignored[l.Target]:
				// Not in a fresh clone, so missing here too: the result is the
				// same in a working copy and in CI
				l.Kind = KindMissing
			default:
				l.Kind = w.kind(l.Target)
			}
		}
	}
	return nil
}

// worktree looks up paths in the worktree with their case as written, so that
// a case-insensitive file system finds no more than a case-sensitive one.
type worktree struct {
	root string
	dirs map[string][]string
}

func (w *worktree) kind(target string) string {
	if target == "." {
		return KindDir
	}
	dir := "."
	for name := range strings.SplitSeq(target, "/") {
		names, ok := w.dirs[dir]
		if !ok {
			entries, err := os.ReadDir(filepath.Join(w.root, filepath.FromSlash(dir)))
			if err != nil {
				entries = nil // not a directory, or unreadable: nothing in it
			}
			names = make([]string, 0, len(entries))
			for _, e := range entries {
				names = append(names, e.Name())
			}
			w.dirs[dir] = names
		}
		if !slices.Contains(names, name) {
			return KindMissing
		}
		dir = path.Join(dir, name)
	}
	info, err := os.Stat(filepath.Join(w.root, filepath.FromSlash(target)))
	switch {
	case err != nil:
		return KindMissing // a broken symbolic link
	case info.IsDir():
		return KindDir
	default:
		return KindFile
	}
}
