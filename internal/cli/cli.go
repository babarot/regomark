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
	"github.com/babarot/regomark/internal/judge"
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
  regomark check [--config .regomark.yaml] [--format text|pretty|github|json] [--judge | --no-judge | --judgments FILE]
  regomark dump [--config .regomark.yaml] [--judge | --judgments FILE]
  regomark questions [--config .regomark.yaml]
  regomark version (or --version)

check evaluates the policies against the documents and reports the
violations; it exits 1 if any is a deny, and 2 if the check cannot be done.
With --judge, or REGOMARK_JUDGE=on in the environment, it asks the policies'
questions with judge.command; --no-judge does not, whatever the environment.
With --judgments, it takes the answers from FILE instead.
dump prints the input the policies receive, as JSON; with --judge or
--judgments, the judgments are in it.
questions prints the policies' questions with their prompts, as JSON, for a
judge that writes the answers to a file for --judgments.

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
		err = dump(ctx, args[1:], dir, stdout, stderr)
	case "questions":
		err = questions(ctx, args[1:], dir, stdout, stderr)
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
	format := flags.String("format", "", "output format: "+strings.Join(report.Formats, ", ")+" (default pretty at a terminal, text otherwise)")
	judgeFlag := flags.Bool("judge", false, "ask the policies' questions with judge.command")
	noJudge := flags.Bool("no-judge", false, "do not ask them, even with REGOMARK_JUDGE=on")
	file := flags.String("judgments", "", "take the answers from `FILE`, written for regomark questions")
	if err := parse(flags, args); err != nil {
		return false, err
	}
	width, tty := terminal(stdout)
	if *format == "" {
		*format = "text"
		if tty {
			*format = "pretty"
		}
	}
	if !slices.Contains(report.Formats, *format) {
		return false, fmt.Errorf("--format: unknown format %q (want one of %s)", *format, strings.Join(report.Formats, ", "))
	}
	how, err := judging(*judgeFlag, *noJudge, *file, os.Getenv("REGOMARK_JUDGE"))
	if err != nil {
		return false, err
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
	judgments, err := ask(ctx, how, root, *cfgPath, cfg, in, qs, stderr)
	if err != nil {
		return false, err
	}
	res, err := set.Eval(ctx, in, judgments, stderr)
	if err != nil {
		return false, err
	}
	if err := res.Skip(cfg.SkipRules); err != nil {
		return false, err
	}
	if err := report.Write(colored(stdout), *format, res.Violations, width); err != nil {
		return false, fmt.Errorf("output: %w", err)
	}
	return res.Denied(), nil
}

// How the questions are answered, if at all.
const (
	noJudging   = iota
	judgeByFlag // --judge
	judgeByEnv  // REGOMARK_JUDGE=on
	judgeByFile // --judgments FILE
)

type judgingMode struct {
	how  int
	file string // with judgeByFile
}

// judging decides how to answer the questions. Only the user turns judging
// on, with a flag or REGOMARK_JUDGE=on; nothing in the repository can, so
// that checking a repository never runs a command it names unasked.
func judging(judgeFlag, noJudge bool, file, env string) (judgingMode, error) {
	given := 0
	for _, b := range []bool{judgeFlag, noJudge, file != ""} {
		if b {
			given++
		}
	}
	switch {
	case given > 1:
		return judgingMode{}, errors.New("give only one of --judge, --no-judge and --judgments")
	case file != "":
		return judgingMode{how: judgeByFile, file: file}, nil
	case judgeFlag:
		return judgingMode{how: judgeByFlag}, nil
	case noJudge:
		return judgingMode{how: noJudging}, nil
	}
	switch env {
	case "on":
		return judgingMode{how: judgeByEnv}, nil
	case "off", "":
		return judgingMode{how: noJudging}, nil
	default:
		return judgingMode{}, fmt.Errorf("REGOMARK_JUDGE is %q; set it to on or off, or unset it", env)
	}
}

// ask answers the questions as mode says, and returns their judgments: not
// run unless they are asked.
func ask(ctx context.Context, mode judgingMode, root, cfgPath string, cfg *config.Config, in *input.Input, qs []policy.Question, stderr io.Writer) ([]input.Judgment, error) {
	notRun := policy.NotRun(qs)
	how := mode.how
	if how == noJudging {
		return notRun, nil
	}
	if how == judgeByFile {
		return fromFile(root, mode.file, in, qs, stderr)
	}
	if cfg.Judge.Command == nil {
		if how == judgeByFlag {
			return nil, fmt.Errorf("--judge: %s sets no judge.command", cfgPath)
		}
		if len(qs) > 0 {
			// REGOMARK_JUDGE=on may be set for every repository, so a
			// repository without a judge does not stop the check
			fmt.Fprintf(stderr, "regomark: REGOMARK_JUDGE is on, but %s sets no judge.command; the questions are not asked\n", cfgPath)
		}
		return notRun, nil
	}
	if len(qs) == 0 {
		return notRun, nil
	}
	if len(qs) > cfg.Judge.MaxQuestions {
		return nil, fmt.Errorf("judge: the policies ask %d questions, more than judge.max_questions (%d)", len(qs), cfg.Judge.MaxQuestions)
	}
	cache, err := cacheDir(root, cfg.Judge.Cache)
	if err != nil {
		return nil, err
	}
	j := &judge.Judge{
		Command:  cfg.Judge.Command,
		Dir:      root,
		Parallel: cfg.Judge.Parallel,
		Timeout:  cfg.Judge.TimeoutDuration,
		CacheDir: cache,
	}
	note(stderr, "regomark: judge: %s", quote(cfg.Judge.Command))
	js, stats, err := j.Run(ctx, qs, in.Sources)
	if err != nil {
		return nil, fmt.Errorf("judge: %w", err)
	}
	note(stderr, "regomark: judge: %d questions, %d cached, %d asked, %d failed", stats.Questions, stats.Cached, stats.Asked, stats.Failed)
	if stats.CacheErr != nil {
		fmt.Fprintf(stderr, "regomark: judge: answers not cached: %v\n", stats.CacheErr)
	}
	return js, nil
}

// fromFile answers the questions from a file of answers, at a path relative
// to root unless absolute.
func fromFile(root, file string, in *input.Input, qs []policy.Question, stderr io.Writer) ([]input.Judgment, error) {
	p := file
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, filepath.FromSlash(file))
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("--judgments: %s: %w", file, unwrapPath(err))
	}
	js, stats, err := judge.FromFile(qs, in.Sources, data, file)
	if err != nil {
		return nil, fmt.Errorf("--judgments: %w", err)
	}
	note(stderr, "regomark: judge: %d questions, %d answered from %s, %d failed", stats.Questions, stats.Answered, file, stats.Failed)
	if stats.Unmatched > 0 {
		fmt.Fprintf(stderr, "regomark: judge: %d answers in %s match no question; were the documents or the policies changed after regomark questions?\n", stats.Unmatched, file)
	}
	return js, nil
}

// unwrapPath drops the path from a file system error, which the message
// names already as the user gave it.
func unwrapPath(err error) error {
	if pe, ok := errors.AsType[*fs.PathError](err); ok {
		return pe.Err
	}
	return err
}

// cacheDir returns the directory of cached answers: dir as configured,
// relative to root unless absolute, or regomark/judge in the user cache
// directory. The answers are keyed by what was asked, so repositories can
// share them.
func cacheDir(root, dir string) (string, error) {
	if dir != "" {
		if filepath.IsAbs(dir) {
			return dir, nil
		}
		return filepath.Join(root, filepath.FromSlash(dir)), nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("judge: no cache directory (set judge.cache): %w", err)
	}
	return filepath.Join(base, "regomark", "judge"), nil
}

// quote shows a command as a shell would take it.
func quote(args []string) string {
	var b strings.Builder
	for i, a := range args {
		if i > 0 {
			b.WriteByte(' ')
		}
		if a != "" && strings.Trim(a, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_./=:,+@%") == "" {
			b.WriteString(a)
			continue
		}
		b.WriteString("'" + strings.ReplaceAll(a, "'", `'\''`) + "'")
	}
	return b.String()
}

func dump(ctx context.Context, args []string, dir string, stdout, stderr io.Writer) error {
	flags, cfgPath := newFlagSet("dump", stderr)
	judgeFlag := flags.Bool("judge", false, "ask the policies' questions with judge.command, and include the judgments")
	file := flags.String("judgments", "", "take the answers from `FILE`, and include the judgments")
	if err := parse(flags, args); err != nil {
		return err
	}
	// Not REGOMARK_JUDGE: what dump prints does not depend on the environment
	mode, err := judging(*judgeFlag, false, *file, "")
	if err != nil {
		return err
	}
	root, cfg, in, err := load(dir, *cfgPath)
	if err != nil {
		return err
	}
	if mode.how != noJudging {
		set, err := policy.Load(root, cfg.Policy, cfg.Entrypoint)
		if err != nil {
			return err
		}
		qs, err := set.Questions(ctx, in)
		if err != nil {
			return err
		}
		js, err := ask(ctx, mode, root, *cfgPath, cfg, in, qs, stderr)
		if err != nil {
			return err
		}
		in.Judgments = js
		if err := input.Validate(in); err != nil {
			return err
		}
	}
	return writeJSON(stdout, in)
}

func questions(ctx context.Context, args []string, dir string, stdout, stderr io.Writer) error {
	flags, cfgPath := newFlagSet("questions", stderr)
	if err := parse(flags, args); err != nil {
		return err
	}
	root, cfg, in, err := load(dir, *cfgPath)
	if err != nil {
		return err
	}
	set, err := policy.Load(root, cfg.Policy, cfg.Entrypoint)
	if err != nil {
		return err
	}
	qs, err := set.Questions(ctx, in)
	if err != nil {
		return err
	}
	out, err := judge.Export(qs, in.Sources)
	if err != nil {
		return err
	}
	return writeJSON(stdout, out)
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("output: %w", err)
	}
	return nil
}
