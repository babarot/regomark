// Package gitrepo asks git about the repository regomark runs in.
package gitrepo

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Root returns the top-level directory of the worktree that contains dir.
func Root(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --show-toplevel: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return filepath.FromSlash(strings.TrimSpace(string(out))), nil
}

// Ignored returns which of the root-relative paths git ignores. Tracked
// paths are never ignored, as in git check-ignore.
func Ignored(root string, paths []string) (map[string]bool, error) {
	ignored := map[string]bool{}
	if len(paths) == 0 {
		return ignored, nil
	}
	cmd := exec.Command("git", "check-ignore", "--stdin", "-z")
	cmd.Dir = root
	cmd.Stdin = strings.NewReader(strings.Join(paths, "\x00") + "\x00")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		// Exit status 1 means none of the paths is ignored
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return ignored, nil
		}
		return nil, fmt.Errorf("git check-ignore: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	for p := range strings.SplitSeq(string(out), "\x00") {
		if p != "" {
			ignored[p] = true
		}
	}
	return ignored, nil
}

// Files returns the root-relative paths of the files in the worktree that git
// does not ignore: the tracked ones and the untracked ones not ignored, as git
// ls-files lists them. It does not look inside .git, nor inside a nested
// repository such as a submodule, which it lists as one entry. A tracked
// file may be missing from the worktree.
func Files(root string) ([]string, error) {
	cmd := exec.Command("git", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var files []string
	for p := range strings.SplitSeq(string(out), "\x00") {
		if p != "" {
			files = append(files, p)
		}
	}
	return files, nil
}
