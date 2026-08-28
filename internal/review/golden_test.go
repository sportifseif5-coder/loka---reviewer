package review

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/sportifseif5-coder/loka---reviewer/internal/config"
	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/store"
)

var updateGolden = flag.Bool("update", false, "update golden files")

// normalizedSnapshot is the deterministic projection of a review result used
// for golden comparisons. IDs and timestamps are intentionally excluded.
type normalizedSnapshot struct {
	Repo         string          `json:"repo"`
	Mode         string          `json:"mode"`
	AnalyzersRun []string        `json:"analyzers_run"`
	Findings     []model.Finding `json:"findings"`
}

func TestGoldenEmptyRepo(t *testing.T) {
	repo := filepath.Join("testdata", "repos", "empty")
	cfg := config.Default()

	s, err := store.Open("")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	eng := NewEngine(cfg, s)
	res, err := eng.Review(context.Background(), model.ReviewRequest{
		RepoPath: repo,
		Mode:     string(cfg.Mode),
	})
	if err != nil {
		t.Fatalf("Review: %v", err)
	}

	got := normalizedSnapshot{
		Repo:         filepath.Base(repo),
		Mode:         res.Mode,
		AnalyzersRun: res.AnalyzersRun,
		Findings:     res.Findings,
	}
	gotBytes, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	gotBytes = append(gotBytes, '\n')

	goldenPath := filepath.Join("testdata", "golden", "empty.json")
	if *updateGolden {
		if err := os.WriteFile(goldenPath, gotBytes, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v (run with -update to create it)", err)
	}
	if string(want) != string(gotBytes) {
		t.Errorf("golden mismatch\n--- want ---\n%s\n--- got ---\n%s", want, gotBytes)
	}
}

func TestReviewPersistsToStore(t *testing.T) {
	repo := filepath.Join("testdata", "repos", "empty")
	cfg := config.Default()

	s, err := store.Open("")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	eng := NewEngine(cfg, s)
	res, err := eng.Review(context.Background(), model.ReviewRequest{RepoPath: repo})
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if len(res.Findings) != 0 {
		t.Fatalf("empty repo produced %d findings", len(res.Findings))
	}
	if res.ID == "" {
		t.Fatal("review has no ID")
	}
	loaded, err := s.GetReview(context.Background(), res.ID)
	if err != nil {
		t.Fatalf("GetReview: %v", err)
	}
	if loaded.RepoPath != repo {
		t.Errorf("repo path = %q, want %q", loaded.RepoPath, repo)
	}
}

func TestReviewRejectsBadPath(t *testing.T) {
	cfg := config.Default()
	eng := NewEngine(cfg, nil)
	_, err := eng.Review(context.Background(), model.ReviewRequest{RepoPath: "/does/not/exist"})
	if err == nil {
		t.Fatal("expected error for missing repo path")
	}
	if _, err := eng.Review(context.Background(), model.ReviewRequest{RepoPath: ""}); err == nil {
		t.Fatal("expected error for empty repo path")
	}
}

func TestEngineSkipsFailingAnalyzer(t *testing.T) {
	cfg := config.Default()
	s, err := store.Open("")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	eng := NewEngine(cfg, s)
	eng.RegisterAnalyzer(failAnalyzer{})
	res, err := eng.Review(context.Background(), model.ReviewRequest{
		RepoPath: filepath.Join("testdata", "repos", "empty"),
	})
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if len(res.Degradations) == 0 {
		t.Fatal("expected a degradation for the failing analyzer")
	}
	if len(res.AnalyzersRun) != 0 {
		t.Errorf("failing analyzer still recorded in analyzers_run")
	}
}

type failAnalyzer struct{}

func (failAnalyzer) Name() string { return "fail" }

func (failAnalyzer) Analyze(context.Context, AnalysisUnit) ([]model.Finding, error) {
	return nil, os.ErrClosed
}
