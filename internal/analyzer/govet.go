package analyzer

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/vcs"
)

// GoVetBridge runs the system go vet on packages that contain changed Go
// files and turns diagnostics into findings. It is deterministic and
// offline: go vet needs no network for standard-library packages.
type GoVetBridge struct {
	// GoBin overrides the go executable; empty means "go" on PATH.
	GoBin string
}

// Name returns the analyzer name.
func (GoVetBridge) Name() string { return "go-vet" }

// Analyze runs go vet over directories containing changed .go files.
// If no Go files changed it produces nothing; if go is unavailable or the
// target is not a Go module it returns an error so the engine records a
// degradation and the baseline still ships.
func (g GoVetBridge) Analyze(ctx context.Context, unit AnalysisUnit) ([]model.Finding, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	dirs := changedGoDirs(unit.Changed)
	if len(dirs) == 0 {
		return nil, nil
	}

	goBin := g.GoBin
	if goBin == "" {
		goBin = "go"
	}

	var findings []model.Finding
	for _, d := range dirs {
		out, err := g.vet(ctx, goBin, unit.RepoPath, d)
		parsed := parseVetOutput(string(out), unit.RepoPath)
		if err != nil {
			// Non-zero exit only counts as a successful run if it actually
			// produced parseable diagnostics; anything else is a real failure
			// (missing module, missing go, ...) and is recorded as a
			// degradation so the baseline still ships.
			if len(parsed) == 0 {
				return nil, fmt.Errorf("go vet %s: %w", d, err)
			}
		}
		findings = append(findings, parsed...)
	}
	return findings, nil
}

func (g GoVetBridge) vet(ctx context.Context, goBin, repoPath, dir string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, goBin, "vet", dir)
	cmd.Dir = repoPath
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		// Diagnostics may land on stderr or stdout; return whichever has
		// content alongside the error so the caller can decide.
		if out.Len() > 0 {
			return out.Bytes(), err
		}
		return errb.Bytes(), err
	}
	return out.Bytes(), nil
}

// changedGoDirs returns the unique directories holding changed .go files.
func changedGoDirs(changed []vcs.ChangedFile) []string {
	set := map[string]bool{}
	for _, cf := range changed {
		if strings.HasSuffix(cf.Path, ".go") {
			set[filepath.Dir(cf.Path)] = true
		}
	}
	dirs := make([]string, 0, len(set))
	for d := range set {
		dirs = append(dirs, d)
	}
	return dirs
}

// parseVetOutput turns `path/file.go:LINE:COL: message` lines into findings.
func parseVetOutput(out, repoPath string) []model.Finding {
	var findings []model.Finding
	for _, ln := range strings.Split(out, "\n") {
		file, line, msg, ok := splitDiagnostic(ln)
		if !ok {
			continue
		}
		rel := strings.TrimPrefix(file, repoPath+string(filepath.Separator))
		rel = strings.TrimPrefix(rel, "./")
		if rel == "" {
			rel = file
		}
		findings = append(findings, model.Finding{
			RuleID:     "go-vet",
			Category:   "lint",
			Severity:   model.SeverityWarning,
			Source:     model.SourceAnalyzer,
			Location:   model.Location{File: rel, LineStart: line, LineEnd: line},
			Message:    msg,
			Reasoning:  "reported by go vet (offline lint bridge)",
			Confidence: 1.0,
		})
	}
	return findings
}

// splitDiagnostic parses `file:line:col: message`, tolerating a missing col.
func splitDiagnostic(ln string) (file string, line int, msg string, ok bool) {
	ln = strings.TrimSpace(ln)
	if ln == "" {
		return "", 0, "", false
	}
	first := strings.Index(ln, ":")
	if first <= 0 {
		return "", 0, "", false
	}
	file = ln[:first]
	rest := ln[first+1:]
	second := strings.Index(rest, ":")
	if second <= 0 {
		return "", 0, "", false
	}
	lineStr := rest[:second]
	n, err := strconv.Atoi(lineStr)
	if err != nil {
		return "", 0, "", false
	}
	msgPart := rest[second+1:]
	// Strip the column if present (file:line:col: message).
	if idx := strings.Index(msgPart, ": "); idx > 0 {
		if _, err := strconv.Atoi(msgPart[:idx]); err == nil {
			msgPart = msgPart[idx+2:]
		}
	}
	return file, n, msgPart, true
}
