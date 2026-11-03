package review

import (
	"fmt"

	"github.com/sportifseif5-coder/loka---reviewer/internal/advisory"
	"github.com/sportifseif5-coder/loka---reviewer/internal/analyzer"
	"github.com/sportifseif5-coder/loka---reviewer/internal/config"
	"github.com/sportifseif5-coder/loka---reviewer/internal/provider"
	"github.com/sportifseif5-coder/loka---reviewer/internal/rules"
	"github.com/sportifseif5-coder/loka---reviewer/internal/store"
	"github.com/sportifseif5-coder/loka---reviewer/internal/vcs"
)

// Pipeline is a fully wired review engine plus any non-fatal setup notes.
// CLI and desktop share this so the local LLM path (Ollama / llama.cpp) is
// available on both surfaces (Phase 1.9 / invariant I2).
type Pipeline struct {
	Engine   *Engine
	Warnings []string
}

// NewPipeline builds the default pipeline: git VCS, secret detection, go vet,
// language lints, the local advisory DB, default + included rules, and the
// configured provider router. Provider misconfiguration degrades to the
// deterministic baseline instead of aborting (ADR-0003).
func NewPipeline(cfg *config.Config, s *store.Store, repoPath string) *Pipeline {
	eng := NewEngine(cfg, s)
	eng.RegisterVCS(vcs.NewGit())
	eng.RegisterAnalyzer(analyzer.SecretDetector{})
	eng.RegisterAnalyzer(analyzer.GoVetBridge{})
	eng.RegisterAnalyzer(analyzer.PythonLint{})
	eng.RegisterAnalyzer(analyzer.TypeScriptLint{})
	eng.RegisterAnalyzer(advisory.Scanner{DB: advisory.Bundled()})

	rs, warnings := rules.Load(repoPath, cfg.Rules.Include)
	eng.RegisterRules(rs)

	router, err := provider.Resolve(cfg)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("%v (LLM stage disabled)", err))
	} else if router.Len() > 0 {
		eng.RegisterProviderRouter(router)
	}

	return &Pipeline{Engine: eng, Warnings: warnings}
}
