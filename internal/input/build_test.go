package input

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/babarot/regomark/internal/config"
)

func TestWorktreeKind(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "a.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("nowhere", filepath.Join(root, "docs", "broken")); err != nil {
		t.Fatal(err)
	}
	w := worktree{root: root, dirs: map[string][]string{}}
	for target, want := range map[string]string{
		".":            KindDir,
		"docs":         KindDir,
		"docs/sub":     KindDir,
		"docs/a.md":    KindFile,
		"docs/A.md":    KindMissing, // even on a case-insensitive file system
		"Docs/a.md":    KindMissing,
		"docs/b.md":    KindMissing,
		"docs/a.md/x":  KindMissing,
		"docs/broken":  KindMissing,
		"missing/a.md": KindMissing,
	} {
		if got := w.kind(target); got != want {
			t.Errorf("kind(%q) = %s, want %s", target, got, want)
		}
	}
}

func TestBuild(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	root := t.TempDir()
	files := map[string]string{
		".gitignore":       "docs/local*.md\nout/\n",
		"docs/b.md":        "# B\n\n[a](a.md) [out](../out/x.txt) [ext](https://example.com)\n",
		"docs/a.md":        "# A\n",
		"docs/local.md":    "# ignored document\n",
		"docs/000-tmpl.md": "# excluded document\n",
		"docs/notes.txt":   "not Markdown\n",
		"docs/sub/c.md":    "# not right in the directory\n",
		"out/x.txt":        "x\n",
	}
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A symbolic link to a document is a document
	if err := os.Symlink("a.md", filepath.Join(root, "docs", "c.md")); err != nil {
		t.Fatal(err)
	}
	// One to nothing cannot be read; ignored or excluded, it is not checked
	for _, name := range []string{"local-broken.md", "000-broken.md"} {
		if err := os.Symlink("nowhere.md", filepath.Join(root, "docs", name)); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	in, err := Build(root, &config.Config{Documents: config.Documents{Dir: "docs"}, Exclude: []string{"docs/000-*.md"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(in); err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, d := range in.Documents {
		paths = append(paths, d.Path)
	}
	if want := []string{"docs/a.md", "docs/b.md", "docs/c.md"}; !reflect.DeepEqual(paths, want) {
		t.Errorf("documents = %v, want %v", paths, want)
	}
	links := in.Documents[1].Links
	want := []Link{
		{Dest: "a.md", Text: "a", Line: 3, Target: "docs/a.md", Kind: KindFile},
		{Dest: "../out/x.txt", Text: "out", Line: 3, Target: "out/x.txt", Kind: KindMissing},
		{Dest: "https://example.com", Text: "ext", Line: 3, Target: "", Kind: KindExternal},
	}
	if !reflect.DeepEqual(links, want) {
		t.Errorf("links:\n got %+v\nwant %+v", links, want)
	}

	if err := os.Symlink("nowhere.md", filepath.Join(root, "docs", "e.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(root, &config.Config{Documents: config.Documents{Dir: "docs"}, Exclude: []string{"docs/000-*.md"}}); err == nil || !strings.Contains(err.Error(), "cannot read docs/e.md") {
		t.Errorf("Build with a broken link as a document: error = %v", err)
	}
	if _, err := Build(root, &config.Config{Documents: config.Documents{Dir: "docs/sub/none"}}); err == nil {
		t.Error("Build succeeded on a missing directory")
	}
	if _, err := Build(root, &config.Config{Documents: config.Documents{Dir: "out"}}); err == nil {
		t.Error("Build succeeded without documents")
	}
}

func TestBuildPatterns(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	root := t.TempDir()
	for name, content := range map[string]string{
		".gitignore":               "local/\n",
		"AGENTS.md":                "# root\n",
		"CLAUDE.md":                "# root\n",
		"README.md":                "# not named\n",
		"a/AGENTS.md":              "# a\n",
		"a/b/c/AGENTS.md":          "# c\n",
		"a/b/c/AGENTS.txt":         "not Markdown\n",
		"docs/x.md":                "# x\n",
		"docs/deep/er/y.md":        "# y\n",
		"docs/drafts/z.md":         "# excluded\n",
		"docs/AGENTS.md":           "# matched twice\n",
		"local/AGENTS.md":          "# ignored\n",
		"nested/AGENTS.md":         "# in a nested repository\n",
		".git-not/AGENTS.md":       "# not .git\n",
		"docs.md/not-a-doc.txt":    "x\n",
		"other/docs/x.md":          "# docs not at the root\n",
		"other/AGENTS.md.d/x.conf": "x\n",
	} {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	// One in a nested repository is not one of this repository's
	if err := os.MkdirAll(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("git", "init", "-q", "nested")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init nested: %v\n%s", err, out)
	}
	// One tracked but deleted from the worktree is not one either
	cmd = exec.Command("git", "add", "a/AGENTS.md", "docs/gone.md")
	if err := os.WriteFile(filepath.Join(root, "docs", "gone.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	if err := os.Remove(filepath.Join(root, "docs", "gone.md")); err != nil {
		t.Fatal(err)
	}
	// Nor is one in .git
	if err := os.WriteFile(filepath.Join(root, ".git", "AGENTS.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	paths := func(cfg *config.Config) ([]string, error) {
		t.Helper()
		in, err := Build(root, cfg)
		if err != nil {
			return nil, err
		}
		if err := Validate(in); err != nil {
			t.Fatal(err)
		}
		var ps []string
		for _, d := range in.Documents {
			if d.Name != path.Base(d.Path) {
				t.Errorf("name %q of %q", d.Name, d.Path)
			}
			ps = append(ps, d.Path)
		}
		return ps, nil
	}

	got, err := paths(&config.Config{
		Documents: config.Documents{Patterns: []string{"**/AGENTS.md", "**/CLAUDE.md", "docs/**/*.md"}},
		Exclude:   []string{"**/drafts/**"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		".git-not/AGENTS.md",
		"AGENTS.md",
		"CLAUDE.md",
		"a/AGENTS.md",
		"a/b/c/AGENTS.md",
		"docs/AGENTS.md",
		"docs/deep/er/y.md",
		"docs/x.md",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("documents = %v, want %v", got, want)
	}

	// A pattern that names something other than Markdown matches nothing
	_, err = paths(&config.Config{Documents: config.Documents{Patterns: []string{"**/*.txt", "**/AGENTS.md.d"}}})
	if err == nil || !strings.Contains(err.Error(), "documents: no Markdown documents match **/*.txt, **/AGENTS.md.d") {
		t.Errorf("no match: error = %v", err)
	}
	// Ignored and excluded documents leave nothing
	_, err = paths(&config.Config{Documents: config.Documents{Patterns: []string{"local/*.md", "docs/drafts/*"}}, Exclude: []string{"docs/**"}})
	if err == nil || !strings.Contains(err.Error(), "no Markdown documents match") {
		t.Errorf("all left out: error = %v", err)
	}

	// An ignored directory that cannot be read is not looked into
	if err := os.MkdirAll(filepath.Join(root, "local", "locked"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(root, "local", "locked"), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(root, "local", "locked"), 0o755) })
	if _, err := paths(&config.Config{Documents: config.Documents{Patterns: []string{"**/AGENTS.md"}}}); err != nil {
		t.Errorf("with an unreadable ignored directory: %v", err)
	}

	// A broken symbolic link that matches cannot be read
	if err := os.Symlink("nowhere.md", filepath.Join(root, "a", "b", "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}
	_, err = paths(&config.Config{Documents: config.Documents{Patterns: []string{"**/CLAUDE.md"}}})
	if err == nil || !strings.Contains(err.Error(), "cannot read a/b/CLAUDE.md") {
		t.Errorf("broken link: error = %v", err)
	}
}
