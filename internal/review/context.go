package review

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
)

// DefaultContextRadius is the number of context lines shown above and below a
// finding when the caller does not specify one.
const DefaultContextRadius = 3

// CodeLine is one numbered working-tree source line for the workbench's inline
// finding view.
type CodeLine struct {
	Number int    `json:"number"`
	Text   string `json:"text"`
}

// FileWindow returns the working-tree lines of file around the finding
// location: the located range plus radius context lines above and below. The
// path is resolved inside repoPath and must not escape it. The read is
// read-only and never mutates the tree (invariant I7).
func FileWindow(repoPath, file string, loc model.Location, radius int) ([]CodeLine, error) {
	if repoPath == "" || file == "" {
		return nil, fmt.Errorf("file window: repo and file are required")
	}
	if radius <= 0 {
		radius = DefaultContextRadius
	}
	abs, err := safeJoin(repoPath, file)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	total := len(lines)

	anchorStart := loc.LineStart
	if anchorStart <= 0 {
		anchorStart = 1
	}
	anchorEnd := loc.LineEnd
	if anchorEnd < anchorStart {
		anchorEnd = anchorStart
	}

	start := anchorStart - radius
	if start < 1 {
		start = 1
	}
	end := anchorEnd + radius
	if end > total {
		end = total
	}
	if start > end {
		return nil, nil
	}

	out := make([]CodeLine, 0, end-start+1)
	for n := start; n <= end; n++ {
		out = append(out, CodeLine{Number: n, Text: lines[n-1]})
	}
	return out, nil
}

// safeJoin resolves file under root and rejects absolute paths and any path
// that climbs out of the repository.
func safeJoin(root, file string) (string, error) {
	if filepath.IsAbs(file) {
		return "", fmt.Errorf("file window: absolute path %q", file)
	}
	cleaned := filepath.Clean(file)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("file window: path %q escapes the repository", file)
	}
	return filepath.Join(root, cleaned), nil
}
