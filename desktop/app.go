//go:build desktop

package main

import (
	"context"
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
// frontend through Wails and call into the review engine.
type App struct {
	ctx context.Context
}

// NewApp constructs the workbench backend.
func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// Version exposes the build version to the frontend.
func (a *App) Version() string {
	return version.String()
}

// Review runs the deterministic pipeline against a repository and returns
// the review result.
func (a *App) Review(repoPath string) (*model.ReviewResult, error) {
	if repoPath == "" {
		return nil, model.ErrEmptyRepoPath
	}
	cfg, _, err := config.LoadForRepo(repoPath, "")
	if err != nil {
		return nil, err
	}
	db := defaultDBPath()
	s, err := store.Open(db)
	if err != nil {
		return nil, err
	}
	defer s.Close()

	eng := review.NewEngine(cfg, s)
	eng.RegisterVCS(vcs.NewGit())
	eng.RegisterAnalyzer(analyzer.SecretDetector{})
	eng.RegisterAnalyzer(analyzer.GoVetBridge{})
	rs, _ := loadRules(repoPath, cfg.Rules.Include)
	eng.RegisterRules(rs)

	return eng.Review(context.Background(), model.ReviewRequest{
		RepoPath: repoPath,
		Mode:     string(cfg.Mode),
	})
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
