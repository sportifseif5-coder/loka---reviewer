package review

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sportifseif5-coder/loka---reviewer/internal/config"
	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/provider"
	"github.com/sportifseif5-coder/loka---reviewer/internal/store"
	"github.com/sportifseif5-coder/loka---reviewer/internal/vcs"
)

// captureProvider records the prompt it was handed and returns an empty
// finding list, letting tests assert what reached the context pack without
// depending on an LLM.
type captureProvider struct {
	prompt  string
	content string
}

func (c *captureProvider) Name() string { return "capture" }
func (c *captureProvider) Local() bool  { return true }
func (c *captureProvider) Available(context.Context) bool {
	return true
}
func (c *captureProvider) Complete(_ context.Context, req provider.Request) (provider.Result, error) {
	c.prompt = req.Prompt
	return provider.Result{Content: c.content, Model: "capture"}, nil
}

// impactFixtureRepo builds a git repo with two committed Go files in the same
// package where caller.go references a symbol declared in callee.go, then
// modifies callee.go in the working tree so the diff is the callee file.
func impactFixtureRepo(t *testing.T) string {
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

	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("callee.go", "package e2e\n\n// Callee returns one.\nfunc Callee() int { return 1 }\n")
	write("caller.go", "package e2e\n\n// Caller uses Callee from another file.\nfunc Caller() int { return Callee() }\n")
	run("add", ".")
	run("commit", "-q", "-m", "base")

	// Modify the callee in the working tree: its hash changes, so the index
	// sync re-parses the package, and ImpactSet on callee.go must surface
	// caller.go as a distance-1 neighbour.
	write("callee.go", "package e2e\n\n// Callee returns one.\nfunc Callee() int { return 1 }\n\n// CalleeTwo is new.\nfunc CalleeTwo() int { return 2 }\n")
	return dir
}

func TestLLMStageContextCarriesImpactFile(t *testing.T) {
	repo := impactFixtureRepo(t)
	cfg := config.Default()
	s, err := store.Open("")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	eng := NewEngine(cfg, s)
	eng.RegisterVCS(vcs.NewGit())

	cap := &captureProvider{content: "[]"}
	router := provider.NewRouter()
	router.Add(cap)
	eng.RegisterProviderRouter(router)

	res, err := eng.Review(context.Background(), model.ReviewRequest{RepoPath: repo})
	if err != nil {
		t.Fatalf("Review: %v", err)
	}

	if cap.prompt == "" {
		t.Fatal("LLM stage never ran (no prompt captured)")
	}
	// caller.go references the changed callee.go, so it must be in the pack
	// as a distance-1 impact file.
	for _, want := range []string{
		"Related code (files impacted by the change):",
		"--- caller.go (distance 1)",
		"func Caller() int { return Callee() }",
	} {
		if !strings.Contains(cap.prompt, want) {
			t.Errorf("prompt missing %q:\n%s", want, cap.prompt)
		}
	}
	if len(res.Degradations) != 0 {
		t.Errorf("unexpected degradations: %v", res.Degradations)
	}
}
