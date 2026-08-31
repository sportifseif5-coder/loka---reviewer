// Package analyzer implements the deterministic analyzer pipeline
// (architecture section 6). Analyzers always run and produce the
// reproducible baseline (invariant I3). They operate on the changed-file
// slice extracted from the working-tree diff, so a review is deterministic
// given the same repository state.
package analyzer

import (
	"context"

	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/vcs"
)

// AnalysisUnit is the input handed to an analyzer: the repository path and
// the files whose added lines are under review.
type AnalysisUnit struct {
	RepoPath string
	// Changed carries per-file added lines extracted from the diff.
	Changed []vcs.ChangedFile
}

// Analyzer is the deterministic analyzer interface. Implementations are
// registered on the engine at startup.
type Analyzer interface {
	Name() string
	Analyze(ctx context.Context, unit AnalysisUnit) ([]model.Finding, error)
}

// AnalyzeFunc adapts a plain function to the Analyzer interface.
type AnalyzeFunc struct {
	name string
	fn   func(ctx context.Context, unit AnalysisUnit) ([]model.Finding, error)
}

// NewAnalyzer wraps a function as an Analyzer.
func NewAnalyzer(name string, fn func(ctx context.Context, unit AnalysisUnit) ([]model.Finding, error)) *AnalyzeFunc {
	return &AnalyzeFunc{name: name, fn: fn}
}

// Name returns the analyzer name.
func (a *AnalyzeFunc) Name() string { return a.name }

// Analyze calls the wrapped function.
func (a *AnalyzeFunc) Analyze(ctx context.Context, unit AnalysisUnit) ([]model.Finding, error) {
	return a.fn(ctx, unit)
}
