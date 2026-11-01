package cli

import (
	"bytes"
	"context"
	"flag"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the expected output of the fixtures")

// isolateGit keeps the user's and the system's git configuration, global
// excludes included, out of the tests.
func isolateGit(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

// TestFixtures runs regomark on each directory in testdata/fixtures:
//
//	repo/    the repository, laid over testdata/base; a file named gitignore
//	         becomes .gitignore, which could not be kept here as it is, and
//	         a symbolic link is copied as a link
//	args     the arguments, separated by spaces (default: check)
//	cwd      the directory to run in, relative to the repository (default: root)
//	stdout   the expected standard output
//	stderr   the expected standard error
//	exit     the expected exit code
//
// go test -update rewrites stdout, stderr and exit.
func TestFixtures(t *testing.T) {
	isolateGit(t)
	dirs, err := filepath.Glob(filepath.Join("testdata", "fixtures", "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) == 0 {
		t.Fatal("no fixtures")
	}
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			root := t.TempDir()
			copyTree(t, filepath.Join("testdata", "base"), root)
			copyTree(t, filepath.Join(dir, "repo"), root)
			git(t, root, "init", "-q")
			git(t, root, "add", "-A")

			args := []string{"check"}
			if b, err := os.ReadFile(filepath.Join(dir, "args")); err == nil {
				args = strings.Fields(string(b))
			}
			cwd := root
			if b, err := os.ReadFile(filepath.Join(dir, "cwd")); err == nil {
				cwd = filepath.Join(root, filepath.FromSlash(strings.TrimSpace(string(b))))
			}
			var stdout, stderr bytes.Buffer
			code := Run(context.Background(), args, cwd, &stdout, &stderr)

			if *update {
				write(t, filepath.Join(dir, "stdout"), stdout.String())
				write(t, filepath.Join(dir, "stderr"), stderr.String())
				write(t, filepath.Join(dir, "exit"), strconv.Itoa(code)+"\n")
				return
			}
			if want := read(t, filepath.Join(dir, "exit")); strings.TrimSpace(want) != strconv.Itoa(code) {
				t.Errorf("exit code = %d, want %s", code, strings.TrimSpace(want))
			}
			if want := read(t, filepath.Join(dir, "stdout")); stdout.String() != want {
				t.Errorf("stdout:\n%s\nwant:\n%s", stdout.String(), want)
			}
			if want := read(t, filepath.Join(dir, "stderr")); stderr.String() != want {
				t.Errorf("stderr:\n%s\nwant:\n%s", stderr.String(), want)
			}
		})
	}
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if d.Name() == "gitignore" {
			rel = filepath.Join(filepath.Dir(rel), ".gitignore")
		}
		target := filepath.Join(dst, rel)
		if d.Type()&fs.ModeSymlink != 0 {
			// Kept as a link, broken or not
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func write(t *testing.T, name, s string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunCommands(t *testing.T) {
	isolateGit(t)
	notRepo := t.TempDir()
	tests := []struct {
		args   []string
		code   int
		stdout string // a substring
		stderr string // a substring
	}{
		{[]string{"version"}, ExitOK, "regomark 0.", ""},
		{[]string{"--version"}, ExitOK, "regomark 0.", ""},
		{[]string{"-v"}, ExitOK, "regomark 0.", ""},
		{[]string{"version", "x"}, ExitError, "", "Usage:"},
		{nil, ExitError, "", "Usage:"},
		{[]string{"--help"}, ExitOK, "Usage:", ""},
		{[]string{"check", "-h"}, ExitOK, "", "-format"},
		{[]string{"lint"}, ExitError, "", `unknown command "lint"`},
		{[]string{"check", "--bogus"}, ExitError, "", "flag provided but not defined"},
		{[]string{"dump", "extra"}, ExitError, "", `unexpected argument "extra"`},
		{[]string{"check"}, ExitError, "", "git rev-parse --show-toplevel"},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(context.Background(), tt.args, notRepo, &stdout, &stderr)
			if code != tt.code {
				t.Errorf("exit code = %d, want %d", code, tt.code)
			}
			if !strings.Contains(stdout.String(), tt.stdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.stdout)
			}
			if !strings.Contains(stderr.String(), tt.stderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.stderr)
			}
		})
	}
}
