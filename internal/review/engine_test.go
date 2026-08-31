package review

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sportifseif5-coder/loka---reviewer/internal/analyzer"
	"github.com/sportifseif5-coder/loka---reviewer/internal/config"
	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/rules"
	"github.com/sportifseif5-coder/loka---reviewer/internal/store"
	"github.com/sportifseif5-coder/loka---reviewer/internal/vcs"
)

// fixtureRepo builds a git repo with one committed baseline file and one
// added file containing a secret and a debug print.
func fixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "master")
	run("config", "user.name", "t")
	run("config", "user.email", "t@t")
	if err := os.WriteFile(filepath.Join(dir, "base.go"), []byte("package e2e\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "base")

	if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte(`package e2e

import "fmt"

// Run prints a debug line.
func Run() {
	fmt.Println("debug")
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("aws_secret_access_key = \"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestFullPipelineFindsAndPersists(t *testing.T) {
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

	res, err := eng.Review(context.Background(), model.ReviewRequest{RepoPath: repo})
	if err != nil {
		t.Fatalf("Review: %v", err)
	}

	var hasSecret, hasDebug bool
	for _, f := range res.Findings {
		switch f.RuleID {
		case "secret.aws-secret-key":
			hasSecret = true
			if f.Severity != model.SeverityCritical {
				t.Errorf("secret severity = %q, want critical", f.Severity)
			}
			for _, ev := range f.Evidence {
				if contains(ev, "wJalrXUtnFEMI") {
					t.Errorf("evidence not redacted: %q", ev)
				}
			}
		case "debug-print-in-go":
			hasDebug = true
		}
	}
	if !hasSecret || !hasDebug {
		t.Fatalf("missing expected findings: secret=%v debug=%v (%d findings)",
			hasSecret, hasDebug, len(res.Findings))
	}

	loaded, err := s.GetReview(context.Background(), res.ID)
	if err != nil {
		t.Fatalf("GetReview: %v", err)
	}
	if len(loaded.Findings) != len(res.Findings) {
		t.Errorf("persisted %d findings, want %d", len(loaded.Findings), len(res.Findings))
	}
}

func TestDedupeRemovesDuplicates(t *testing.T) {
	base := []model.Finding{
		{RuleID: "r1", Category: "c", Severity: model.SeverityWarning,
			Location: model.Location{File: "a.go", LineStart: 3}, Confidence: 1},
		{RuleID: "r1", Category: "c", Severity: model.SeverityWarning,
			Location: model.Location{File: "a.go", LineStart: 3}, Confidence: 1},
		{RuleID: "r2", Category: "c", Severity: model.SeverityError,
			Location: model.Location{File: "a.go", LineStart: 3}, Confidence: 1},
	}
	out := dedupe(base)
	if len(out) != 2 {
		t.Fatalf("dedupe left %d findings, want 2", len(out))
	}
	if out[0].RuleID != "r1" || out[1].RuleID != "r2" {
		t.Errorf("dedupe order = %q, %q", out[0].RuleID, out[1].RuleID)
	}
}

func TestRankOrdersBySeverity(t *testing.T) {
	base := []model.Finding{
		{RuleID: "info", Severity: model.SeverityInfo,
			Location: model.Location{File: "a.go", LineStart: 1}},
		{RuleID: "crit", Severity: model.SeverityCritical,
			Location: model.Location{File: "a.go", LineStart: 1}},
		{RuleID: "warn", Severity: model.SeverityWarning,
			Location: model.Location{File: "a.go", LineStart: 1}},
	}
	rank(base)
	if base[0].RuleID != "crit" || base[2].RuleID != "info" {
		t.Errorf("rank order = %q, %q, %q", base[0].RuleID, base[1].RuleID, base[2].RuleID)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
