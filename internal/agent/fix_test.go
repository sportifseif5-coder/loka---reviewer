package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/vcs"
)

func TestFixAgentProposesWhitespacePatch(t *testing.T) {
	findings := []model.Finding{{
		RuleID:   "trailing-whitespace",
		Location: model.Location{File: "a.go", LineStart: 1, LineEnd: 1},
		Message:  "trailing whitespace",
		Demoted:  false,
		Severity: model.SeverityInfo,
		Source:   model.SourceRule,
	}}
	changed := []vcs.ChangedFile{{
		Path:  "a.go",
		Added: []vcs.AddedLine{{Number: 1, Text: "package a  "}},
	}}
	out := NewFixAgent().Propose(findings, changed)
	if out[0].SuggestedFix == "" {
		t.Fatal("expected a suggested fix")
	}
	if !strings.Contains(out[0].SuggestedFix, "package a") {
		t.Errorf("patch = %q", out[0].SuggestedFix)
	}
}

func TestFixAgentSkipsDemoted(t *testing.T) {
	findings := []model.Finding{{
		RuleID:   "trailing-whitespace",
		Location: model.Location{File: "a.go", LineStart: 1},
		Demoted:  true,
	}}
	changed := []vcs.ChangedFile{{
		Path:  "a.go",
		Added: []vcs.AddedLine{{Number: 1, Text: "package a  "}},
	}}
	out := NewFixAgent().Propose(findings, changed)
	if out[0].SuggestedFix != "" {
		t.Errorf("demoted finding should not get a patch: %q", out[0].SuggestedFix)
	}
}

func TestApplySuggestionReplacesLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.go")
	if err := os.WriteFile(path, []byte("package a  \nfunc A() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	patch := unifiedLinePatch("a.go", 1, "package a  ", "package a")
	if err := ApplySuggestion(dir, "a.go", patch); err != nil {
		t.Fatalf("ApplySuggestion: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "package a\nfunc A() {}\n" {
		t.Fatalf("file = %q", got)
	}
}

func TestApplySuggestionRejectsEscape(t *testing.T) {
	dir := t.TempDir()
	err := ApplySuggestion(dir, "../outside.go", unifiedLinePatch("../outside.go", 1, "a", "b"))
	if err == nil {
		t.Fatal("expected path-escape error")
	}
}
