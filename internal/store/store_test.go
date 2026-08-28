package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
)

func TestOpenInMemory(t *testing.T) {
	s, err := Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
}

func TestMigrateCreatesSchema(t *testing.T) {
	s, err := Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	rows, err := s.db.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatalf("query migrations: %v", err)
	}
	defer rows.Close()
	var versions []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		versions = append(versions, v)
	}
	if len(versions) != len(migrations) {
		t.Fatalf("applied %d migrations, want %d", len(versions), len(migrations))
	}
}

func TestSaveAndGetReview(t *testing.T) {
	s, err := Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	want := &model.ReviewResult{
		ID:        "r1",
		RepoPath:  "/tmp/repo",
		Mode:      "offline",
		StartedAt: time.Now().UTC().Add(-time.Minute),
		Findings: []model.Finding{
			{
				ID:         "f1",
				RuleID:     "example-rule",
				Category:   "correctness",
				Severity:   model.SeverityError,
				Source:     model.SourceAnalyzer,
				Location:   model.Location{File: "a.go", LineStart: 3, LineEnd: 3},
				Message:    "something is wrong",
				Reasoning:  "because",
				Confidence: 1.0,
			},
		},
		AnalyzersRun: []string{"example"},
	}
	want.FinishedAt = time.Now().UTC()

	if err := s.SaveReview(ctx, want); err != nil {
		t.Fatalf("SaveReview: %v", err)
	}
	got, err := s.GetReview(ctx, "r1")
	if err != nil {
		t.Fatalf("GetReview: %v", err)
	}
	if got.ID != want.ID || got.RepoPath != want.RepoPath || got.Mode != want.Mode {
		t.Errorf("round trip header mismatch: %+v", got)
	}
	if len(got.Findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(got.Findings))
	}
	f := got.Findings[0]
	if f.ID != "f1" || f.Severity != model.SeverityError || f.Source != model.SourceAnalyzer {
		t.Errorf("finding mismatch: %+v", f)
	}
	if f.Location.File != "a.go" || f.Location.LineStart != 3 {
		t.Errorf("location mismatch: %+v", f.Location)
	}
	if got.StartedAt.IsZero() || got.FinishedAt.IsZero() {
		t.Error("timestamps not persisted")
	}
}

func TestGetReviewNotFound(t *testing.T) {
	s, err := Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	if _, err := s.GetReview(context.Background(), "nope"); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestOpenCreatesDataDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "loka.db")
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	if _, err := s.db.Exec("SELECT 1"); err != nil {
		t.Fatalf("query after open: %v", err)
	}
}
