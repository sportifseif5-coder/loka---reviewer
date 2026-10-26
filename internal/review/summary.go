package review

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/vcs"
)

const summaryPathCap = 3

var severityOrder = []model.Severity{
	model.SeverityCritical,
	model.SeverityError,
	model.SeverityWarning,
	model.SeverityInfo,
}

// summarize builds the structured walkthrough and risk on a review. It is
// pure and deterministic: the same changed-file set and ranked findings
// always produce the same summary (invariant I3).
func summarize(changed []vcs.ChangedFile, findings []model.Finding) model.ReviewSummary {
	parts := []string{changeClause(changed), countClause(findings)}
	if h := highestClause(findings); h != "" {
		parts = append(parts, h)
	}
	return model.ReviewSummary{
		Walkthrough: strings.Join(parts, " "),
		Risk:        riskLevel(findings),
	}
}

func riskLevel(findings []model.Finding) string {
	highest := model.Severity("")
	active := false
	for _, f := range findings {
		if f.Demoted {
			continue
		}
		active = true
		if severityWeight[f.Severity] > severityWeight[highest] {
			highest = f.Severity
		}
	}
	if !active {
		if len(findings) > 0 {
			return model.RiskLow
		}
		return model.RiskNone
	}
	switch highest {
	case model.SeverityCritical:
		return model.RiskCritical
	case model.SeverityError:
		return model.RiskHigh
	case model.SeverityWarning:
		return model.RiskMedium
	default:
		return model.RiskLow
	}
}

func changeClause(changed []vcs.ChangedFile) string {
	paths := changedPaths(changed)
	n := len(paths)
	if n == 0 {
		return "No working-tree changes."
	}
	listed := formatPaths(paths)
	if n == 1 {
		return fmt.Sprintf("Reviewed 1 changed file (%s).", listed)
	}
	return fmt.Sprintf("Reviewed %d changed files (%s).", n, listed)
}

func countClause(findings []model.Finding) string {
	if len(findings) == 0 {
		return "No findings."
	}
	counts := map[model.Severity]int{}
	demoted := 0
	for _, f := range findings {
		counts[f.Severity]++
		if f.Demoted {
			demoted++
		}
	}
	parts := make([]string, 0, len(severityOrder))
	for _, sev := range severityOrder {
		if n := counts[sev]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, sev))
		}
	}
	noun := "findings"
	if len(findings) == 1 {
		noun = "finding"
	}
	s := fmt.Sprintf("%d %s: %s.", len(findings), noun, strings.Join(parts, ", "))
	if demoted > 0 {
		s = strings.TrimSuffix(s, ".") + fmt.Sprintf(" (%d demoted).", demoted)
	}
	return s
}

func highestClause(findings []model.Finding) string {
	for _, f := range findings {
		if f.Demoted {
			continue
		}
		what := f.RuleID
		if what == "" {
			what = f.Category
		}
		loc := f.Location.File
		if loc == "" {
			loc = "?"
		}
		if what != "" {
			return fmt.Sprintf("Highest: %s %s in %s:%d.", f.Severity, what, loc, f.Location.LineStart)
		}
		return fmt.Sprintf("Highest: %s in %s:%d.", f.Severity, loc, f.Location.LineStart)
	}
	return ""
}

func changedPaths(changed []vcs.ChangedFile) []string {
	paths := make([]string, 0, len(changed))
	seen := map[string]bool{}
	for _, cf := range changed {
		if cf.Path == "" || seen[cf.Path] {
			continue
		}
		seen[cf.Path] = true
		paths = append(paths, cf.Path)
	}
	sort.Strings(paths)
	return paths
}

func formatPaths(paths []string) string {
	if len(paths) <= summaryPathCap {
		return strings.Join(paths, ", ")
	}
	return strings.Join(paths[:summaryPathCap], ", ") + fmt.Sprintf(", +%d more", len(paths)-summaryPathCap)
}
