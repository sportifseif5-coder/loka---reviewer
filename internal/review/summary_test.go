package review

import (
	"strings"
	"testing"

	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/vcs"
)

func TestSummarizeEmpty(t *testing.T) {
	got := summarize(nil, nil)
	if got.Risk != model.RiskNone {
		t.Errorf("risk = %q, want %s", got.Risk, model.RiskNone)
	}
	want := "No working-tree changes. No findings."
	if got.Walkthrough != want {
		t.Errorf("walkthrough = %q, want %q", got.Walkthrough, want)
	}
}

func TestSummarizeOneFileNoFindings(t *testing.T) {
	got := summarize([]vcs.ChangedFile{{Path: "a.go"}}, nil)
	if got.Risk != model.RiskNone {
		t.Errorf("risk = %q, want %s", got.Risk, model.RiskNone)
	}
	want := "Reviewed 1 changed file (a.go). No findings."
	if got.Walkthrough != want {
		t.Errorf("walkthrough = %q, want %q", got.Walkthrough, want)
	}
}

func TestSummarizeRanksAndCounts(t *testing.T) {
	changed := []vcs.ChangedFile{{Path: "secret.txt"}, {Path: "app.go"}}
	findings := []model.Finding{
		{RuleID: "debug-print-in-go", Category: "style", Severity: model.SeverityWarning,
			Location: model.Location{File: "app.go", LineStart: 7}},
		{RuleID: "secret.aws-secret-key", Category: "security", Severity: model.SeverityCritical,
			Location: model.Location{File: "secret.txt", LineStart: 1}},
	}
	rank(findings)
	got := summarize(changed, findings)
	if got.Risk != model.RiskCritical {
		t.Errorf("risk = %q, want %s", got.Risk, model.RiskCritical)
	}
	if !strings.Contains(got.Walkthrough, "Reviewed 2 changed files (app.go, secret.txt).") {
		t.Errorf("walkthrough missing sorted files: %q", got.Walkthrough)
	}
	if !strings.Contains(got.Walkthrough, "2 findings: 1 critical, 1 warning.") {
		t.Errorf("walkthrough missing counts: %q", got.Walkthrough)
	}
	if !strings.Contains(got.Walkthrough, "Highest: critical secret.aws-secret-key in secret.txt:1.") {
		t.Errorf("walkthrough missing highest: %q", got.Walkthrough)
	}
}

func TestSummarizeRiskLevels(t *testing.T) {
	cases := []struct {
		sev  model.Severity
		want string
	}{
		{model.SeverityCritical, model.RiskCritical},
		{model.SeverityError, model.RiskHigh},
		{model.SeverityWarning, model.RiskMedium},
		{model.SeverityInfo, model.RiskLow},
	}
	for _, tc := range cases {
		got := summarize(nil, []model.Finding{{Severity: tc.sev, Location: model.Location{File: "a.go", LineStart: 1}}})
		if got.Risk != tc.want {
			t.Errorf("severity %s: risk = %q, want %s", tc.sev, got.Risk, tc.want)
		}
	}
}

func TestSummarizeDemotedOnlyIsLow(t *testing.T) {
	got := summarize(nil, []model.Finding{{
		Severity: model.SeverityInfo,
		Demoted:  true,
		Location: model.Location{File: "a.go", LineStart: 1},
	}})
	if got.Risk != model.RiskLow {
		t.Errorf("risk = %q, want %s", got.Risk, model.RiskLow)
	}
	if !strings.Contains(got.Walkthrough, "(1 demoted).") {
		t.Errorf("walkthrough missing demoted count: %q", got.Walkthrough)
	}
	if strings.Contains(got.Walkthrough, "Highest:") {
		t.Errorf("demoted-only summary should omit Highest: %q", got.Walkthrough)
	}
}

func TestSummarizeSkipsDemotedHighest(t *testing.T) {
	findings := []model.Finding{
		{RuleID: "real", Category: "correctness", Severity: model.SeverityWarning,
			Location: model.Location{File: "a.go", LineStart: 3}},
		{RuleID: "guess", Category: "correctness", Severity: model.SeverityInfo, Demoted: true,
			Location: model.Location{File: "b.go", LineStart: 1}},
	}
	got := summarize(nil, findings)
	if got.Risk != model.RiskMedium {
		t.Errorf("risk = %q, want %s", got.Risk, model.RiskMedium)
	}
	if !strings.Contains(got.Walkthrough, "Highest: warning real in a.go:3.") {
		t.Errorf("highest should skip demoted: %q", got.Walkthrough)
	}
}

func TestSummarizeCapsFileList(t *testing.T) {
	changed := []vcs.ChangedFile{
		{Path: "e.go"}, {Path: "c.go"}, {Path: "a.go"}, {Path: "d.go"}, {Path: "b.go"},
	}
	got := summarize(changed, nil)
	want := "Reviewed 5 changed files (a.go, b.go, c.go, +2 more). No findings."
	if got.Walkthrough != want {
		t.Errorf("walkthrough = %q, want %q", got.Walkthrough, want)
	}
}

func TestSummarizeDeterministicPathOrder(t *testing.T) {
	a := summarize([]vcs.ChangedFile{{Path: "z.go"}, {Path: "a.go"}}, nil)
	b := summarize([]vcs.ChangedFile{{Path: "a.go"}, {Path: "z.go"}}, nil)
	if a != b {
		t.Errorf("path order leaked into summary:\n%q\n%q", a.Walkthrough, b.Walkthrough)
	}
}

func TestSummarizeSingularFinding(t *testing.T) {
	got := summarize(nil, []model.Finding{{
		RuleID:   "r1",
		Category: "c",
		Severity: model.SeverityError,
		Location: model.Location{File: "a.go", LineStart: 2},
	}})
	if got.Risk != model.RiskHigh {
		t.Errorf("risk = %q, want %s", got.Risk, model.RiskHigh)
	}
	if !strings.Contains(got.Walkthrough, "1 finding: 1 error.") {
		t.Errorf("walkthrough missing singular finding: %q", got.Walkthrough)
	}
}

func TestSummarizeUsesCategoryWhenRuleIDEmpty(t *testing.T) {
	got := summarize(nil, []model.Finding{{
		Category: "correctness",
		Severity: model.SeverityWarning,
		Location: model.Location{File: "a.go", LineStart: 4},
	}})
	if !strings.Contains(got.Walkthrough, "Highest: warning correctness in a.go:4.") {
		t.Errorf("walkthrough missing category fallback: %q", got.Walkthrough)
	}
}
