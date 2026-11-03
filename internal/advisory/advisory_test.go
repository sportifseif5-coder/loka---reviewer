package advisory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sportifseif5-coder/loka---reviewer/internal/analyzer"
	"github.com/sportifseif5-coder/loka---reviewer/internal/vcs"
)

func TestBundledDBMatchesVulnerableGoModule(t *testing.T) {
	s := Scanner{DB: Bundled()}
	unit := analyzer.AnalysisUnit{
		Changed: []vcs.ChangedFile{{
			Path: "go.mod",
			Added: []vcs.AddedLine{{
				Number: 8,
				Text:   "\tgithub.com/loka-fixture/vulnerable v0.1.0",
			}},
		}},
	}
	findings, err := s.Analyze(context.Background(), unit)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(findings), findings)
	}
	f := findings[0]
	if f.RuleID != "advisory.GO-2026-0001" {
		t.Errorf("rule = %q", f.RuleID)
	}
	if f.Location.File != "go.mod" || f.Location.LineStart != 8 {
		t.Errorf("location = %+v", f.Location)
	}
	if f.Message == "" || f.Location.File == "" {
		t.Fatal("finding missing location or message (I8)")
	}
}

func TestBundledDBIgnoresPatchedVersion(t *testing.T) {
	s := Scanner{DB: Bundled()}
	unit := analyzer.AnalysisUnit{
		Changed: []vcs.ChangedFile{{
			Path: "go.mod",
			Added: []vcs.AddedLine{{
				Number: 8,
				Text:   "\tgithub.com/loka-fixture/vulnerable v1.2.3",
			}},
		}},
	}
	findings, err := s.Analyze(context.Background(), unit)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("patched version must not match: %+v", findings)
	}
}

func TestUpdateFromLocalFileDoesNotOpenNetwork(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "next.json")
	dst := filepath.Join(dir, "advisories.json")
	body := `{
  "updated_at": "2026-11-02T00:00:00Z",
  "advisories": [
    {"id":"TEST-1","package":"demo/pkg","ecosystem":"go","below":"2.0.0","severity":"high","summary":"test"}
  ]
}`
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Update(src, dst); err != nil {
		t.Fatalf("Update: %v", err)
	}
	db, err := Load(dst)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(db.Advisories) != 1 || db.Advisories[0].ID != "TEST-1" {
		t.Fatalf("loaded %+v", db)
	}
}

func TestUpdateRejectsRemoteURL(t *testing.T) {
	err := Update("https://example.com/advisories.json", t.TempDir()+"/out.json")
	if err == nil {
		t.Fatal("update from URL must be rejected (no egress)")
	}
	if !strings.Contains(err.Error(), "local") {
		t.Errorf("error should mention local-only: %v", err)
	}
}
