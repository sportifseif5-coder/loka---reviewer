//go:build desktop

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sportifseif5-coder/loka---reviewer/internal/analyzer"
	"github.com/sportifseif5-coder/loka---reviewer/internal/config"
	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/review"
	"github.com/sportifseif5-coder/loka---reviewer/internal/rules"
	"github.com/sportifseif5-coder/loka---reviewer/internal/store"
	"github.com/sportifseif5-coder/loka---reviewer/internal/vcs"
	"github.com/sportifseif5-coder/loka---reviewer/internal/version"
)

// App is the desktop workbench backend. Methods on App are bound to the
// frontend through Wails and call into the review engine and the local store.
// The store holds history and index data in the per-user data directory
// (invariant I5); nothing here writes to the user's working tree.
type App struct {
	ctx   context.Context
	db    *store.Store
	dbErr error
}

// NewApp constructs the workbench backend. The store is opened in startup so
// a data-directory failure surfaces as an error on the first bound call
// rather than preventing the window from appearing.
func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.db, a.dbErr = store.Open(defaultDBPath())
}

func (a *App) shutdown(context.Context) {
	if a.db != nil {
		a.db.Close()
	}
}

// baseCtx returns the Wails lifecycle context, falling back to a background
// context so bound methods stay usable before startup in tests.
func (a *App) baseCtx() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

// ready returns the open store, surfacing the startup error if it failed.
func (a *App) ready() (*store.Store, error) {
	if a.db == nil {
		if a.dbErr != nil {
			return nil, a.dbErr
		}
		return nil, fmt.Errorf("store is not open")
	}
	return a.db, nil
}

// Version exposes the build version to the frontend.
func (a *App) Version() string {
	return version.String()
}

// Review runs the deterministic pipeline against a repository and returns
// the review result. The result is persisted by the engine, so it appears in
// the history surfaced by ListRepos/ListReviews.
func (a *App) Review(repoPath string) (*model.ReviewResult, error) {
	if repoPath == "" {
		return nil, model.ErrEmptyRepoPath
	}
	s, err := a.ready()
	if err != nil {
		return nil, err
	}
	cfg, _, err := config.LoadForRepo(repoPath, "")
	if err != nil {
		return nil, err
	}

	eng := review.NewEngine(cfg, s)
	eng.RegisterVCS(vcs.NewGit())
	eng.RegisterAnalyzer(analyzer.SecretDetector{})
	eng.RegisterAnalyzer(analyzer.GoVetBridge{})
	rs, _ := loadRules(repoPath, cfg.Rules.Include)
	eng.RegisterRules(rs)

	return eng.Review(a.baseCtx(), model.ReviewRequest{
		RepoPath: repoPath,
		Mode:     string(cfg.Mode),
	})
}

// ListRepos returns the repositories with stored reviews, newest first, for
// the workbench repo picker and history sidebar.
func (a *App) ListRepos() ([]store.RepoSummary, error) {
	s, err := a.ready()
	if err != nil {
		return nil, err
	}
	return s.ListRepos(a.baseCtx())
}

// ListReviews returns the review history for one repository, newest first.
func (a *App) ListReviews(repoPath string) ([]store.ReviewSummary, error) {
	if repoPath == "" {
		return nil, model.ErrEmptyRepoPath
	}
	s, err := a.ready()
	if err != nil {
		return nil, err
	}
	return s.ListReviews(a.baseCtx(), repoPath)
}

// GetReview loads a stored review and its findings by ID.
func (a *App) GetReview(id string) (*model.ReviewResult, error) {
	if id == "" {
		return nil, fmt.Errorf("review id must not be empty")
	}
	s, err := a.ready()
	if err != nil {
		return nil, err
	}
	return s.GetReview(a.baseCtx(), id)
}

// ReviewContext returns the working-tree lines around a finding location for
// the inline diff view. It is read-only (invariant I7).
func (a *App) ReviewContext(repoPath, file string, lineStart, lineEnd, radius int) ([]review.CodeLine, error) {
	return review.FileWindow(repoPath, file, model.Location{
		File:      file,
		LineStart: lineStart,
		LineEnd:   lineEnd,
	}, radius)
}

// RepoMode reports the effective execution mode for a repository so the
// workbench can show it at all times (constitution section 8).
func (a *App) RepoMode(repoPath string) (string, error) {
	cfg, _, err := config.LoadForRepo(repoPath, "")
	if err != nil {
		return "", err
	}
	return string(cfg.Mode), nil
}

// loadRules compiles the built-in defaults plus any rule files named in
// rules.include, resolving entries against the repo and user config dirs.
func loadRules(repoPath string, include []string) ([]*rules.Rule, []string) {
	rs := rules.Defaults()
	var warnings []string
	for _, rel := range include {
		candidates := []string{}
		if repoPath != "" {
			candidates = append(candidates, filepath.Join(repoPath, ".loka", rel))
			candidates = append(candidates, filepath.Join(repoPath, rel))
		}
		if dir, err := os.UserConfigDir(); err == nil {
			candidates = append(candidates, filepath.Join(dir, "loka", rel))
		}
		loaded := false
		for _, p := range candidates {
			data, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			parsed, err := rules.ParseAll(data, p)
			if err != nil {
				warnings = append(warnings, "rules file "+p+": "+err.Error())
				continue
			}
			rs = append(rs, parsed...)
			loaded = true
			break
		}
		if !loaded {
			warnings = append(warnings, "rules file \""+rel+"\" not found, skipped")
		}
	}
	return rs, warnings
}

// defaultDBPath resolves the per-user data directory.
func defaultDBPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "loka", "loka.db")
	}
	return filepath.Join(dir, "loka", "loka.db")
}
