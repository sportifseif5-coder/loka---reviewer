package agent

import (
	"fmt"
	"strings"
)

// SystemPrompt primes the review model. Output is structured JSON so it can be
// parsed deterministically and verified against the added lines (I8).
const SystemPrompt = `You are an expert code reviewer embedded in a deterministic review pipeline.
You are shown the added lines of a working-tree diff and the deterministic baseline findings.
Report NEW issues on the ADDED lines shown — never on unchanged lines.
Return a JSON array. Every issue must have: "file" (exact path), "line" (the added
line number), "severity" ("info"|"warning"|"error"|"critical"), "message" (one sentence),
and "reasoning" (why it matters). A small number of high-confidence findings beats noise.
Output only the JSON array, no prose, no markdown fences.`

// contextFraction of the context share is reserved for the changed files;
// the baseline findings always fit first because they are ground truth the
// model must not duplicate.
const (
	baselineHeader = "Deterministic baseline findings:\n"
	diffHeader     = "Changed files (added lines):\n"
)

// packContext assembles the token-budgeted context pack handed to the model:
// the changed files' added lines plus the deterministic baseline findings.
// The baseline is written first and never truncated; the diff tail is dropped
// once the context budget is exhausted, with a truncation note appended.
// (Architecture section 4.2; relevance ordering beyond diff order arrives
// with the graph index.)
func packContext(req Request, contextBudget int) string {
	var b strings.Builder
	b.WriteString(diffHeader)
	b.WriteString("(shown first: baseline that must not be duplicated)\n\n")
	b.WriteString(baselineHeader)
	if len(req.Baseline) == 0 {
		b.WriteString("(none)\n")
	} else {
		for _, f := range req.Baseline {
			line := fmt.Sprintf("- [%s] %s:%d %s\n", f.Severity, f.Location.File, f.Location.LineStart, f.Message)
			b.WriteString(line)
		}
	}
	b.WriteString("\n")
	b.WriteString(diffHeader)

	remaining := contextBudget - estimateTokens(b.String())
	truncated := false
	for _, cf := range req.Changed {
		if remaining <= 0 {
			truncated = true
			break
		}
		header := fmt.Sprintf("--- %s\n", cf.Path)
		if estimateTokens(header) > remaining {
			truncated = true
			break
		}
		b.WriteString(header)
		remaining -= estimateTokens(header)
		for _, ln := range cf.Added {
			line := fmt.Sprintf("+%d %s\n", ln.Number, ln.Text)
			if estimateTokens(line) > remaining {
				truncated = true
				break
			}
			b.WriteString(line)
			remaining -= estimateTokens(line)
		}
	}
	if truncated {
		b.WriteString("\n(context truncated to fit the token budget)\n")
	}
	return b.String()
}
