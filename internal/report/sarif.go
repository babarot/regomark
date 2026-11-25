package report

import (
	"encoding/json"
	"io"
	"net/url"
	"slices"
	"strings"

	"github.com/babarot/regomark/internal/policy"
	"github.com/babarot/regomark/internal/version"
)

// The types below are the part of SARIF 2.1.0 regomark writes, in the order
// of its properties in the specification.
// https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID string `json:"id"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	RuleIndex int             `json:"ruleIndex"`
	Level     string          `json:"level"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           sarifRegion           `json:"region"`
}

type sarifArtifactLocation struct {
	URI       string `json:"uri"`
	URIBaseID string `json:"uriBaseId"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
}

// srcRoot names the root of the repository, which the paths are relative to.
// It is left undefined in the log, so that the log does not depend on where
// the repository was checked out; the consumer knows the root, as
// github/codeql-action/upload-sarif does.
const srcRoot = "%SRCROOT%"

// sarif writes the violations as a SARIF 2.1.0 log of one run, whose rules
// are the rule IDs the policy set declares, whether violated or not.
func sarif(w io.Writer, ruleIDs []string, vs []policy.Violation) error {
	rules := make([]sarifRule, len(ruleIDs))
	for i, id := range ruleIDs {
		rules[i] = sarifRule{ID: id}
	}
	results := make([]sarifResult, len(vs))
	for i, v := range vs {
		level := "error"
		if v.Severity == policy.Warn {
			level = "warning"
		}
		results[i] = sarifResult{
			RuleID:    v.Rule,
			RuleIndex: slices.Index(ruleIDs, v.Rule),
			Level:     level,
			Message:   sarifMessage{Text: escapeBraces(v.Message)},
			Locations: []sarifLocation{{
				PhysicalLocation: sarifPhysicalLocation{
					ArtifactLocation: sarifArtifactLocation{URI: pathURI(v.Path), URIBaseID: srcRoot},
					Region:           sarifRegion{StartLine: v.Line},
				},
			}},
		}
	}
	log := sarifLog{
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Version: "2.1.0",
		Runs: []sarifRun{{
			Tool: sarifTool{Driver: sarifDriver{
				Name:           "regomark",
				Version:        version.String(),
				InformationURI: "https://github.com/babarot/regomark",
				Rules:          rules,
			}},
			Results: results,
		}},
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(log)
}

// escapeBraces doubles the curly braces of a message, which SARIF reads as
// placeholders for arguments otherwise (§3.11.5).
func escapeBraces(s string) string {
	return strings.NewReplacer("{", "{{", "}", "}}").Replace(s)
}

// pathURI turns a slash-separated path, relative to the repository root,
// into a relative URI reference: each byte that may not appear in a path of
// a URI is percent-encoded, and a first segment with a colon is preceded by
// ./ so that it is not taken for a scheme.
func pathURI(p string) string {
	return (&url.URL{Path: p}).String()
}
