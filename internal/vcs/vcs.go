// Package vcs reads local version-control state for review input. It is
// strictly read-only: no writer mutates a repository (invariant I7). The
// git implementation shells out to the system git binary so the core stays
// dependency-light and offline.
package vcs

import (
	"context"
	"errors"
)

// ChangeStatus describes how a file changed relative to the base snapshot.
type ChangeStatus string

const (
	StatusAdded    ChangeStatus = "added"
	StatusModified ChangeStatus = "modified"
	StatusDeleted  ChangeStatus = "deleted"
	StatusRenamed  ChangeStatus = "renamed"
)

// Change describes one changed path in the working tree.
type Change struct {
	Path      string
	OldPath   string
	Status    ChangeStatus
	Additions int
	Deletions int
}

// Diff is the review input: the working-tree changes relative to HEAD.
type Diff struct {
	Changes []Change
	// Files maps a path to its unified diff text. Deleted files carry the
	// removal diff; untracked files carry a synthetic all-additions diff.
	Files map[string]string
}

// ErrNotRepository is returned when the target path is not inside a git
// work tree.
var ErrNotRepository = errors.New("not a git repository")

// AddedLine is one added line in the working-tree diff.
type AddedLine struct {
	Number int
	Text   string
}

// ChangedFile is a file with its added lines, the deterministic input for
// analyzers and the rules engine.
type ChangedFile struct {
	Path  string
	Added []AddedLine
}

// VCSAdapter is the read-only interface to a local repository.
type VCSAdapter interface {
	// IsRepo reports whether path is inside a supported work tree.
	IsRepo(ctx context.Context, path string) bool
	// WorkingDiff returns changes in the working tree relative to HEAD,
	// including untracked files.
	WorkingDiff(ctx context.Context, path string) (*Diff, error)
	// Blame returns (commit, author) for a single line of a file.
	Blame(ctx context.Context, path, file string, line int) (commit, author string, err error)
}
