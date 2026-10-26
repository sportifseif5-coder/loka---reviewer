package rules

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingIncludeWarns(t *testing.T) {
	rs, warnings := Load(t.TempDir(), []string{"review_rules.md"})
	if len(rs) == 0 {
		t.Fatal("defaults not loaded")
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want one missing-file note", warnings)
	}
}

func TestLoadRepoLocalRules(t *testing.T) {
	dir := t.TempDir()
	loka := filepath.Join(dir, ".loka")
	if err := os.Mkdir(loka, 0o755); err != nil {
		t.Fatal(err)
	}
	doc := "---\nname: extra-rule\nseverity: info\npattern: TODO\n---\nFlag TODO comments.\n"
	if err := os.WriteFile(filepath.Join(loka, "review_rules.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	rs, warnings := Load(dir, []string{"review_rules.md"})
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	found := false
	for _, r := range rs {
		if r.Name == "extra-rule" {
			found = true
		}
	}
	if !found {
		t.Fatal("repo-local extra-rule not loaded")
	}
}
