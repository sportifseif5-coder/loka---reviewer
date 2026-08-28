//go:build desktop

package main

import (
	"context"

	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/version"
)

// App is the desktop workbench backend. Methods on App are bound to the
// frontend through Wails and are the future seam into the review engine.
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

// Review is the Phase 0 stub binding: it validates the requested path and
// returns an empty review result so the workbench can render end to end.
// Engine wiring (config, storage, pipeline) lands with Phase 1.
func (a *App) Review(repoPath string) (*model.ReviewResult, error) {
	if repoPath == "" {
		return nil, model.ErrEmptyRepoPath
	}
	return &model.ReviewResult{
		ID:           "stub",
		RepoPath:     repoPath,
		Mode:         "offline",
		Findings:     []model.Finding{},
		AnalyzersRun: []string{},
	}, nil
}
