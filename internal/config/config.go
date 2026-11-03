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
)

// DefaultPath is where regomark looks for the configuration, relative to the
// repository root.
const DefaultPath = ".regomark.yaml"

// Config is the content of .regomark.yaml. Every path in it is relative to
// the repository root and uses forward slashes.
type Config struct {
	// Documents is the directory of the Markdown documents to check.
	Documents string `yaml:"documents"`
	// Policy is the directory of the rego policies.
	Policy string `yaml:"policy"`
	// Entrypoint is the package that defines deny and warn (data.<entrypoint>).
	Entrypoint string `yaml:"entrypoint"`
	// SkipRules are the IDs of the rules whose violations are dropped.
	SkipRules []string `yaml:"skip_rules"`
	// Exclude are the documents not to check, as path.Match patterns
	// against their root-relative paths.
	Exclude []string `yaml:"exclude"`
	// Judge is how to ask the policies' questions. It never turns judging
	// on: only the user does, with a flag or REGOMARK_JUDGE.
	Judge Judge `yaml:"judge"`
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
	for _, f := range []struct {
		key   string
		value *string
	}{{"documents", &c.Documents}, {"policy", &c.Policy}} {
		p, err := rootRelative(*f.value)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", f.key, err))
			continue
		}
		*f.value = p
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
		if _, err := path.Match(p, ""); err != nil || p == "" {
			errs = append(errs, fmt.Errorf("exclude: %q is not a valid pattern", p))
		}
	}
	errs = append(errs, c.Judge.validate()...)
	return errors.Join(errs...)
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
		if ok, _ := path.Match(pat, p); ok {
			return true
		}
	}
	return false
}
