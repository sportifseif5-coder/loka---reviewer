// Package rules implements the rules engine (architecture section 9).
// Rules are plain-language instructions with optional YAML front-matter that
// scope them to paths and, for deterministic rules, a regex pattern that
// matches added lines. Default rules are conservative and low-noise.
package rules

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/vcs"
)

// Rule is one compiled rule.
type Rule struct {
	// Name uniquely identifies the rule.
	Name string
	// Paths are glob patterns scoping the rule; empty means all paths.
	Paths []string
	// Severity ranks findings produced by this rule.
	Severity model.Severity
	// Pattern is an optional deterministic regex matched against added
	// lines. Empty means the rule needs model-assisted evaluation.
	Pattern string
	// Message is the plain-language instruction (the rule body).
	Message string
	// Source is where the rule came from (file path or "default").
	Source string

	pathRe []*regexp.Regexp
	regex  *regexp.Regexp
}

// ModelAssisted reports whether the rule needs semantic (LLM) judgment.
func (r *Rule) ModelAssisted() bool {
	return r.Pattern == ""
}

// MatchesPath reports whether path is in scope for the rule.
func (r *Rule) MatchesPath(path string) bool {
	if len(r.pathRe) == 0 {
		return true
	}
	for _, re := range r.pathRe {
		if re.MatchString(path) {
			return true
		}
	}
	return false
}

// frontMatter is the structured section of a rule document.
type frontMatter struct {
	Name     string   `yaml:"name"`
	Paths    []string `yaml:"paths"`
	Severity string   `yaml:"severity"`
	Pattern  string   `yaml:"pattern"`
}

// Parse compiles a single rule document (front-matter plus body).
func Parse(data []byte, source string) (*Rule, error) {
	fm, body, err := splitFrontMatter(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}

	name := fm.Name
	if name == "" {
		return nil, fmt.Errorf("%s: rule requires a name", source)
	}
	sev := model.Severity(fm.Severity)
	if fm.Severity == "" {
		sev = model.SeverityWarning
	}
	if !validSeverity(sev) {
		return nil, fmt.Errorf("%s: invalid severity %q", source, fm.Severity)
	}

	r := &Rule{
		Name:     name,
		Paths:    fm.Paths,
		Severity: sev,
		Pattern:  strings.TrimSpace(fm.Pattern),
		Message:  strings.TrimSpace(body),
		Source:   source,
	}
	if len(r.Paths) > 0 {
		for _, p := range r.Paths {
			re, err := globToRegexp(p)
			if err != nil {
				return nil, fmt.Errorf("%s: bad path %q: %w", source, p, err)
			}
			r.pathRe = append(r.pathRe, re)
		}
	}
	if r.Pattern != "" {
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			return nil, fmt.Errorf("%s: bad pattern: %w", source, err)
		}
		r.regex = re
	}
	return r, nil
}

// ParseFile loads a single rule from a Markdown file path.
func ParseFile(path string) (*Rule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data, path)
}

// ParseAll parses every rule document in a Markdown file. Documents are
// separated by a line of exactly "---" at the start of a rule block.
func ParseAll(data []byte, source string) ([]*Rule, error) {
	docs := splitDocuments(string(data))
	if len(docs) == 0 {
		return nil, nil
	}
	var out []*Rule
	for i, d := range docs {
		if strings.TrimSpace(d) == "" {
			continue
		}
		r, err := Parse([]byte(d), fmt.Sprintf("%s#%d", source, i+1))
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// Defaults returns the built-in low-noise rule set.
func Defaults() []*Rule {
	out := make([]*Rule, 0, len(defaultSpecs))
	for _, spec := range defaultSpecs {
		r, err := Parse([]byte(spec), "default")
		if err != nil {
			// Defaults are authored to compile; treat failure as a bug.
			panic(fmt.Sprintf("bad default rule: %v", err))
		}
		out = append(out, r)
	}
	return out
}

// AddedLine is one added line in the working-tree diff.
type AddedLine = vcs.AddedLine

// ChangedFile is a file with its added lines.
type ChangedFile = vcs.ChangedFile

// Evaluate applies deterministic rules to the changed files and returns
// findings. Files whose paths are out of scope are skipped. Model-assisted
// rules are not evaluated here (they are deferred to the provider layer).
func Evaluate(files []vcs.ChangedFile, rs []*Rule) []model.Finding {
	var findings []model.Finding
	for _, f := range files {
		for _, r := range rs {
			if r.ModelAssisted() || !r.MatchesPath(f.Path) {
				continue
			}
			for _, al := range f.Added {
				loc := r.regex.FindStringIndex(al.Text)
				if loc == nil {
					continue
				}
				findings = append(findings, model.Finding{
					RuleID:     r.Name,
					Category:   "rule",
					Severity:   r.Severity,
					Source:     model.SourceRule,
					Location:   model.Location{File: f.Path, LineStart: al.Number, LineEnd: al.Number},
					Message:    r.Message,
					Reasoning:  fmt.Sprintf("rule %q matched pattern %q", r.Name, r.Pattern),
					Confidence: 1.0,
					Evidence:   []string{al.Text},
				})
			}
		}
	}
	return findings
}

// ParseChangedFiles extracts added lines from per-file unified diffs.
func ParseChangedFiles(diffs map[string]string) []vcs.ChangedFile {
	return vcs.ParseChangedFiles(diffs)
}

// globToRegexp converts a simple glob (*, **, ?) to an anchored regexp.
// A single * matches within one path segment; ** crosses segment boundaries,
// and "**/" also matches an empty prefix (a file at the repo root).
func globToRegexp(glob string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); i++ {
		switch glob[i] {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				i++
				if i+1 < len(glob) && glob[i+1] == '/' {
					b.WriteString("(?:.*/)?")
					i++
				} else {
					b.WriteString(".*")
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(glob[i])))
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

func splitFrontMatter(data []byte) (frontMatter, string, error) {
	s := string(data)
	if !strings.HasPrefix(s, "---") {
		return frontMatter{}, "", errors.New("missing front-matter: rule must start with ---")
	}
	lines := strings.SplitN(s, "\n", 2)
	if len(lines) != 2 {
		return frontMatter{}, "", errors.New("malformed rule document")
	}
	rest := lines[1]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		if idx := strings.Index(rest, "---"); idx >= 0 {
			end = idx
		} else {
			return frontMatter{}, "", errors.New("unterminated front-matter")
		}
	}
	fmBlock := rest[:end]
	body := strings.TrimSpace(rest[end+len("\n---"):])
	var fm frontMatter
	if err := yaml.Unmarshal([]byte(fmBlock), &fm); err != nil {
		return frontMatter{}, "", fmt.Errorf("front-matter: %w", err)
	}
	return fm, body, nil
}

// splitDocuments splits a rules file into rule documents. Every rule has
// exactly two "---" lines (a front-matter opener and closer); the closer of
// one rule is also the opener of the next, so documents begin at every
// even-indexed separator line.
func splitDocuments(s string) []string {
	lines := strings.Split(s, "\n")
	var separators []int
	for i, ln := range lines {
		if strings.TrimSpace(ln) == "---" {
			separators = append(separators, i)
		}
	}
	if len(separators) == 0 {
		return nil
	}
	var docs []string
	for k, start := range separators {
		if k%2 != 0 {
			continue
		}
		end := len(lines)
		if k+2 < len(separators) {
			end = separators[k+2]
		}
		docs = append(docs, strings.Join(lines[start:end], "\n"))
	}
	return docs
}

func validSeverity(s model.Severity) bool {
	switch s {
	case model.SeverityInfo, model.SeverityWarning, model.SeverityError, model.SeverityCritical:
		return true
	}
	return false
}
