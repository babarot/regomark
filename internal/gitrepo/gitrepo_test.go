package gitrepo

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func isolateGit(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRoot(t *testing.T) {
	isolateGit(t)
	root := t.TempDir()
	run(t, root, "init", "-q")
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := Root(sub)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("Root = %s, want %s", got, want)
	}
}

func TestRootOutsideRepository(t *testing.T) {
	isolateGit(t)
	if _, err := Root(t.TempDir()); err == nil {
		t.Error("Root succeeded outside a repository")
	}
}

func TestIgnored(t *testing.T) {
	isolateGit(t)
	root := t.TempDir()
	run(t, root, "init", "-q")
	writeFile(t, filepath.Join(root, ".gitignore"), "build/\n*.log\n日本語.md\n")
	writeFile(t, filepath.Join(root, "build", "out.txt"), "x")
	writeFile(t, filepath.Join(root, "kept.log"), "x")
	writeFile(t, filepath.Join(root, "a b.md"), "x")
	run(t, root, "add", ".gitignore", "a b.md")
	run(t, root, "add", "-f", "kept.log")

	got, err := Ignored(root, []string{"build", "build/out.txt", "build/none", "kept.log", "other.log", "a b.md", "日本語.md", "docs/x.md"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"build": true, "build/out.txt": true, "build/none": true, "other.log": true, "日本語.md": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Ignored = %v, want %v (a tracked file is never ignored)", got, want)
	}

	none, err := Ignored(root, []string{"a b.md"})
	if err != nil || len(none) != 0 {
		t.Errorf("Ignored = %v, %v; want none", none, err)
	}
}
