// Command loka is the local-first AI code reviewer. Phase 0 ships the
// "review" subcommand implementing the empty-review flow and "version".
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sportifseif5-coder/loka---reviewer/internal/analyzer"
	"github.com/sportifseif5-coder/loka---reviewer/internal/config"
	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/provider"
	"github.com/sportifseif5-coder/loka---reviewer/internal/review"
	"github.com/sportifseif5-coder/loka---reviewer/internal/rules"
	"github.com/sportifseif5-coder/loka---reviewer/internal/store"
	"github.com/sportifseif5-coder/loka---reviewer/internal/vcs"
	"github.com/sportifseif5-coder/loka---reviewer/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "review":
		runReview(os.Args[2:])
	case "version":
		fmt.Println(version.String())
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `loka - local-first AI code reviewer

Usage:
  loka review [-repo PATH] [-config PATH] [-db PATH]
  loka version
`)
}

func runReview(args []string) {
	fs := flag.NewFlagSet("review", flag.ExitOnError)
	repoPath := fs.String("repo", "", "path to the repository to review")
	configPath := fs.String("config", "", "path to an explicit config file")
	dbPath := fs.String("db", "", "path to the loka database (defaults to the user data dir)")
	jsonOut := fs.Bool("json", false, "emit the full review result as JSON")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	cfg, warnings, err := config.LoadForRepo(*repoPath, *configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}

	db := *dbPath
	if db == "" {
		db = defaultDBPath()
	}
	s, err := store.Open(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "store error: %v\n", err)
		os.Exit(1)
	}
	defer s.Close()

	eng := review.NewEngine(cfg, s)
	eng.RegisterVCS(vcs.NewGit())
	eng.RegisterAnalyzer(analyzer.SecretDetector{})
	eng.RegisterAnalyzer(analyzer.GoVetBridge{})

	rs, warnings := loadRules(*repoPath, cfg.Rules.Include)
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}
	eng.RegisterRules(rs)

	// Provider routing is additive: a misconfigured provider degrades to the
	// deterministic baseline instead of aborting the review.
	router, err := provider.Resolve(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v (LLM stage disabled)\n", err)
	} else if router.Len() > 0 {
		eng.RegisterProviderRouter(router)
	}

	res, err := eng.Review(context.Background(), model.ReviewRequest{
		RepoPath: *repoPath,
		Mode:     string(cfg.Mode),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "review error: %v\n", err)
		os.Exit(1)
	}

	if *jsonOut {
		out, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshal error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(out))
		return
	}

	fmt.Printf("review %s complete: %d findings (mode %s, %d analyzers)\n",
		res.ID, len(res.Findings), res.Mode, len(res.AnalyzersRun))
	for _, d := range res.Degradations {
		fmt.Fprintf(os.Stderr, "degradation: %s\n", d)
	}
}

// defaultDBPath resolves the per-user data directory.
func defaultDBPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "loka", "loka.db")
	}
	return filepath.Join(dir, "loka", "loka.db")
}

// loadRules compiles the built-in defaults plus any rule files named in
// rules.include. Include entries are resolved against the repository-local
// .loka/ dir, the repository root, and the user config dir. Missing files
// produce warnings, never errors.
func loadRules(repoPath string, include []string) ([]*rules.Rule, []string) {
	rs := rules.Defaults()
	var warnings []string

	candidates := func(rel string) []string {
		var out []string
		if repoPath != "" {
			out = append(out, filepath.Join(repoPath, ".loka", rel))
			out = append(out, filepath.Join(repoPath, rel))
		}
		if dir, err := os.UserConfigDir(); err == nil {
			out = append(out, filepath.Join(dir, "loka", rel))
		}
		return out
	}

	for _, rel := range include {
		loaded := false
		for _, p := range candidates(rel) {
			data, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			parsed, err := rules.ParseAll(data, p)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("rules file %s: %v", p, err))
				continue
			}
			rs = append(rs, parsed...)
			loaded = true
			break
		}
		if !loaded {
			warnings = append(warnings, fmt.Sprintf("rules file %q not found, skipped", rel))
		}
	}
	return rs, warnings
}
