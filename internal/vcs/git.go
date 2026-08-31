package vcs

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Git is a VCSAdapter backed by the system git binary.
type Git struct {
	// Bin is the git executable; defaults to "git".
	Bin string
}

// NewGit returns a Git adapter using the "git" binary found on PATH.
func NewGit() *Git {
	return &Git{Bin: "git"}
}

func (g *Git) bin() string {
	if g.Bin == "" {
		return "git"
	}
	return g.Bin
}

// run executes git with the given arguments inside dir.
func (g *Git) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, g.bin(), args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return out.Bytes(), nil
}

// IsRepo reports whether path is inside a git work tree.
func (g *Git) IsRepo(ctx context.Context, path string) bool {
	_, err := g.run(ctx, path, "rev-parse", "--is-inside-work-tree")
	return err == nil
}

// WorkingDiff returns the working-tree diff relative to HEAD, including
// untracked files. Staged and unstaged changes are combined.
func (g *Git) WorkingDiff(ctx context.Context, path string) (*Diff, error) {
	if !g.IsRepo(ctx, path) {
		return nil, ErrNotRepository
	}

	statusOut, err := g.run(ctx, path, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return nil, err
	}

	// status line: XY path / XY old -> new / ?? path
	entries, err := parseStatus(string(statusOut))
	if err != nil {
		return nil, err
	}

	d := &Diff{Files: map[string]string{}}
	for _, e := range entries {
		change := Change{Path: e.path, OldPath: e.oldPath, Status: e.status}
		switch e.status {
		case StatusDeleted:
			d.Files[e.path] = oneFileDiff(ctx, g, path, e.path)
		case StatusAdded:
			if e.untracked {
				// No base version exists; synthesize an all-additions diff
				// from the working file so analyzers see the content.
				d.Files[e.path] = syntheticAddedDiff(path, e.path)
			} else {
				d.Files[e.path] = oneFileDiff(ctx, g, path, e.path)
			}
		default:
			diffText := oneFileDiff(ctx, g, path, e.path)
			if e.status == StatusRenamed && diffText == "" {
				diffText = oneFileDiff(ctx, g, path, e.oldPath)
			}
			d.Files[e.path] = diffText
		}
		change.Additions, change.Deletions = countStat(d.Files[e.path])
		d.Changes = append(d.Changes, change)
	}
	return d, nil
}

// oneFileDiff returns the unified diff of a file against HEAD, which covers
// both staged and unstaged changes.
func oneFileDiff(ctx context.Context, g *Git, dir, file string) string {
	out, err := g.run(ctx, dir, "diff", "HEAD", "--unified=3", "--", file)
	if err != nil {
		return ""
	}
	return string(out)
}

// ParseChangedFiles extracts added lines from per-file unified diffs.
func ParseChangedFiles(diffs map[string]string) []ChangedFile {
	var out []ChangedFile
	for path, text := range diffs {
		out = append(out, ChangedFile{Path: path, Added: addedLines(text)})
	}
	return out
}

// addedLines parses added lines out of a unified diff, tracking the new-side
// line numbers.
func addedLines(diff string) []AddedLine {
	var out []AddedLine
	newLine := 0
	for _, ln := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(ln, "@@"):
			// @@ -o[,n] +n[,m] @@  -> the second range gives the new start.
			if fields := strings.Fields(ln); len(fields) >= 3 {
				if n, ok := parseNewStart(fields[2]); ok {
					newLine = n
				}
			}
		case strings.HasPrefix(ln, "+++"), strings.HasPrefix(ln, "---"):
			continue
		case strings.HasPrefix(ln, "+"):
			out = append(out, AddedLine{Number: newLine, Text: strings.TrimPrefix(ln, "+")})
			newLine++
		case strings.HasPrefix(ln, " "):
			newLine++
		case strings.HasPrefix(ln, "-"):
			// deletion: new-side counter does not advance
		}
	}
	return out
}

func parseNewStart(field string) (int, bool) {
	t := strings.TrimPrefix(field, "+")
	parts := strings.SplitN(t, ",", 2)
	n, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, false
	}
	return n, true
}

// syntheticAddedDiff builds a unified diff treating every line as an
// addition, for files git does not track yet.
func syntheticAddedDiff(dir, file string) string {
	var b strings.Builder
	rel := file
	b.WriteString("--- /dev/null\n")
	b.WriteString("+++ b/" + rel + "\n")
	data, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	b.WriteString("@@ -0,0 +1," + strconv.Itoa(len(lines)) + " @@\n")
	for _, ln := range lines {
		b.WriteString("+" + ln + "\n")
	}
	return b.String()
}

// Blame returns the commit and author of the commit that last touched a line.
func (g *Git) Blame(ctx context.Context, path, file string, line int) (commit, author string, err error) {
	arg := strconv.Itoa(line) + "," + strconv.Itoa(line)
	out, err := g.run(ctx, path, "blame", "-L", arg, "--porcelain", "--", file)
	if err != nil {
		return "", "", err
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) > 0 {
		fields := strings.Fields(lines[0])
		if len(fields) > 0 {
			commit = fields[0]
		}
	}
	for _, ln := range lines {
		if strings.HasPrefix(ln, "author ") {
			author = strings.TrimSpace(strings.TrimPrefix(ln, "author "))
			break
		}
	}
	return commit, author, nil
}

// statusEntry is a parsed porcelain status line.
type statusEntry struct {
	path      string
	oldPath   string
	status    ChangeStatus
	untracked bool
}

func parseStatus(out string) ([]statusEntry, error) {
	var entries []statusEntry
	for _, raw := range strings.Split(out, "\n") {
		if raw == "" {
			continue
		}
		if strings.HasPrefix(raw, "??") {
			entries = append(entries, statusEntry{
				path:      strings.TrimSpace(raw[2:]),
				status:    StatusAdded,
				untracked: true,
			})
			continue
		}
		if len(raw) < 3 {
			continue
		}
		x, y := raw[0], raw[1]
		rest := strings.TrimSpace(raw[3:])
		status := StatusModified
		switch {
		case x == 'A' || y == 'A':
			status = StatusAdded
		case x == 'D' || y == 'D':
			status = StatusDeleted
		case x == 'R':
			status = StatusRenamed
		}
		e := statusEntry{path: rest, status: status}
		if status == StatusRenamed {
			if parts := strings.SplitN(rest, " -> ", 2); len(parts) == 2 {
				e.oldPath = parts[0]
				e.path = parts[1]
			}
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// countStat counts added and deleted lines in a unified diff by inspecting
// hunk body lines, ignoring file headers and context.
func countStat(diff string) (additions, deletions int) {
	inHunk := false
	for _, ln := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(ln, "@@"):
			inHunk = true
		case !inHunk, strings.HasPrefix(ln, "+++"), strings.HasPrefix(ln, "---"):
			continue
		case strings.HasPrefix(ln, "+"):
			additions++
		case strings.HasPrefix(ln, "-"):
			deletions++
		}
	}
	return additions, deletions
}
