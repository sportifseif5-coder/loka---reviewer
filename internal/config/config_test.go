package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Mode != ModeOffline {
		t.Fatalf("default mode = %q, want offline", cfg.Mode)
	}
	if len(cfg.Providers) != 1 {
		t.Fatalf("default providers = %d, want 1", len(cfg.Providers))
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config invalid: %v", err)
	}
}

func TestParseValid(t *testing.T) {
	data := []byte(`
mode: online
providers:
  - name: ollama
    model: qwen2.5-coder:14b
budgets:
  review_tokens: 20000
rules:
  include: [review_rules.md]
  sync: [cursor, copilot]
`)
	cfg, warnings, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if cfg.Mode != ModeOnline {
		t.Errorf("mode = %q, want online", cfg.Mode)
	}
	if cfg.Providers[0].Model != "qwen2.5-coder:14b" {
		t.Errorf("model = %q", cfg.Providers[0].Model)
	}
	if cfg.Budgets.ReviewTokens != 20000 {
		t.Errorf("review_tokens = %d", cfg.Budgets.ReviewTokens)
	}
	if len(cfg.Rules.Sync) != 2 {
		t.Errorf("rules.sync = %v", cfg.Rules.Sync)
	}
}

func TestParseUnknownKeyWarns(t *testing.T) {
	data := []byte("mode: offline\nbananas: true\n")
	_, warnings, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected a warning for unknown key")
	}
	if !strings.Contains(warnings[0], "bananas") {
		t.Fatalf("warning %q does not mention the unknown key", warnings[0])
	}
}

func TestParseUnknownNestedKeyWarns(t *testing.T) {
	data := []byte("providers:\n  - name: ollama\n    model: x\n    banana: 1\n")
	_, warnings, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "providers.banana") {
			found = true
		}
	}
	if !found {
		t.Fatalf("warnings %v do not include providers.banana", warnings)
	}
}

func TestParseBadMode(t *testing.T) {
	cfg, _, err := Parse([]byte("mode: sideways\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for invalid mode")
	}
}

func TestMergeOverlayWins(t *testing.T) {
	base := Default()
	overlay, _, err := Parse([]byte("mode: agent\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	base.Merge(overlay, map[string]bool{"mode": true})
	if base.Mode != ModeAgent {
		t.Errorf("merged mode = %q, want agent", base.Mode)
	}
	if base.Budgets.ReviewTokens != Default().Budgets.ReviewTokens {
		t.Errorf("merge overwrote untouched budgets")
	}
}

func TestMergeOnlyMergesPresentFields(t *testing.T) {
	base := Default()
	base.Budgets.ReviewTokens = 999
	overlay, _, err := Parse([]byte("mode: online\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	base.Merge(overlay, map[string]bool{"mode": true})
	if base.Budgets.ReviewTokens != 999 {
		t.Errorf("review_tokens = %d, want 999 (untouched by overlay)", base.Budgets.ReviewTokens)
	}
	if base.Mode != ModeOnline {
		t.Errorf("mode = %q, want online", base.Mode)
	}
}

func TestLoadMissingFileWarns(t *testing.T) {
	_, warnings, err := Load("/nonexistent/review.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected warning for missing file")
	}
}

func TestLoadForRepoInheritance(t *testing.T) {
	dir := t.TempDir()
	userDir := filepath.Join(dir, "user")
	repoDir := filepath.Join(dir, "repo")
	if err := os.MkdirAll(filepath.Join(userDir, "loka"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repoDir, ".loka"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Point the user scope at the temp dir, then layer repo-local config on
	// top using the default (offline) base.
	t.Setenv("XDG_CONFIG_HOME", userDir)
	userFile := filepath.Join(userDir, "loka", "review.yaml")
	if err := os.WriteFile(userFile, []byte("budgets:\n  review_tokens: 999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repoFile := filepath.Join(repoDir, ".loka", "review.yaml")
	if err := os.WriteFile(repoFile, []byte("mode: agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadForRepo(repoDir, "")
	if err != nil {
		t.Fatalf("LoadForRepo: %v", err)
	}
	if cfg.Mode != ModeAgent {
		t.Errorf("mode = %q, want agent (repo-local wins)", cfg.Mode)
	}
	if cfg.Budgets.ReviewTokens != 999 {
		t.Errorf("review_tokens = %d, want 999 (user scope inherited)", cfg.Budgets.ReviewTokens)
	}
}

func TestProviderIsLocal(t *testing.T) {
	local := true
	remote := false
	explicitLocal := Provider{Name: "ollama", Local: &local}
	explicitRemote := Provider{Name: "p", Local: &remote}
	noURL := Provider{Name: "p", BaseURL: ""}
	withURL := Provider{Name: "p", BaseURL: "https://api.example.com"}
	if !explicitLocal.IsLocal() {
		t.Error("explicit local should be local")
	}
	if explicitRemote.IsLocal() {
		t.Error("explicit remote should not be local")
	}
	if !noURL.IsLocal() {
		t.Error("no base URL implies local")
	}
	if withURL.IsLocal() {
		t.Error("base URL set implies remote")
	}
}
