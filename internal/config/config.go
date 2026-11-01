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
}

// Parse decodes the configuration strictly, rejecting unknown keys, and
// validates it.
func Parse(b []byte) (*Config, error) {
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		if te, ok := errors.AsType[*yaml.TypeError](err); ok {
			// Name the keys as the file does, not as the Go type does
			return nil, errors.New(strings.ReplaceAll(strings.Join(te.Errors, "\n"), " not found in type config.Config", " is not a known key"))
		}
		return nil, err
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

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
	return errors.Join(errs...)
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
