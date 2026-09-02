package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
)

// rawFinding is the JSON shape the model must return. The layer is
// non-deterministic by nature; every accepted finding is labeled Source=llm
// and verified against the added lines before it is trusted.
type rawFinding struct {
	File      string `json:"file"`
	Line      int    `json:"line"`
	Severity  string `json:"severity"`
	Message   string `json:"message"`
	Reasoning string `json:"reasoning,omitempty"`
	RuleID    string `json:"rule,omitempty"`
}

// parseFindings converts model output into candidate findings, enforcing I8:
// an entry without a location and a message is dropped and counted, never
// silently kept.
func parseFindings(content string) ([]model.Finding, int, error) {
	var raw []rawFinding
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, 0, fmt.Errorf("parse model output as JSON: %w", err)
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
			Severity:  parseSeverity(rf.Severity),
			Source:    model.SourceLLM,
			Location:  model.Location{File: rf.File, LineStart: rf.Line, LineEnd: rf.Line},
			Message:   rf.Message,
			Reasoning: rf.Reasoning,
			// Start below the deterministic baseline; the verification agent
			// raises this for confirmed findings and drops it on demotion.
			Confidence: 0.5,
		})
	}
	return out, dropped, nil
}

func parseSeverity(s string) model.Severity {
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
