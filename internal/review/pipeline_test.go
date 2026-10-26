package review

import (
	"context"
	"testing"

	"github.com/sportifseif5-coder/loka---reviewer/internal/config"
	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/store"
)

func TestNewPipelineWiresLLMRouter(t *testing.T) {
	cfg := config.Default()
	s, err := store.Open("")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	pipe := NewPipeline(cfg, s, t.TempDir())
	if pipe.Engine == nil {
		t.Fatal("engine is nil")
	}
	if pipe.Engine.router == nil {
		t.Fatal("provider router not registered; LLM stage would be skipped even with a local model")
	}
	if pipe.Engine.router.Len() == 0 {
		t.Fatal("provider router is empty")
	}
	if pipe.Engine.vcs == nil {
		t.Fatal("VCS adapter not registered")
	}
	if len(pipe.Engine.analyzers) < 2 {
		t.Fatalf("analyzers = %d, want secret-detection and go-vet", len(pipe.Engine.analyzers))
	}
	if len(pipe.Engine.rules) == 0 {
		t.Fatal("default rules not registered")
	}
}

func TestNewPipelineRunsFixtureRepo(t *testing.T) {
	repo := fixtureRepo(t)
	cfg := config.Default()
	s, err := store.Open("")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	pipe := NewPipeline(cfg, s, repo)
	res, err := pipe.Engine.Review(context.Background(), model.ReviewRequest{RepoPath: repo})
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if len(res.Findings) == 0 {
		t.Fatal("expected baseline findings on the fixture repo")
	}
	if res.Summary.Risk == "" {
		t.Fatal("summary risk is empty")
	}
}

func TestEngineHasNoWorkingTreeMutators(t *testing.T) {
	// Invariant I7: the engine never writes the working tree. The VCS
	// adapter surface is read-only (IsRepo, WorkingDiff, Blame, ReadAtHEAD). Apply of
	// suggested fixes is deferred until Phase 2; this package must not grow
	// an apply/merge/push helper in the meantime.
	var e Engine
	_ = e
	type forbidden interface {
		Apply(context.Context, string) error
		Merge(context.Context, string) error
		Push(context.Context, string) error
	}
	if _, ok := any(&e).(forbidden); ok {
		t.Fatal("engine implements a working-tree mutator (I7)")
	}
}
