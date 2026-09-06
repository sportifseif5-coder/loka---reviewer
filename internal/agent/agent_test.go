package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/provider"
	"github.com/sportifseif5-coder/loka---reviewer/internal/vcs"
)

func sampleChanged() []vcs.ChangedFile {
	return []vcs.ChangedFile{
		{Path: "a.go", Added: []vcs.AddedLine{{Number: 1, Text: "package a"}, {Number: 2, Text: "func A() {}"}}},
	}
}

func TestVerifyConfirmedAndDemoted(t *testing.T) {
	changed := sampleChanged()
	candidates := []model.Finding{
		{Location: model.Location{File: "a.go", LineStart: 2}},
		{Location: model.Location{File: "a.go", LineStart: 99}},
		{Location: model.Location{File: "b.go", LineStart: 1}},
	}
	v := NewVerificationAgent()
	out, demoted := v.Verify(candidates, changed)
	if demoted != 2 {
		t.Fatalf("demoted = %d, want 2", demoted)
	}
	if out[0].Demoted || out[0].Confidence != ConfirmConfidence {
		t.Errorf("first finding should be verified: %+v", out[0])
	}
	if !out[1].Demoted || out[1].Severity != model.SeverityInfo || out[1].Confidence != DemoteConfidence {
		t.Errorf("second finding should be demoted to info: %+v", out[1])
	}
	if !out[2].Demoted {
		t.Errorf("finding in an unchanged file must be demoted: %+v", out[2])
	}
}

func TestParseFindings(t *testing.T) {
	content := `[
		{"file":"a.go","line":2,"severity":"critical","message":"risk","reasoning":"because","rule":"x"},
		{"file":"a.go","line":3,"message":"defaulted"}
	]`
	findings, dropped, err := parseFindings(content)
	if err != nil {
		t.Fatalf("parseFindings: %v", err)
	}
	if len(findings) != 2 || dropped != 0 {
		t.Fatalf("got %d findings, %d dropped", len(findings), dropped)
	}
	if findings[0].Source != model.SourceLLM || findings[0].Severity != model.SeverityCritical || findings[0].RuleID != "x" {
		t.Errorf("first finding parsed wrong: %+v", findings[0])
	}
	if findings[1].Severity != model.SeverityWarning || findings[1].RuleID != "llm" {
		t.Errorf("second finding should default: %+v", findings[1])
	}

	if _, _, err := parseFindings("not json"); err == nil {
		t.Errorf("expected error on non-JSON output")
	}

	invalid := `[{"message":"no location"},{"file":"a.go","line":2}]`
	findings, dropped, err = parseFindings(invalid)
	if err != nil {
		t.Fatalf("parseFindings: %v", err)
	}
	if len(findings) != 0 || dropped != 2 {
		t.Errorf("got %d findings, %d dropped; want 0, 2", len(findings), dropped)
	}
}

func TestPackContextIncludesBaselineAndAddedLines(t *testing.T) {
	req := Request{
		Changed: sampleChanged(),
		Baseline: []model.Finding{
			{RuleID: "secret", Severity: model.SeverityCritical,
				Location: model.Location{File: "a.go", LineStart: 1}, Message: "hardcoded key"},
		},
	}
	ctx := packContext(req, 4096)
	for _, want := range []string{"a.go", "+2 func A() {}", "hardcoded key", "Deterministic baseline"} {
		if !strings.Contains(ctx, want) {
			t.Errorf("context missing %q:\n%s", want, ctx)
		}
	}
	if strings.Contains(ctx, "context truncated") {
		t.Errorf("large budget should not truncate:\n%s", ctx)
	}
}

func TestPackContextTruncatesByBudgetNotBaseline(t *testing.T) {
	req := Request{
		Changed: sampleChanged(),
		Baseline: []model.Finding{
			{RuleID: "secret", Severity: model.SeverityCritical,
				Location: model.Location{File: "a.go", LineStart: 1}, Message: "must survive"},
		},
	}
	ctx := packContext(req, 20)
	if !strings.Contains(ctx, "context truncated") {
		t.Errorf("tiny budget must truncate:\n%s", ctx)
	}
	if !strings.Contains(ctx, "must survive") {
		t.Errorf("baseline must never be truncated away:\n%s", ctx)
	}
}

func TestPackContextIncludesRelevantByDistanceOrder(t *testing.T) {
	req := Request{
		Changed: sampleChanged(),
		Relevant: []ContextFile{
			// Deliberately unsorted: distance 2 before distance 1.
			{Path: "far.go", Distance: 2, Code: "package a\nfunc Far() {}\n"},
			{Path: "near.go", Distance: 1, Code: "package a\nfunc Near() { A() }\n"},
		},
	}
	ctx := packContext(req, 4096)
	near := strings.Index(ctx, "--- near.go (distance 1)")
	far := strings.Index(ctx, "--- far.go (distance 2)")
	if near < 0 || far < 0 {
		t.Fatalf("relevant files missing from context:\n%s", ctx)
	}
	if near > far {
		t.Errorf("closest-impact file must sort before farthest-impact:\n%s", ctx)
	}
	for _, want := range []string{"func Near() { A() }", "func Far() {}"} {
		if !strings.Contains(ctx, want) {
			t.Errorf("context missing relevant code %q:\n%s", want, ctx)
		}
	}
	if strings.Contains(ctx, "context truncated") {
		t.Errorf("large budget should not truncate:\n%s", ctx)
	}
}

func TestPackContextRelevantTruncatedAwayKeepsBaseline(t *testing.T) {
	req := Request{
		Changed: sampleChanged(),
		Baseline: []model.Finding{
			{RuleID: "secret", Severity: model.SeverityCritical,
				Location: model.Location{File: "a.go", LineStart: 1}, Message: "must survive"},
		},
		Relevant: []ContextFile{
			{Path: "caller.go", Distance: 1, Code: "package a\nfunc Caller() { A() }\n"},
		},
	}
	ctx := packContext(req, 20)
	if !strings.Contains(ctx, "context truncated") {
		t.Errorf("tiny budget must truncate:\n%s", ctx)
	}
	if !strings.Contains(ctx, "must survive") {
		t.Errorf("baseline must survive relevant truncation:\n%s", ctx)
	}
	if strings.Contains(ctx, "func Caller") {
		t.Errorf("related code must be truncated away before the baseline:\n%s", ctx)
	}
}

func TestReviewAgentPipeline(t *testing.T) {
	response := `[
		{"file":"a.go","line":2,"severity":"error","message":"real","reasoning":"why"},
		{"file":"a.go","line":99,"severity":"warning","message":"off","reasoning":"why"},
		{"file":"a.go","message":"no line"}
	]`
	router := provider.NewRouter()
	router.Add(provider.NewFixture("fx", "qwen-test", true,
		provider.Fixture{System: SystemPrompt, Response: response}))

	ra := NewReviewAgent()
	res, err := ra.Run(context.Background(), router, true, Request{Changed: sampleChanged()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Findings) != 2 {
		t.Fatalf("findings = %d, want 2", len(res.Findings))
	}
	if res.Invalid != 1 || res.Demoted != 1 {
		t.Errorf("invalid/demoted = %d/%d, want 1/1", res.Invalid, res.Demoted)
	}
	if res.Model != "qwen-test" {
		t.Errorf("model = %q", res.Model)
	}
	var verified, demotedFinding *model.Finding
	for i := range res.Findings {
		if res.Findings[i].Demoted {
			demotedFinding = &res.Findings[i]
		} else {
			verified = &res.Findings[i]
		}
	}
	if verified == nil || verified.Severity != model.SeverityError || verified.Confidence != ConfirmConfidence {
		t.Errorf("verified finding wrong: %+v", verified)
	}
	if demotedFinding == nil || demotedFinding.Severity != model.SeverityInfo || demotedFinding.Confidence != DemoteConfidence {
		t.Errorf("demoted finding wrong: %+v", demotedFinding)
	}
}

func TestReviewAgentNoProvider(t *testing.T) {
	ra := NewReviewAgent()
	if _, err := ra.Run(context.Background(), nil, true, Request{}); !errors.Is(err, provider.ErrNoProvider) {
		t.Errorf("nil router err = %v, want ErrNoProvider", err)
	}
	if _, err := ra.Run(context.Background(), provider.NewRouter(), true, Request{}); !errors.Is(err, provider.ErrNoProvider) {
		t.Errorf("empty router err = %v, want ErrNoProvider", err)
	}
}

func TestReviewAgentUnmatchedFixtureFails(t *testing.T) {
	router := provider.NewRouter()
	router.Add(provider.NewFixture("fx", "qwen-test", true)) // no recorded entries
	ra := NewReviewAgent()
	if _, err := ra.Run(context.Background(), router, true, Request{}); err == nil {
		t.Errorf("expected an error when no fixture matches")
	}
}

func TestSplitBudget(t *testing.T) {
	c, o := splitBudget(0)
	if c+o != DefaultBudget {
		t.Errorf("splitBudget(0) = %d+%d, want sum %d", c, o, DefaultBudget)
	}
	c, o = splitBudget(12000)
	if c != 9000 || o != 3000 {
		t.Errorf("splitBudget(12000) = %d+%d, want 9000+3000", c, o)
	}
}
