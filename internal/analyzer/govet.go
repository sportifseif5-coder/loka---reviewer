package analyzer

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	var firstErr error
	attempted, failed := 0, 0
	for _, d := range dirs {
		out, err := g.vet(ctx, goBin, unit.RepoPath, vetTarget(d))
		parsed := parseVetOutput(string(out), unit.RepoPath)
		if err != nil && len(parsed) == 0 {
			// Packages excluded by build tags (e.g. //go:build desktop) are
			// not a review failure; skip them so a tagged-out directory does
			// not degrade the rest of the lint bridge.
			if excludedByBuildTags(out, err) {
				continue
			}
			attempted++
			failed++
			if firstErr == nil {
				firstErr = fmt.Errorf("go vet %s: %w", d, err)
			}
			continue
		}
		attempted++
		findings = append(findings, parsed...)
	}
	if attempted > 0 && failed == attempted && firstErr != nil {
		return nil, firstErr
	}
	return findings, nil
}

// vetTarget turns a repository-relative directory into a go-tool package
// path. Bare names like "cmd/loka" are resolved as stdlib; prefixing "./"
// keeps the lint bridge inside this module.
func vetTarget(dir string) string {
	if dir == "" || dir == "." {
		return "."
	}
	if filepath.IsAbs(dir) || strings.HasPrefix(dir, ".") {
		return dir
	}
	return "./" + dir
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
	sort.Strings(dirs)
	return dirs
}

// excludedByBuildTags reports whether go vet failed only because every file
// in the package is tagged out of the default build.
func excludedByBuildTags(out []byte, err error) bool {
	if err == nil {
		return false
	}
	msg := string(out) + err.Error()
	return strings.Contains(msg, "build constraints exclude all Go files")
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
