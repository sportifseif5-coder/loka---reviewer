// Package review implements the core review engine: it orchestrates the
// deterministic analyzer pipeline, rules, and (later) the LLM/agent layers,
// and persists results. In Phase 0 the pipeline is intentionally empty; the
// flow must produce a valid, persisted zero-finding review.
package review

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/sportifseif5-coder/loka---reviewer/internal/config"
	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/store"
)

// AnalysisUnit is the input handed to an analyzer (architecture section 6.1).
type AnalysisUnit struct {
	RepoPath string
	Files    []string
}

// Analyzer is the deterministic analyzer interface. Implementations are
// registered in the Engine; Phase 0 ships none.
type Analyzer interface {
	Name() string
	Analyze(ctx context.Context, unit AnalysisUnit) ([]model.Finding, error)
}

// Engine runs review lifecycle end to end.
type Engine struct {
	cfg       *config.Config
	store     *store.Store
	analyzers []Analyzer
}

// NewEngine builds an engine from configuration and an optional store.
func NewEngine(cfg *config.Config, s *store.Store) *Engine {
	return &Engine{cfg: cfg, store: s}
}

// RegisterAnalyzer adds a deterministic analyzer to the pipeline.
func (e *Engine) RegisterAnalyzer(a Analyzer) {
	e.analyzers = append(e.analyzers, a)
}

// Review runs one review against a repository and persists the result.
func (e *Engine) Review(ctx context.Context, req model.ReviewRequest) (*model.ReviewResult, error) {
	if err := validateRepo(req.RepoPath); err != nil {
		return nil, err
	}

	started := time.Now()
	res := &model.ReviewResult{
		ID:           newID(),
		RepoPath:     req.RepoPath,
		Mode:         string(e.cfg.Mode),
		StartedAt:    started,
		Findings:     []model.Finding{},
		AnalyzersRun: []string{},
	}

	// Deterministic pipeline: analyzers run first; rules and LLM layers are
	// added in later phases. The baseline is always produced.
	for _, a := range e.analyzers {
		unit := AnalysisUnit{RepoPath: req.RepoPath}
		findings, err := a.Analyze(ctx, unit)
		if err != nil {
			res.Degradations = append(res.Degradations,
				fmt.Sprintf("analyzer %s failed: %v", a.Name(), err))
			continue
		}
		res.AnalyzersRun = append(res.AnalyzersRun, a.Name())
		res.Findings = append(res.Findings, findings...)
	}

	res.FinishedAt = time.Now()

	if e.store != nil {
		if err := e.store.SaveReview(ctx, res); err != nil {
			return nil, fmt.Errorf("persist review: %w", err)
		}
	}
	return res, nil
}

// validateRepo checks that the target exists and is a directory. Git-aware
// validation arrives with the VCS adapter in Phase 1.
func validateRepo(path string) error {
	if path == "" {
		return model.ErrEmptyRepoPath
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("repo path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("repo path %s is not a directory", path)
	}
	return nil
}

// newID returns a short random hex identifier.
func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
