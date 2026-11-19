// Package config reads .regomark.yaml.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/babarot/regomark/internal/glob"
)

// DefaultPath is where regomark looks for the configuration, relative to the
// repository root.
const DefaultPath = ".regomark.yaml"

// Config is the content of .regomark.yaml. Every path in it is relative to
// the repository root and uses forward slashes.
type Config struct {
	// Documents is which Markdown documents to check.
	Documents Documents `yaml:"documents"`
	// Policy is the directory of the rego policies.
	Policy string `yaml:"policy"`
	// Entrypoint is the package that defines deny and warn (data.<entrypoint>).
	Entrypoint string `yaml:"entrypoint"`
	// SkipRules are the IDs of the rules whose violations are dropped.
	SkipRules []string `yaml:"skip_rules"`
	// Exclude are the documents not to check, as patterns (see package
	// glob) against their root-relative paths.
	Exclude []string `yaml:"exclude"`
	// Judge is how to ask the policies' questions. It never turns judging
	// on: only the user does, with a flag or REGOMARK_JUDGE.
	Judge Judge `yaml:"judge"`
}

// Documents is the documents key: a directory, whose *.md files right in it
// are the documents, or patterns, which the root-relative paths of the
// documents match. Exactly one of Dir and Patterns is set in a valid
// configuration.
type Documents struct {
	// Dir is the directory, when documents is a string without glob
	// metacharacters.
	Dir string
	// Patterns are the patterns, when documents is a list or a string with
	// glob metacharacters.
	Patterns []string
}

// UnmarshalYAML takes either a string or a list of strings.
func (d *Documents) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		var s string
		if err := n.Decode(&s); err != nil {
			return err
		}
		if glob.HasMeta(s) {
			*d = Documents{Patterns: []string{s}}
		} else {
			*d = Documents{Dir: s}
		}
		return nil
	case yaml.SequenceNode:
		var ps []string
		if err := n.Decode(&ps); err != nil {
			return err
		}
		*d = Documents{Patterns: ps}
		return nil
	}
	return fmt.Errorf("line %d: documents: must be a directory or a list of patterns", n.Line)
}

// String is how messages name the documents: the directory, or the
// patterns separated by commas.
func (d Documents) String() string {
	if d.Patterns == nil {
		return d.Dir
	}
	return strings.Join(d.Patterns, ", ")
}

// Judge is the judge section of the configuration.
type Judge struct {
	// Command is the command to run for each question, without a shell:
	// the prompt on its standard input, the answer on its standard output.
	Command []string `yaml:"command"`
	// Parallel is how many commands run at once (default 4).
	Parallel int `yaml:"parallel"`
	// Timeout is the limit for one command, as a Go duration (default 120s).
	Timeout string `yaml:"timeout"`
	// Cache is the directory of the cached answers, absolute or relative to
	// the repository root (default: regomark/judge in the user cache
	// directory).
	Cache string `yaml:"cache"`
	// MaxQuestions is how many questions may be asked in one run (default
	// 200), a limit on what a run can cost.
	MaxQuestions int `yaml:"max_questions"`

	// TimeoutDuration is Timeout parsed.
	TimeoutDuration time.Duration `yaml:"-"`
}

// Judge defaults.
const (
	DefaultParallel     = 4
	DefaultTimeout      = 120 * time.Second
	DefaultMaxQuestions = 200
)

// Parse decodes the configuration strictly, rejecting unknown keys, and
// validates it.
func Parse(b []byte) (*Config, error) {
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		if te, ok := errors.AsType[*yaml.TypeError](err); ok {
			// Name the keys as the file does, not as the Go type does
			return nil, errors.New(notFound.ReplaceAllString(strings.Join(te.Errors, "\n"), " is not a known key"))
		}
		return nil, err
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// notFound is how the decoder reports an unknown key, naming the Go type.
var notFound = regexp.MustCompile(` not found in type config\.\w+`)

// identifier is one segment of a package path, as rego accepts it unquoted.
var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (c *Config) validate() error {
	var errs []error
	errs = append(errs, c.Documents.validate()...)
	if p, err := rootRelative(c.Policy); err != nil {
		errs = append(errs, fmt.Errorf("policy: %w", err))
	} else {
		c.Policy = p
	}
	if c.Entrypoint == "" {
		errs = append(errs, errors.New("entrypoint: must be set"))
	} else {
		for s := range strings.SplitSeq(c.Entrypoint, ".") {
			if !identifier.MatchString(s) {
				errs = append(errs, fmt.Errorf("entrypoint: %q is not a package name", c.Entrypoint))
				break
			}
		}
	}
	for _, id := range c.SkipRules {
		if id == "" {
			errs = append(errs, errors.New("skip_rules: an entry is empty"))
		}
	}
	for _, p := range c.Exclude {
		if p == "" || glob.Validate(p) != nil {
			errs = append(errs, fmt.Errorf("exclude: %q is not a valid pattern", p))
		}
	}
	errs = append(errs, c.Judge.validate()...)
	return errors.Join(errs...)
}

// validate checks the directory or every pattern, and cleans them.
func (d *Documents) validate() []error {
	if d.Patterns == nil {
		p, err := rootRelative(d.Dir)
		if err != nil {
			return []error{fmt.Errorf("documents: %w", err)}
		}
		d.Dir = p
		return nil
	}
	if len(d.Patterns) == 0 {
		return []error{errors.New("documents: must be set")}
	}
	var errs []error
	for i, pat := range d.Patterns {
		if pat == "" {
			errs = append(errs, errors.New("documents: an entry is empty"))
			continue
		}
		p, err := rootRelative(pat)
		if err != nil {
			errs = append(errs, fmt.Errorf("documents: %w", err))
			continue
		}
		if glob.Validate(p) != nil {
			errs = append(errs, fmt.Errorf("documents: %q is not a valid pattern", pat))
			continue
		}
		d.Patterns[i] = p
	}
	return errs
}

// validate checks the judge section and fills in the defaults.
func (j *Judge) validate() []error {
	var errs []error
	if j.Command != nil && (len(j.Command) == 0 || j.Command[0] == "") {
		errs = append(errs, errors.New("judge.command: must name a command"))
	}
	switch {
	case j.Parallel == 0:
		j.Parallel = DefaultParallel
	case j.Parallel < 0:
		errs = append(errs, errors.New("judge.parallel: must be at least 1"))
	}
	j.TimeoutDuration = DefaultTimeout
	if j.Timeout != "" {
		d, err := time.ParseDuration(j.Timeout)
		if err != nil || d <= 0 {
			errs = append(errs, fmt.Errorf("judge.timeout: %q is not a positive duration such as 120s", j.Timeout))
		}
		j.TimeoutDuration = d
	}
	switch {
	case j.MaxQuestions == 0:
		j.MaxQuestions = DefaultMaxQuestions
	case j.MaxQuestions < 0:
		errs = append(errs, errors.New("judge.max_questions: must be at least 1"))
	}
	if strings.Contains(j.Cache, `\`) {
		errs = append(errs, fmt.Errorf("judge.cache: %q: use forward slashes", j.Cache))
	}
	return errs
}

// rootRelative checks that p is a relative path inside the repository and
// cleans it.
func rootRelative(p string) (string, error) {
	if p == "" {
		return "", errors.New("must be set")
	}
	if strings.Contains(p, `\`) {
		return "", fmt.Errorf("%q: use forward slashes", p)
	}
	if path.IsAbs(p) {
		return "", fmt.Errorf("%q: must be relative to the repository root", p)
	}
	p = path.Clean(p)
	if p == ".." || strings.HasPrefix(p, "../") {
		return "", fmt.Errorf("%q: must be inside the repository", p)
	}
	return p, nil
}

// Excluded reports whether the document at the root-relative path p matches
// one of the exclude patterns.
func (c *Config) Excluded(p string) bool {
	for _, pat := range c.Exclude {
		if glob.Match(pat, p) {
			return true
		}
	}
	return false
}
