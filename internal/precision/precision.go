// Package precision is the Phase 3 noise-budget release gate (constitution
// section 11). It runs the deterministic analyzers against a fixed corpus
// and fails when unexpected findings appear.
package precision

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sportifseif5-coder/loka---reviewer/internal/advisory"
	"github.com/sportifseif5-coder/loka---reviewer/internal/analyzer"
	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/vcs"
)

// MinPrecision is the v1 release gate: zero false positives on the corpus.
const MinPrecision = 1.0

// Expect is the per-case allow-list of rule IDs.
type Expect struct {
	Allow []string `json:"allow"`
}

// CaseResult is one corpus directory.
type CaseResult struct {
	Name           string   `json:"name"`
	Got            []string `json:"got"`
	Allow          []string `json:"allow"`
	FalsePositives []string `json:"false_positives,omitempty"`
	FalseNegatives []string `json:"false_negatives,omitempty"`
}

// Report is the gate output.
type Report struct {
	Cases          int          `json:"cases"`
	TruePositives  int          `json:"true_positives"`
	FalsePositives int          `json:"false_positives"`
	FalseNegatives int          `json:"false_negatives"`
	Precision      float64      `json:"precision"`
	Pass           bool         `json:"pass"`
	Details        []CaseResult `json:"details,omitempty"`
}

// Marshal encodes a report.
func (r Report) Marshal() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// UnmarshalReport decodes a report.
func UnmarshalReport(b []byte) (Report, error) {
	var r Report
	err := json.Unmarshal(b, &r)
	return r, err
}

// Evaluate runs the deterministic analyzers on every case under dir.
func Evaluate(dir string) (Report, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Report{}, err
	}
	var cases []string
	for _, e := range entries {
		if e.IsDir() {
			cases = append(cases, e.Name())
		}
	}
	sort.Strings(cases)
	if len(cases) == 0 {
		return Report{}, fmt.Errorf("precision corpus %s has no cases", dir)
	}

	var rep Report
	for _, name := range cases {
		cr, err := evalCase(filepath.Join(dir, name), name)
		if err != nil {
			return Report{}, err
		}
		rep.Details = append(rep.Details, cr)
		rep.Cases++
		rep.FalsePositives += len(cr.FalsePositives)
		rep.FalseNegatives += len(cr.FalseNegatives)
		for _, g := range cr.Got {
			for _, a := range cr.Allow {
				if g == a {
					rep.TruePositives++
					break
				}
			}
		}
	}
	den := rep.TruePositives + rep.FalsePositives
	if den == 0 {
		rep.Precision = 1
	} else {
		rep.Precision = float64(rep.TruePositives) / float64(den)
	}
	rep.Pass = rep.FalsePositives == 0 && rep.Precision >= MinPrecision
	return rep, nil
}

func evalCase(dir, name string) (CaseResult, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "expect.json"))
	if err != nil {
		return CaseResult{}, fmt.Errorf("%s: %w", name, err)
	}
	var exp Expect
	if err := json.Unmarshal(raw, &exp); err != nil {
		return CaseResult{}, fmt.Errorf("%s expect.json: %w", name, err)
	}
	unit, err := unitFromDir(dir)
	if err != nil {
		return CaseResult{}, err
	}
	got := unique(ruleIDs(runAnalyzers(unit)))
	allow := unique(exp.Allow)
	cr := CaseResult{Name: name, Got: got, Allow: allow}
	allowSet := map[string]bool{}
	for _, a := range allow {
		allowSet[a] = true
	}
	gotSet := map[string]bool{}
	for _, g := range got {
		gotSet[g] = true
		if !allowSet[g] {
			cr.FalsePositives = append(cr.FalsePositives, g)
		}
	}
	for _, a := range allow {
		if !gotSet[a] {
			cr.FalseNegatives = append(cr.FalseNegatives, a)
		}
	}
	return cr, nil
}

func unitFromDir(dir string) (analyzer.AnalysisUnit, error) {
	var changed []vcs.ChangedFile
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.EqualFold(d.Name(), "expect.json") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		var added []vcs.AddedLine
		for i, line := range strings.Split(string(data), "\n") {
			if i == len(strings.Split(string(data), "\n"))-1 && line == "" {
				break
			}
			added = append(added, vcs.AddedLine{Number: i + 1, Text: line})
		}
		changed = append(changed, vcs.ChangedFile{Path: rel, Added: added})
		return nil
	})
	return analyzer.AnalysisUnit{RepoPath: dir, Changed: changed}, err
}

func runAnalyzers(unit analyzer.AnalysisUnit) []model.Finding {
	as := []analyzer.Analyzer{
		analyzer.SecretDetector{},
		analyzer.PythonLint{},
		analyzer.TypeScriptLint{},
		advisory.Scanner{DB: advisory.Bundled()},
	}
	var out []model.Finding
	for _, a := range as {
		fs, err := a.Analyze(context.Background(), unit)
		if err != nil {
			continue
		}
		out = append(out, fs...)
	}
	return out
}

func ruleIDs(fs []model.Finding) []string {
	var out []string
	for _, f := range fs {
		id := f.RuleID
		if id == "" {
			id = f.Category
		}
		out = append(out, id)
	}
	return out
}

func unique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
