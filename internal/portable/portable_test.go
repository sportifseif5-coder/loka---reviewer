package portable

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/store"
)

func TestExportImportRoundTrip(t *testing.T) {
	src, err := store.Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	res := &model.ReviewResult{
		ID:       "rev-export",
		RepoPath: "/tmp/demo",
		Mode:     "offline",
		Findings: []model.Finding{{
			ID:       "f1",
			Category: "lint",
			Severity: model.SeverityWarning,
			Source:   model.SourceAnalyzer,
			Location: model.Location{File: "a.go", LineStart: 1, LineEnd: 1},
			Message:  "demo",
		}},
		Summary: model.ReviewSummary{Risk: "low", Walkthrough: "one finding"},
	}
	if err := src.SaveReview(context.Background(), res); err != nil {
		t.Fatal(err)
	}
	if err := src.RecordFeedback(context.Background(), model.FeedbackEvent{
		RepoPath: "/tmp/demo", Category: "lint", Action: model.FeedbackDismiss,
	}); err != nil {
		t.Fatal(err)
	}

	archive := filepath.Join(t.TempDir(), "loka-export")
	if err := Export(context.Background(), src, archive); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if _, err := os.Stat(filepath.Join(archive, "reviews.json")); err != nil {
		t.Fatalf("reviews.json missing: %v", err)
	}

	dst, err := store.Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()
	if err := Import(context.Background(), dst, archive); err != nil {
		t.Fatalf("Import: %v", err)
	}
	got, err := dst.GetReview(context.Background(), "rev-export")
	if err != nil {
		t.Fatalf("GetReview: %v", err)
	}
	if got.Summary.Walkthrough != "one finding" || len(got.Findings) != 1 {
		t.Fatalf("imported %+v", got)
	}
	fb, err := dst.ListFeedback(context.Background(), "/tmp/demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(fb) != 1 {
		t.Fatalf("feedback = %d", len(fb))
	}
}

func TestExportRefusesToWriteOutsideDir(t *testing.T) {
	s, err := store.Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	err = Export(context.Background(), s, "/etc/loka-export")
	if err == nil {
		t.Fatal("export into /etc must fail")
	}
}
