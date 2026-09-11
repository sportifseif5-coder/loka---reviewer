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

func TestListReposAndReviews(t *testing.T) {
	s, err := Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	at := func(hour int) time.Time {
		return time.Date(2026, 9, 6, hour, 0, 0, 0, time.UTC)
	}
	save := func(id, repo string, started time.Time, findings int) {
		t.Helper()
		r := &model.ReviewResult{
			ID:         id,
			RepoPath:   repo,
			Mode:       "offline",
			StartedAt:  started,
			FinishedAt: started.Add(time.Second),
		}
		for i := 0; i < findings; i++ {
			r.Findings = append(r.Findings, model.Finding{
				ID:       id + "-f" + string(rune('a'+i)),
				Category: "correctness",
				Severity: model.SeverityWarning,
				Source:   model.SourceAnalyzer,
				Location: model.Location{File: "a.go", LineStart: 1},
				Message:  "m",
			})
		}
		if err := s.SaveReview(ctx, r); err != nil {
			t.Fatalf("SaveReview %s: %v", id, err)
		}
	}
	save("r1", "/repo/a", at(1), 1)
	save("r2", "/repo/a", at(3), 2)
	save("r3", "/repo/b", at(2), 0)
	// Same second as r3, id tie-break makes r4 the latest for /repo/b.
	save("r4", "/repo/b", at(2), 0)

	repos, err := s.ListRepos(ctx)
	if err != nil {
		t.Fatalf("ListRepos: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("ListRepos = %d rows, want 2: %+v", len(repos), repos)
	}
	if repos[0].RepoPath != "/repo/a" || !repos[0].LastStartedAt.Equal(at(3)) {
		t.Errorf("first repo = %+v, want /repo/a at 03:00", repos[0])
	}
	if repos[0].LastFindings != 2 || repos[0].ReviewCount != 2 {
		t.Errorf("/repo/a counts = findings %d reviews %d, want 2/2", repos[0].LastFindings, repos[0].ReviewCount)
	}
	if repos[1].RepoPath != "/repo/b" || !repos[1].LastStartedAt.Equal(at(2)) {
		t.Errorf("second repo = %+v, want /repo/b at 02:00", repos[1])
	}
	if repos[1].ReviewCount != 2 {
		t.Errorf("/repo/b ReviewCount = %d, want 2", repos[1].ReviewCount)
	}

	reviews, err := s.ListReviews(ctx, "/repo/a")
	if err != nil {
		t.Fatalf("ListReviews: %v", err)
	}
	if len(reviews) != 2 || reviews[0].ID != "r2" || reviews[1].ID != "r1" {
		t.Fatalf("ListReviews(/repo/a) = %+v, want [r2 r1]", reviews)
	}
	if reviews[0].Mode != "offline" || reviews[0].FindingsCount != 2 {
		t.Errorf("review summary = %+v, want offline/2", reviews[0])
	}
	if !reviews[0].FinishedAt.After(reviews[0].StartedAt) {
		t.Errorf("FinishedAt not persisted: %+v", reviews[0])
	}
	// The id tie-break orders r4 before r3 within the same second.
	bReviews, err := s.ListReviews(ctx, "/repo/b")
	if err != nil {
		t.Fatalf("ListReviews(/repo/b): %v", err)
	}
	if len(bReviews) != 2 || bReviews[0].ID != "r4" || bReviews[1].ID != "r3" {
		t.Fatalf("ListReviews(/repo/b) = %+v, want [r4 r3]", bReviews)
	}

	empty, err := s.ListReviews(ctx, "/repo/missing")
	if err != nil {
		t.Fatalf("ListReviews(missing): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("ListReviews(missing) = %+v, want empty", empty)
	}
}
