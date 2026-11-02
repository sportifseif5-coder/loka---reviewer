package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/vcs"
)

// FixAgent proposes unified-diff suggestions for confirmed findings.
// It never writes the working tree; ApplySuggestion is a separate, explicit
// user action (invariant I7).
type FixAgent struct{}

// NewFixAgent builds a fix agent.
func NewFixAgent() *FixAgent { return &FixAgent{} }

// Propose attaches SuggestedFix on findings it can patch in place.
func (a *FixAgent) Propose(findings []model.Finding, changed []vcs.ChangedFile) []model.Finding {
	lines := map[string]map[int]string{}
	for _, cf := range changed {
		m := map[int]string{}
		for _, ln := range cf.Added {
			m[ln.Number] = ln.Text
		}
		lines[cf.Path] = m
	}
	out := make([]model.Finding, len(findings))
	copy(out, findings)
	for i := range out {
		if out[i].Demoted || out[i].SuggestedFix != "" {
			continue
		}
		fileLines := lines[out[i].Location.File]
		old, ok := fileLines[out[i].Location.LineStart]
		if !ok {
			continue
		}
		next, ok := rewriteLine(out[i], old)
		if !ok || next == old {
			continue
		}
		out[i].SuggestedFix = unifiedLinePatch(out[i].Location.File, out[i].Location.LineStart, old, next)
	}
	return out
}

func rewriteLine(f model.Finding, old string) (string, bool) {
	switch f.RuleID {
	case "trailing-whitespace":
		return strings.TrimRight(old, " \t"), true
	case "debug-print-in-go":
		trimmed := strings.TrimSpace(old)
		if strings.HasPrefix(trimmed, "fmt.Print") || strings.HasPrefix(trimmed, "log.Println") {
			return "", true
		}
	}
	return old, false
}

// unifiedLinePatch is a one-hunk unified diff replacing a single line.
func unifiedLinePatch(file string, line int, old, next string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "--- a/%s\n+++ b/%s\n@@ -%d,1 +%d,1 @@\n", file, file, line, line)
	fmt.Fprintf(&b, "-%s\n", old)
	fmt.Fprintf(&b, "+%s\n", next)
	return b.String()
}

// ApplySuggestion writes a one-hunk line replacement produced by Propose.
// The path must stay inside repoPath (I7: explicit user action, no escape).
func ApplySuggestion(repoPath, file, patch string) error {
	if repoPath == "" || file == "" {
		return fmt.Errorf("apply: repo and file are required")
	}
	if filepath.IsAbs(file) {
		return fmt.Errorf("apply: absolute path %q", file)
	}
	cleaned := filepath.Clean(file)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return fmt.Errorf("apply: path %q escapes the repository", file)
	}
	abs := filepath.Join(repoPath, cleaned)
	oldLine, newLine, line, err := parseLinePatch(patch)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if line < 1 || line > len(lines) {
		return fmt.Errorf("apply: line %d out of range", line)
	}
	if lines[line-1] != oldLine {
		return fmt.Errorf("apply: line %d does not match patch", line)
	}
	lines[line-1] = newLine
	out := strings.Join(lines, "\n")
	if !strings.HasSuffix(string(data), "\n") && !strings.HasSuffix(out, "\n") {
		return os.WriteFile(abs, []byte(out), 0o644)
	}
	return os.WriteFile(abs, []byte(out+"\n"), 0o644)
}

func parseLinePatch(patch string) (oldLine, newLine string, line int, err error) {
	var minus, plus string
	for _, ln := range strings.Split(patch, "\n") {
		if strings.HasPrefix(ln, "@@ ") {
			var start int
			if _, scanErr := fmt.Sscanf(ln, "@@ -%d,", &start); scanErr != nil {
				return "", "", 0, fmt.Errorf("apply: bad hunk header")
			}
			line = start
			continue
		}
		if strings.HasPrefix(ln, "-") && !strings.HasPrefix(ln, "---") {
			minus = strings.TrimPrefix(ln, "-")
			continue
		}
		if strings.HasPrefix(ln, "+") && !strings.HasPrefix(ln, "+++") {
			plus = strings.TrimPrefix(ln, "+")
		}
	}
	if line == 0 {
		return "", "", 0, fmt.Errorf("apply: missing hunk")
	}
	return minus, plus, line, nil
}
