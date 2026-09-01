package review

import (
	"strings"
	"testing"

	"github.com/sportifseif5-coder/loka---reviewer/internal/analyzer"
	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/vcs"
)

func TestParseLLMFindingsValid(t *testing.T) {
	content := `[
		{"file":"a.go","line":3,"severity":"critical","message":"injection risk","reasoning":"untrusted input","rule":"inject"},
		{"file":"b.go","line":9,"message":"missing close"}
	]`
	findings, dropped, err := parseLLMFindings(content)
	if err != nil {
		t.Fatalf("parseLLMFindings: %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("got %d findings, want 2", len(findings))
	}
	if findings[0].Source != model.SourceLLM || findings[0].Severity != model.SeverityCritical {
		t.Errorf("first finding source/severity = %s/%s", findings[0].Source, findings[0].Severity)
	}
	if findings[1].Severity != model.SeverityWarning {
		t.Errorf("default severity = %q, want warning", findings[1].Severity)
	}
	if findings[0].RuleID != "inject" || findings[1].RuleID != "llm" {
		t.Errorf("rule ids = %q, %q", findings[0].RuleID, findings[1].RuleID)
	}
	if dropped != 0 {
		t.Errorf("dropped = %d, want 0", dropped)
	}
}

func TestParseLLMFindingsDropsInvalid(t *testing.T) {
	content := `[
		{"file":"a.go","line":3,"message":"ok"},
		{"message":"no location"},
		{"file":"b.go","message":"no line"}
	]`
	findings, dropped, err := parseLLMFindings(content)
	if err != nil {
		t.Fatalf("parseLLMFindings: %v", err)
	}
	if len(findings) != 1 {
		t.Errorf("got %d findings, want 1 valid", len(findings))
	}
	if dropped != 2 {
		t.Errorf("dropped = %d, want 2", dropped)
	}
}

func TestParseLLMFindingsBadJSON(t *testing.T) {
	if _, _, err := parseLLMFindings("not json"); err == nil {
		t.Errorf("expected error on non-JSON output")
	}
}

func TestPackContextIncludesAddedLinesAndBaseline(t *testing.T) {
	// Build a minimal unit by hand to avoid a repo fixture.
	u := analyzer.AnalysisUnit{
		Changed: []vcs.ChangedFile{
			{Path: "a.go", Added: []vcs.AddedLine{{Number: 1, Text: "secret := \"x\""}}},
		},
	}
	findings := []model.Finding{
		{RuleID: "secret", Severity: model.SeverityCritical,
			Location: model.Location{File: "a.go", LineStart: 1}, Message: "hardcoded key"},
	}
	ctx := packContext(u, findings)
	if !strings.Contains(ctx, "a.go") || !strings.Contains(ctx, "+1 ") {
		t.Errorf("context missing changed-file content:\n%s", ctx)
	}
	if !strings.Contains(ctx, "hardcoded key") {
		t.Errorf("context missing baseline finding:\n%s", ctx)
	}
}
