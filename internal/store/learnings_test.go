package store

import (
	"context"
	"testing"
	"time"

	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
)

func TestRecordFeedbackAndListLearnings(t *testing.T) {
	s, err := Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	if err := s.RecordFeedback(ctx, model.FeedbackEvent{
		RepoPath:  "/tmp/repo",
		FindingID: "f1",
		Category:  "debug-print-in-go",
		Action:    model.FeedbackDismiss,
		At:        time.Now().UTC(),
	}); err != nil {
		t.Fatalf("RecordFeedback: %v", err)
	}

	events, err := s.ListFeedback(ctx, "/tmp/repo")
	if err != nil {
		t.Fatalf("ListFeedback: %v", err)
	}
	if len(events) != 1 || events[0].Action != model.FeedbackDismiss {
		t.Fatalf("events = %+v", events)
	}
}

func TestDistillDismissalsIntoLearning(t *testing.T) {
	s, err := Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		if err := s.RecordFeedback(ctx, model.FeedbackEvent{
			RepoPath: "/tmp/repo",
			Category: "debug-print-in-go",
			Action:   model.FeedbackDismiss,
			At:       now,
		}); err != nil {
			t.Fatal(err)
		}
	}

	n, err := s.Distill(ctx, "/tmp/repo")
	if err != nil {
		t.Fatalf("Distill: %v", err)
	}
	if n != 1 {
		t.Fatalf("distilled = %d, want 1", n)
	}
	ls, err := s.ListLearnings(ctx, "/tmp/repo")
	if err != nil {
		t.Fatalf("ListLearnings: %v", err)
	}
	if len(ls) != 1 {
		t.Fatalf("learnings = %+v", ls)
	}
	if ls[0].Category != "debug-print-in-go" {
		t.Errorf("category = %q", ls[0].Category)
	}
	if ls[0].Weight < 3 {
		t.Errorf("weight = %v, want >= 3", ls[0].Weight)
	}
}
