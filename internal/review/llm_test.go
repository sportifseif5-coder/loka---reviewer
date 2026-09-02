package review

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sportifseif5-coder/loka---reviewer/internal/agent"
	"github.com/sportifseif5-coder/loka---reviewer/internal/analyzer"
	"github.com/sportifseif5-coder/loka---reviewer/internal/config"
	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/provider"
	"github.com/sportifseif5-coder/loka---reviewer/internal/rules"
	"github.com/sportifseif5-coder/loka---reviewer/internal/store"
	"github.com/sportifseif5-coder/loka---reviewer/internal/vcs"
)

// fixtureAgentRouter builds a router with one local fixture provider whose
// recorded response fires for the review prompt (system matches, prompt
// wildcard). remote flips the fixture to a remote routing locality.
func fixtureAgentRouter(remote bool, response string) *provider.Router {
	r := provider.NewRouter()
	r.Add(provider.NewFixture("fx", "qwen-test", !remote,
		provider.Fixture{System: agent.SystemPrompt, Response: response}))
	return r
}

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

	// app.go is a newly added file, so line 7 is an added line -> verified.
	eng.RegisterProviderRouter(fixtureAgentRouter(false, `[{"file":"app.go","line":7,"severity":"warning",
		"message":"println swallows errors","reasoning":"use fmt.Fprintf to a writer"}]`))

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
	if llm.Demoted {
		t.Errorf("finding on an added line must be verified, not demoted")
	}
	if llm.Confidence != agent.ConfirmConfidence {
		t.Errorf("verified confidence = %v, want %v", llm.Confidence, agent.ConfirmConfidence)
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

func TestLLMStageDemotesNonAddedLine(t *testing.T) {
	repo := fixtureRepo(t)
	// Make base.go a modified file: its committed line 1 is now unchanged.
	content := []byte("package e2e\n\n// Added is new.\nfunc Added() {}\n")
	if err := os.WriteFile(filepath.Join(repo, "base.go"), content, 0o644); err != nil {
		t.Fatal(err)
	}

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

	// base.go:1 is the unchanged package clause -> must be demoted.
	eng.RegisterProviderRouter(fixtureAgentRouter(false, `[{"file":"base.go","line":1,"severity":"error",
		"message":"something is off","reasoning":"flagged anyway"}]`))

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
		t.Fatalf("no LLM finding present")
	}
	if !llm.Demoted {
		t.Errorf("finding off added lines must be demoted")
	}
	if llm.Severity != model.SeverityInfo {
		t.Errorf("demoted severity = %q, want info", llm.Severity)
	}
	if llm.Confidence != agent.DemoteConfidence {
		t.Errorf("demoted confidence = %v, want %v", llm.Confidence, agent.DemoteConfidence)
	}

	found := false
	for _, d := range res.Degradations {
		if contains(d, "demoted 1 finding(s)") {
			found = true
		}
	}
	if !found {
		t.Errorf("Degradations = %v, want a demotion note", res.Degradations)
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

	eng.RegisterProviderRouter(fixtureAgentRouter(true, `[{"file":"app.go","line":7,"message":"x"}]`))

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
