package input

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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
		".gitignore":       "docs/local.md\nout/\n",
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
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	in, err := Build(root, &config.Config{Documents: "docs", Exclude: []string{"docs/000-*.md"}})
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
	if want := []string{"docs/a.md", "docs/b.md"}; !reflect.DeepEqual(paths, want) {
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

	if _, err := Build(root, &config.Config{Documents: "docs/sub/none"}); err == nil {
		t.Error("Build succeeded on a missing directory")
	}
	if _, err := Build(root, &config.Config{Documents: "out"}); err == nil {
		t.Error("Build succeeded without documents")
	}
}
