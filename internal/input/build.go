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
	"github.com/babarot/regomark/internal/glob"
)

// Build reads the documents the configuration names from the repository at
// root and resolves their links.
func Build(root string, cfg *config.Config) (*Input, error) {
	paths, err := documentPaths(root, cfg)
	if err != nil {
		return nil, err
	}
	in := &Input{SchemaVersion: SchemaVersion, Root: ".", Documents: []Document{}, Judgments: []Judgment{}, Sources: map[string][]byte{}}
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
		in.Sources[p] = src
	}
	if err := resolveLinks(root, in.Documents); err != nil {
		return nil, err
	}
	return in, nil
}

// documentPaths returns the root-relative paths of the documents the
// configuration names, sorted, leaving out the excluded ones and the ones git
// ignores.
func documentPaths(root string, cfg *config.Config) ([]string, error) {
	var (
		paths []string
		err   error
	)
	if cfg.Documents.Patterns == nil {
		paths, err = dirPaths(root, cfg.Documents.Dir)
	} else {
		paths, err = matchingPaths(root, cfg.Documents.Patterns)
	}
	if err != nil {
		return nil, err
	}
	paths = slices.DeleteFunc(paths, cfg.Excluded)
	ignored, err := gitrepo.Ignored(root, paths)
	if err != nil {
		return nil, err
	}
	paths = slices.DeleteFunc(paths, func(p string) bool { return ignored[p] })
	// A document that cannot be read stops the check rather than dropping
	// out of it, which would let the rest pass
	for _, p := range paths {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			return nil, fmt.Errorf("documents: cannot read %s: %w", p, unwrapPath(err))
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("documents: %s is not a regular file", p)
		}
	}
	if len(paths) == 0 {
		if cfg.Documents.Patterns == nil {
			return nil, fmt.Errorf("documents: no Markdown documents to check in %s", cfg.Documents)
		}
		return nil, fmt.Errorf("documents: no Markdown documents match %s", cfg.Documents)
	}
	slices.Sort(paths)
	return paths, nil
}

// dirPaths returns the root-relative paths of the Markdown files right in
// the directory dir.
func dirPaths(root, dir string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(dir)))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("documents: %s does not exist", dir)
		}
		return nil, fmt.Errorf("documents: %w", err)
	}
	var paths []string
	for _, e := range entries {
		if !e.IsDir() && isMarkdown(e.Name()) {
			paths = append(paths, path.Join(dir, e.Name()))
		}
	}
	return paths, nil
}

// matchingPaths returns the root-relative paths of the Markdown files in the
// worktree that match one of the patterns. It lists what git lists, so it
// does not go into what git ignores, .git or a nested repository such as a
// submodule.
func matchingPaths(root string, patterns []string) ([]string, error) {
	files, err := gitrepo.Files(root)
	if err != nil {
		return nil, fmt.Errorf("documents: %w", err)
	}
	var paths []string
	for _, f := range files {
		if !isMarkdown(path.Base(f)) || !slices.ContainsFunc(patterns, func(pat string) bool { return glob.Match(pat, f) }) {
			continue
		}
		// Tracked but deleted, or outside a sparse checkout: not in the
		// worktree, so not a document
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(f))); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		paths = append(paths, f)
	}
	// An unmerged file is listed once per stage
	slices.Sort(paths)
	return slices.Compact(paths), nil
}

func isMarkdown(name string) bool {
	return strings.HasSuffix(name, ".md")
}

// unwrapPath drops the absolute path from a file system error, which names
// the document already.
func unwrapPath(err error) error {
	if pe, ok := errors.AsType[*fs.PathError](err); ok {
		return pe.Err
	}
	return err
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
