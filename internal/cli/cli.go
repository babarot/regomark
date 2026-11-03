// Package cli is the regomark command line.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/babarot/regomark/internal/config"
	"github.com/babarot/regomark/internal/gitrepo"
	"github.com/babarot/regomark/internal/input"
	"github.com/babarot/regomark/internal/policy"
	"github.com/babarot/regomark/internal/report"
	"github.com/babarot/regomark/internal/version"
)

// Exit codes.
const (
	ExitOK    = 0 // no deny
	ExitDeny  = 1 // at least one deny
	ExitError = 2 // the check could not be done
)

const usage = `Usage:
  regomark check [--config .regomark.yaml] [--format text|github|json]
  regomark dump [--config .regomark.yaml]
  regomark version (or --version)

check evaluates the policies against the documents and reports the
violations; it exits 1 if any is a deny, and 2 if the check cannot be done.
dump prints the input the policies receive, as JSON.

Paths, --config included, are relative to the root of the git repository,
so regomark gives the same result from any directory in it.
`

// Run runs regomark with the arguments (without the program name) in the
// directory dir, and returns the exit code.
func Run(ctx context.Context, args []string, dir string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return ExitError
	}
	var err error
	switch args[0] {
	case "check":
		var denied bool
		denied, err = check(ctx, args[1:], dir, stdout, stderr)
		if err == nil && denied {
			return ExitDeny
		}
	case "dump":
		err = dump(args[1:], dir, stdout, stderr)
	case "version", "--version", "-v":
		if len(args) > 1 {
			err = errUsage
			break
		}
		_, err = fmt.Fprintln(stdout, "regomark", version.String())
	case "-h", "-help", "--help", "help":
		fmt.Fprint(stdout, usage)
		return ExitOK
	default:
		err = fmt.Errorf("unknown command %q", args[0])
		fmt.Fprint(stderr, usage)
	}
	switch {
	case err == nil:
		return ExitOK
	case errors.Is(err, flag.ErrHelp):
		return ExitOK
	case errors.Is(err, errUsage):
		fmt.Fprint(stderr, usage)
		return ExitError
	default:
		fmt.Fprintf(stderr, "regomark: %v\n", err)
		return ExitError
	}
}

var errUsage = errors.New("usage")

func newFlagSet(name string, stderr io.Writer) (*flag.FlagSet, *string) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfg := fs.String("config", config.DefaultPath, "configuration file, relative to the repository root")
	return fs, cfg
}

func parse(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		return errUsage // the flag package has said what was wrong
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("%s: unexpected argument %q", fs.Name(), fs.Arg(0))
	}
	return nil
}

// load finds the repository root from dir, reads the configuration and builds
// the input, validated.
func load(dir, cfgPath string) (string, *config.Config, *input.Input, error) {
	root, err := gitrepo.Root(dir)
	if err != nil {
		return "", nil, nil, err
	}
	cfg, err := loadConfig(root, cfgPath)
	if err != nil {
		return "", nil, nil, err
	}
	in, err := input.Build(root, cfg)
	if err != nil {
		return "", nil, nil, err
	}
	if err := input.Validate(in); err != nil {
		return "", nil, nil, err
	}
	return root, cfg, in, nil
}

// loadConfig reads the configuration file at name, relative to root unless
// absolute.
func loadConfig(root, name string) (*config.Config, error) {
	file := name
	if !filepath.IsAbs(file) {
		file = filepath.Join(root, filepath.FromSlash(name))
	}
	b, err := os.ReadFile(file)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("config: %s does not exist", name)
		}
		return nil, fmt.Errorf("config: %w", err)
	}
	cfg, err := config.Parse(b)
	if err != nil {
		return nil, fmt.Errorf("config: %s: %w", name, err)
	}
	return cfg, nil
}

func check(ctx context.Context, args []string, dir string, stdout, stderr io.Writer) (bool, error) {
	flags, cfgPath := newFlagSet("check", stderr)
	format := flags.String("format", "text", "output format: "+strings.Join(report.Formats, ", "))
	if err := parse(flags, args); err != nil {
		return false, err
	}
	if !slices.Contains(report.Formats, *format) {
		return false, fmt.Errorf("--format: unknown format %q (want one of %s)", *format, strings.Join(report.Formats, ", "))
	}
	root, cfg, in, err := load(dir, *cfgPath)
	if err != nil {
		return false, err
	}
	set, err := policy.Load(root, cfg.Policy, cfg.Entrypoint)
	if err != nil {
		return false, err
	}
	qs, err := set.Questions(ctx, in)
	if err != nil {
		return false, err
	}
	res, err := set.Eval(ctx, in, policy.NotRun(qs), stderr)
	if err != nil {
		return false, err
	}
	if err := res.Skip(cfg.SkipRules); err != nil {
		return false, err
	}
	if err := report.Write(stdout, *format, res.Violations); err != nil {
		return false, fmt.Errorf("output: %w", err)
	}
	return res.Denied(), nil
}

func dump(args []string, dir string, stdout, stderr io.Writer) error {
	flags, cfgPath := newFlagSet("dump", stderr)
	if err := parse(flags, args); err != nil {
		return err
	}
	_, _, in, err := load(dir, *cfgPath)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(in); err != nil {
		return fmt.Errorf("output: %w", err)
	}
	return nil
}
