package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sportifseif5-coder/loka---reviewer/internal/vcs"
)

// SystemPrompt primes the review model. Output is structured JSON so it can be
// parsed deterministically and verified against the added lines (I8).
const SystemPrompt = `You are an expert code reviewer embedded in a deterministic review pipeline.
You are shown the added lines of a working-tree diff, the deterministic baseline findings,
and related code from files impacted by the change (context only).
Report NEW issues on the ADDED lines shown — never on unchanged or related-file lines.
Return a JSON array. Every issue must have: "file" (exact path), "line" (the added
line number), "severity" ("info"|"warning"|"error"|"critical"), "message" (one sentence),
and "reasoning" (why it matters). A small number of high-confidence findings beats noise.
Output only the JSON array, no prose, no markdown fences.`

// The context pack is assembled in relevance order (architecture section 4.2):
// the baseline findings always fit first because they are ground truth the
// model must not duplicate, then the changed files' added lines, then the
// related files' code from the graph-index impact slice (architecture 5.3)
// ordered by distance from the change. Oversized packs are truncated from the
// tail (the least relevant impact code first), never the baseline.
const (
	baselineHeader = "Deterministic baseline findings:\n"
	diffHeader     = "Changed files (added lines):\n"
	relevantHeader = "Related code (files impacted by the change):\n"
)

// packContext assembles the token-budgeted context pack handed to the model:
// the deterministic baseline findings, the changed files' added lines, and
// the related files' code from the impact slice. The baseline is written
// first and never truncated; content that does not fit is dropped from the
// tail (farthest-impact code first), with a truncation note appended.
func packContext(req Request, contextBudget int) string {
	var b strings.Builder
	truncated := false

	// Baseline is ground truth the model must not duplicate.
	b.WriteString(baselineHeader)
	if len(req.Baseline) == 0 {
		b.WriteString("(none)\n")
	} else {
		for _, f := range req.Baseline {
			fmt.Fprintf(&b, "- [%s] %s:%d %s\n", f.Severity, f.Location.File, f.Location.LineStart, f.Message)
		}
	}

	remaining := contextBudget - estimateTokens(b.String())
	if !emit(&b, &remaining, "\n") {
		truncated = true
	}
	if !emitDiff(&b, &remaining, req.Changed) {
		truncated = true
	}
	if !emitRelevant(&b, &remaining, req.Relevant) {
		truncated = true
	}
	if truncated {
		b.WriteString("\n(context truncated to fit the token budget)\n")
	}
	return b.String()
}

// emit appends s when it fits the remaining token budget.
func emit(b *strings.Builder, remaining *int, s string) bool {
	t := estimateTokens(s)
	if *remaining < t {
		return false
	}
	b.WriteString(s)
	*remaining -= t
	return true
}

// emitDiff writes the changed files' added lines. Files are shown in diff
// order; each hunk is skipped once the budget is exhausted.
func emitDiff(b *strings.Builder, remaining *int, changed []vcs.ChangedFile) bool {
	if !emit(b, remaining, diffHeader) {
		return false
	}
	for _, cf := range changed {
		if !emit(b, remaining, fmt.Sprintf("--- %s\n", cf.Path)) {
			return false
		}
		for _, ln := range cf.Added {
			if !emit(b, remaining, fmt.Sprintf("+%d %s\n", ln.Number, ln.Text)) {
				return false
			}
		}
	}
	return true
}

// emitRelevant writes the related files' code from the impact slice, ordered
// by distance then path so the closest-impact files are shown first and the
// tail (farthest impact) is what truncation drops first.
func emitRelevant(b *strings.Builder, remaining *int, relevant []ContextFile) bool {
	if len(relevant) == 0 {
		return true
	}
	if !emit(b, remaining, relevantHeader) {
		return false
	}
	files := make([]ContextFile, len(relevant))
	copy(files, relevant)
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].Distance != files[j].Distance {
			return files[i].Distance < files[j].Distance
		}
		return files[i].Path < files[j].Path
	})
	for _, f := range files {
		if !emit(b, remaining, fmt.Sprintf("--- %s (distance %d)\n", f.Path, f.Distance)) {
			return false
		}
		lines := strings.Split(strings.TrimSuffix(f.Code, "\n"), "\n")
		for _, ln := range lines {
			if ln == "" {
				continue
			}
			if !emit(b, remaining, ln+"\n") {
				return false
			}
		}
	}
	return true
}
