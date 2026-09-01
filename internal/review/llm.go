package review

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sportifseif5-coder/loka---reviewer/internal/analyzer"
	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
)

// llmSystemPrompt primes the model to produce only actionable findings with
// locations, keeping output parseable and consistent with I8.
const llmSystemPrompt = `You are an expert code reviewer embedded in a deterministic review pipeline.
You see the changed files of a working tree and the deterministic baseline findings.
Return a JSON array of NEW issues not already covered by the baseline.
Every issue must have: "file" (path), "line" (line number in the changed file),
"severity" ("info"|"warning"|"error"|"critical"), "message" (one sentence),
and "reasoning" (why it matters). Prefer a small number of high-confidence findings
over noise. Output only the JSON array, no prose.`

// packContext assembles the compact, token-budgeted review context handed to
// the LLM layer: changed files with added lines plus the deterministic
// baseline findings. The full context pack assembly is refined in the agent
// layer; this keeps the Phase 1 LLM stage minimal.
func packContext(unit analyzer.AnalysisUnit, baseline []model.Finding) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Changed files (added lines):\n")
	for _, cf := range unit.Changed {
		fmt.Fprintf(&b, "--- %s\n", cf.Path)
		for _, ln := range cf.Added {
			fmt.Fprintf(&b, "+%d %s\n", ln.Number, ln.Text)
		}
	}
	fmt.Fprintf(&b, "\nDeterministic baseline findings:\n")
	if len(baseline) == 0 {
		fmt.Fprintf(&b, "(none)\n")
	}
	for _, f := range baseline {
		fmt.Fprintf(&b, "- [%s] %s:%d %s\n", f.Severity, f.Location.File, f.Location.LineStart, f.Message)
	}
	return b.String()
}

// llmFinding is the JSON shape the LLM layer must return. Non-deterministic
// by nature; labeled Source=llm.
type llmFinding struct {
	File      string `json:"file"`
	Line      int    `json:"line"`
	Severity  string `json:"severity"`
	Message   string `json:"message"`
	Reasoning string `json:"reasoning,omitempty"`
	RuleID    string `json:"rule,omitempty"`
}

// parseLLMFindings converts model output into model.Finding values, enforcing
// I8: an entry without a location and a message is dropped and counted, never
// silently kept.
func parseLLMFindings(content string) ([]model.Finding, int, error) {
	var raw []llmFinding
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, 0, fmt.Errorf("parse LLM output as JSON: %w", err)
	}

	out := make([]model.Finding, 0, len(raw))
	dropped := 0
	for _, rf := range raw {
		if rf.File == "" || rf.Line <= 0 || rf.Message == "" {
			dropped++
			continue
		}
		ruleID := rf.RuleID
		if ruleID == "" {
			ruleID = "llm"
		}
		out = append(out, model.Finding{
			RuleID:    ruleID,
			Category:  "llm",
			Severity:  parseLLMSeverity(rf.Severity),
			Source:    model.SourceLLM,
			Location:  model.Location{File: rf.File, LineStart: rf.Line, LineEnd: rf.Line},
			Message:   rf.Message,
			Reasoning: rf.Reasoning,
			// Non-deterministic output is labeled and never ranked above the
			// deterministic baseline.
			Confidence: 0.5,
		})
	}
	return out, dropped, nil
}

func parseLLMSeverity(s string) model.Severity {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical":
		return model.SeverityCritical
	case "error":
		return model.SeverityError
	case "info":
		return model.SeverityInfo
	default:
		return model.SeverityWarning
	}
}
