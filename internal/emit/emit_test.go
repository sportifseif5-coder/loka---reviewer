package emit

import (
	"context"
	"strings"
	"testing"

	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
)

type capturePoster struct {
	calls []PostedComment
}

func (c *capturePoster) Post(_ context.Context, comment PostedComment) error {
	c.calls = append(c.calls, comment)
	return nil
}

func sampleResult() *model.ReviewResult {
	return &model.ReviewResult{
		ID:   "rev1",
		Mode: "offline",
		Findings: []model.Finding{{
			ID:        "f1",
			RuleID:    "secret.github-token",
			Category:  "secret",
			Severity:  model.SeverityCritical,
			Source:    model.SourceAnalyzer,
			Location:  model.Location{File: "cmd/main.go", LineStart: 12, LineEnd: 12},
			Message:   "GitHub personal access token committed to source",
			Reasoning: "bundled secret pattern",
		}},
	}
}

func TestFormatGitHubInlineComment(t *testing.T) {
	body := Format(sampleResult().Findings[0], "github")
	if !strings.Contains(body, "cmd/main.go:12") {
		t.Errorf("body missing location: %s", body)
	}
	if !strings.Contains(body, "GitHub personal access token") {
		t.Errorf("body missing message: %s", body)
	}
	if strings.Contains(strings.ToLower(body), "merge") {
		t.Errorf("comment must not mention merge: %s", body)
	}
}

func TestEmitterPostsCommentsNotMerge(t *testing.T) {
	p := &capturePoster{}
	e := Emitter{Poster: p}
	n, err := e.Emit(context.Background(), Target{Host: HostGitHub, Owner: "o", Repo: "r", Number: 7}, sampleResult())
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if n != 1 {
		t.Fatalf("posted %d, want 1", n)
	}
	if len(p.calls) != 1 {
		t.Fatalf("poster calls = %d", len(p.calls))
	}
	c := p.calls[0]
	if c.Path != "cmd/main.go" || c.Line != 12 {
		t.Errorf("anchor = %s:%d", c.Path, c.Line)
	}
	if c.Merge || c.Push {
		t.Fatal("emit-only: Merge/Push must stay false")
	}
}

func TestEmitterHasNoMergeSurface(t *testing.T) {
	var e Emitter
	type forbidden interface {
		Merge(context.Context, Target) error
		Push(context.Context, Target) error
		Approve(context.Context, Target) error
	}
	if _, ok := any(&e).(forbidden); ok {
		t.Fatal("emitter implements a merge/push/approve method (I7)")
	}
}

func TestParseTarget(t *testing.T) {
	gh, err := ParseTarget("https://github.com/acme/app/pull/42")
	if err != nil {
		t.Fatalf("github: %v", err)
	}
	if gh.Host != HostGitHub || gh.Owner != "acme" || gh.Repo != "app" || gh.Number != 42 {
		t.Errorf("github target = %+v", gh)
	}
	gl, err := ParseTarget("https://gitlab.com/acme/app/-/merge_requests/9")
	if err != nil {
		t.Fatalf("gitlab: %v", err)
	}
	if gl.Host != HostGitLab || gl.Number != 9 {
		t.Errorf("gitlab target = %+v", gl)
	}
}

func TestDryRunDoesNotPost(t *testing.T) {
	p := &capturePoster{}
	e := Emitter{Poster: p, DryRun: true}
	n, err := e.Emit(context.Background(), Target{Host: HostGitHub, Owner: "o", Repo: "r", Number: 1}, sampleResult())
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if n != 1 {
		t.Fatalf("dry-run count = %d", n)
	}
	if len(p.calls) != 0 {
		t.Fatalf("dry-run posted: %+v", p.calls)
	}
}
