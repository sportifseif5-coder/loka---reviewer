package analyzer

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
)

type langPattern struct {
	rule     string
	severity model.Severity
	message  string
	re       *regexp.Regexp
}

func scanLang(unit AnalysisUnit, exts map[string]bool, rulePrefix string, pats []langPattern) []model.Finding {
	var out []model.Finding
	for _, cf := range unit.Changed {
		ext := strings.ToLower(filepath.Ext(cf.Path))
		if !exts[ext] {
			continue
		}
		for _, ln := range cf.Added {
			for _, p := range pats {
				if !p.re.MatchString(ln.Text) {
					continue
				}
				out = append(out, model.Finding{
					RuleID:     p.rule,
					Category:   "lint",
					Severity:   p.severity,
					Source:     model.SourceAnalyzer,
					Location:   model.Location{File: cf.Path, LineStart: ln.Number, LineEnd: ln.Number},
					Message:    p.message,
					Reasoning:  "offline " + rulePrefix + " lint bridge",
					Evidence:   []string{strings.TrimSpace(ln.Text)},
					Confidence: 1.0,
				})
			}
		}
	}
	return out
}

// PythonLint is the Phase 2 Python lint bridge: local pattern checks on
// added lines. Missing ruff/python binaries degrade to this offline set.
type PythonLint struct{}

// Name returns the analyzer name.
func (PythonLint) Name() string { return "python-lint" }

var pythonExts = map[string]bool{".py": true}

var pythonPats = []langPattern{
	{
		rule:     "python.pickle-loads",
		severity: model.SeverityError,
		message:  "pickle.loads can execute arbitrary code",
		re:       regexp.MustCompile(`pickle\.loads\s*\(`),
	},
	{
		rule:     "python.yaml-load",
		severity: model.SeverityWarning,
		message:  "yaml.load without a SafeLoader is unsafe",
		re:       regexp.MustCompile(`yaml\.load\s*\(`),
	},
	{
		rule:     "python.eval",
		severity: model.SeverityError,
		message:  "eval() on untrusted input is a code-execution sink",
		re:       regexp.MustCompile(`\beval\s*\(`),
	},
}

// Analyze scans added Python lines.
func (PythonLint) Analyze(ctx context.Context, unit AnalysisUnit) ([]model.Finding, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return scanLang(unit, pythonExts, "python", pythonPats), nil
}

// TypeScriptLint is the Phase 2 TypeScript/JavaScript lint bridge.
type TypeScriptLint struct{}

// Name returns the analyzer name.
func (TypeScriptLint) Name() string { return "typescript-lint" }

var tsExts = map[string]bool{".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true}

var tsPats = []langPattern{
	{
		rule:     "ts.inner-html",
		severity: model.SeverityWarning,
		message:  "assigning innerHTML can introduce XSS",
		re:       regexp.MustCompile(`\.innerHTML\s*=`),
	},
	{
		rule:     "ts.eval",
		severity: model.SeverityError,
		message:  "eval() on untrusted input is a code-execution sink",
		re:       regexp.MustCompile(`\beval\s*\(`),
	},
	{
		rule:     "ts.document-write",
		severity: model.SeverityWarning,
		message:  "document.write is an XSS sink",
		re:       regexp.MustCompile(`document\.write\s*\(`),
	},
}

// Analyze scans added TypeScript/JavaScript lines.
func (TypeScriptLint) Analyze(ctx context.Context, unit AnalysisUnit) ([]model.Finding, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return scanLang(unit, tsExts, "typescript", tsPats), nil
}
