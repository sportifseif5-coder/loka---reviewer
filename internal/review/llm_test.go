package review

import (
	"context"
	"testing"

	"github.com/sportifseif5-coder/loka---reviewer/internal/analyzer"
	"github.com/sportifseif5-coder/loka---reviewer/internal/config"
	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/provider"
	"github.com/sportifseif5-coder/loka---reviewer/internal/rules"
	"github.com/sportifseif5-coder/loka---reviewer/internal/store"
	"github.com/sportifseif5-coder/loka---reviewer/internal/vcs"
)

func TestLLMStageMergesFindings(t *testing.T) {
	repo := fixtureRepo(t)
	cfg := config.Default()
	s, err := store.Open("")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	eng := NewEngine(cfg, s)
	eng.RegisterVCS(vcs.NewGit())
	eng.RegisterAnalyzer(analyzer.SecretDetector{})
	eng.RegisterRules(rules.Defaults())

	fx := provider.NewFixture("fx", "qwen-test", true,
		provider.Fixture{
			System: llmSystemPrompt,
			Response: `[{"file":"app.go","line":4,"severity":"error",
				"message":"missing nil check","reasoning":"x may be nil"}]`,
		})
	r := provider.NewRouter()
	r.Add(fx)
	eng.RegisterProviderRouter(r)

	res, err := eng.Review(context.Background(), model.ReviewRequest{RepoPath: repo})
	if err != nil {
		t.Fatalf("Review: %v", err)
	}

	var llm *model.Finding
	for i := range res.Findings {
		if res.Findings[i].Source == model.SourceLLM {
			llm = &res.Findings[i]
		}
	}
	if llm == nil {
		t.Fatalf("no LLM finding merged (%d findings)", len(res.Findings))
	}
	if llm.Location.File != "app.go" || llm.Location.LineStart != 4 {
		t.Errorf("llm location = %+v, want app.go:4", llm.Location)
	}
	if llm.Severity != model.SeverityError {
		t.Errorf("llm severity = %q, want error", llm.Severity)
	}
	if llm.Reasoning == "" {
		t.Errorf("llm finding must carry reasoning (I8)")
	}

	found := false
	for _, a := range res.AnalyzersRun {
		if a == "llm:qwen-test" {
			found = true
		}
	}
	if !found {
		t.Errorf("AnalyzersRun = %v, want llm:qwen-test recorded", res.AnalyzersRun)
	}
}

func TestLLMStageOfflineExcludesRemote(t *testing.T) {
	repo := fixtureRepo(t)
	cfg := config.Default() // mode is offline
	s, err := store.Open("")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	eng := NewEngine(cfg, s)
	eng.RegisterVCS(vcs.NewGit())
	eng.RegisterAnalyzer(analyzer.SecretDetector{})
	eng.RegisterRules(rules.Defaults())

	fx := provider.NewFixture("remote-fx", "qwen-test", false,
		provider.Fixture{System: llmSystemPrompt, Response: `[{"file":"app.go","line":4,"message":"x"}]`})
	r := provider.NewRouter()
	r.Add(fx)
	eng.RegisterProviderRouter(r)

	res, err := eng.Review(context.Background(), model.ReviewRequest{RepoPath: repo})
	if err != nil {
		t.Fatalf("Review: %v", err)
	}

	for _, f := range res.Findings {
		if f.Source == model.SourceLLM {
			t.Errorf("offline mode produced an LLM finding from a remote provider")
		}
	}
	if len(res.Degradations) == 0 {
		t.Errorf("expected an LLM-stage degradation in offline mode")
	}
}

func TestLLMStageFailureDegradesAndKeepsBaseline(t *testing.T) {
	repo := fixtureRepo(t)
	cfg := config.Default()
	s, err := store.Open("")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	eng := NewEngine(cfg, s)
	eng.RegisterVCS(vcs.NewGit())
	eng.RegisterAnalyzer(analyzer.SecretDetector{})
	eng.RegisterRules(rules.Defaults())

	// No recorded fixture -> Complete fails -> LLM stage skipped.
	r := provider.NewRouter()
	r.Add(provider.NewFixture("fx", "qwen-test", true))
	eng.RegisterProviderRouter(r)

	res, err := eng.Review(context.Background(), model.ReviewRequest{RepoPath: repo})
	if err != nil {
		t.Fatalf("Review: %v", err)
	}

	if len(res.Degradations) == 0 {
		t.Errorf("expected a degradation when the LLM stage fails")
	}
	// Baseline must still ship: secret + debug-print findings.
	var secrets, debug int
	for _, f := range res.Findings {
		switch f.RuleID {
		case "secret.aws-secret-key":
			secrets++
		case "debug-print-in-go":
			debug++
		}
	}
	if secrets < 1 || debug < 1 {
		t.Errorf("baseline lost on LLM failure: secret=%d debug=%d", secrets, debug)
	}
}
