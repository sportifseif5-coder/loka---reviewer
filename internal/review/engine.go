// Package review implements the core review engine: it orchestrates the
// deterministic analyzer pipeline and the rules engine, merges and ranks the
// baseline, and persists results (architecture sections 4 and 6). In offline
// mode no network path is ever opened.
package review

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/sportifseif5-coder/loka---reviewer/internal/agent"
	"github.com/sportifseif5-coder/loka---reviewer/internal/analyzer"
	"github.com/sportifseif5-coder/loka---reviewer/internal/config"
	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/provider"
	"github.com/sportifseif5-coder/loka---reviewer/internal/rules"
	"github.com/sportifseif5-coder/loka---reviewer/internal/store"
	"github.com/sportifseif5-coder/loka---reviewer/internal/vcs"
)

// Engine runs review lifecycle end to end.
type Engine struct {
	cfg       *config.Config
	store     *store.Store
	vcs       vcs.VCSAdapter
	analyzers []analyzer.Analyzer
	rules     []*rules.Rule
	router    *provider.Router
}

// NewEngine builds an engine from configuration and an optional store.
func NewEngine(cfg *config.Config, s *store.Store) *Engine {
	return &Engine{cfg: cfg, store: s}
}

// RegisterVCS installs the read-only VCS adapter.
func (e *Engine) RegisterVCS(v vcs.VCSAdapter) {
	e.vcs = v
}

// RegisterAnalyzer adds a deterministic analyzer to the pipeline.
func (e *Engine) RegisterAnalyzer(a analyzer.Analyzer) {
	e.analyzers = append(e.analyzers, a)
}

// RegisterRules adds rules to the evaluation set.
func (e *Engine) RegisterRules(rs []*rules.Rule) {
	e.rules = append(e.rules, rs...)
}

// RegisterProviderRouter installs the LLM routing set. Without a router the
// LLM stage is skipped and only the deterministic baseline is produced.
func (e *Engine) RegisterProviderRouter(r *provider.Router) {
	e.router = r
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

	// Working-tree diff is the deterministic review input.
	changed := e.collectDiff(ctx, req.RepoPath, res)
	unit := analyzer.AnalysisUnit{RepoPath: req.RepoPath, Changed: changed}

	// Deterministic analyzers run first and always; the baseline must be
	// produced even if the LLM/agent layer is unavailable (invariants I2/I3).
	baseline := make([]model.Finding, 0, 16)
	for _, a := range e.analyzers {
		findings, err := a.Analyze(ctx, unit)
		if err != nil {
			res.Degradations = append(res.Degradations,
				fmt.Sprintf("analyzer %s failed: %v", a.Name(), err))
			continue
		}
		res.AnalyzersRun = append(res.AnalyzersRun, a.Name())
		baseline = append(baseline, findings...)
	}

	if len(e.rules) > 0 {
		res.AnalyzersRun = append(res.AnalyzersRun, "rules")
		baseline = append(baseline, rules.Evaluate(changed, e.rules)...)
	}

	// The LLM stage is additive: on any failure it degrades to a note and the
	// deterministic baseline still ships (ADR-0003 routing policy, I2/I3).
	res.Findings = baseline
	if llm := e.llmStage(ctx, unit, baseline, res); len(llm) > 0 {
		res.Findings = append(res.Findings, llm...)
	}

	res.Findings = dedupe(res.Findings)
	rank(res.Findings)
	for i := range res.Findings {
		if res.Findings[i].ID == "" {
			res.Findings[i].ID = newID()
		}
	}
	res.FinishedAt = time.Now()

	if e.store != nil {
		if err := e.store.SaveReview(ctx, res); err != nil {
			return nil, fmt.Errorf("persist review: %w", err)
		}
	}
	return res, nil
}

// llmStage runs the agent-layer review pass over the changed files and the
// deterministic baseline. In offline mode only local providers are eligible;
// remote providers require the per-repository consent implied by a non-offline
// mode (invariant I6). Every failure path degrades to a note and returns no
// findings; the baseline is always delivered.
func (e *Engine) llmStage(ctx context.Context, unit analyzer.AnalysisUnit, baseline []model.Finding, res *model.ReviewResult) []model.Finding {
	if e.router == nil {
		return nil
	}

	localOnly := e.cfg.Mode == config.ModeOffline
	ar, err := agent.NewReviewAgent().Run(ctx, e.router, localOnly, agent.Request{
		RepoPath: unit.RepoPath,
		Changed:  unit.Changed,
		Baseline: baseline,
		Budget:   e.cfg.Budgets.ReviewTokens,
	})
	if err != nil {
		res.Degradations = append(res.Degradations,
			fmt.Sprintf("LLM stage skipped: %v", err))
		return nil
	}
	if ar.Invalid > 0 {
		res.Degradations = append(res.Degradations,
			fmt.Sprintf("LLM stage dropped %d finding(s) missing location or message", ar.Invalid))
	}
	if ar.Demoted > 0 {
		res.Degradations = append(res.Degradations,
			fmt.Sprintf("LLM stage demoted %d finding(s) not on added lines", ar.Demoted))
	}
	res.AnalyzersRun = append(res.AnalyzersRun, "llm:"+ar.Model)
	return ar.Findings
}

// collectDiff populates the changed-file slice from the VCS adapter,
// degrading to a warning when the target is not a git work tree.
func (e *Engine) collectDiff(ctx context.Context, repoPath string, res *model.ReviewResult) []vcs.ChangedFile {
	if e.vcs == nil {
		return nil
	}
	diff, err := e.vcs.WorkingDiff(ctx, repoPath)
	if err != nil {
		if err == vcs.ErrNotRepository {
			res.Degradations = append(res.Degradations,
				"repository has no git work tree; reviewing without a diff")
		} else {
			res.Degradations = append(res.Degradations,
				fmt.Sprintf("reading working diff failed: %v", err))
		}
		return nil
	}
	return vcs.ParseChangedFiles(diff.Files)
}

// dedupe merges findings that share a location and rule identity. The
// deterministic baseline is ground truth, so identical duplicates collapse
// to the first occurrence.
func dedupe(findings []model.Finding) []model.Finding {
	seen := map[string]bool{}
	out := make([]model.Finding, 0, len(findings))
	for _, f := range findings {
		key := f.Location.File + ":" + itoa(f.Location.LineStart) + ":" + f.RuleID
		if f.RuleID == "" {
			key += ":" + f.Category
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, f)
	}
	return out
}

var severityWeight = map[model.Severity]int{
	model.SeverityInfo:     1,
	model.SeverityWarning:  2,
	model.SeverityError:    3,
	model.SeverityCritical: 4,
}

// rank orders findings by severity, then confidence, then location.
func rank(findings []model.Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		wi, wj := severityWeight[findings[i].Severity], severityWeight[findings[j].Severity]
		if wi != wj {
			return wi > wj
		}
		if findings[i].Confidence != findings[j].Confidence {
			return findings[i].Confidence > findings[j].Confidence
		}
		if findings[i].Location.File != findings[j].Location.File {
			return findings[i].Location.File < findings[j].Location.File
		}
		return findings[i].Location.LineStart < findings[j].Location.LineStart
	})
}

// validateRepo checks that the target exists and is a directory.
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

func itoa(n int) string {
	if n <= 0 {
		return "0"
	}
	return fmt.Sprintf("%d", n)
}
