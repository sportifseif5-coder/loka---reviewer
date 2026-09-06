// Package agent implements the LLM-augmented review layer (architecture
// section 8). The Review Agent assembles a token-budgeted context pack and
// asks the provider router for candidate findings; the Verification Agent
// checks candidates against the added lines deterministically and demotes
// rather than deletes those it cannot confirm. Every failure path degrades
// so the deterministic baseline always ships (invariants I2/I3).
package agent

import (
	"context"

	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/provider"
	"github.com/sportifseif5-coder/loka---reviewer/internal/vcs"
)

// DefaultBudget bounds a review pass when no budget is configured.
const DefaultBudget = 12000

// ContextFile is one file of the graph-index impact slice (architecture
// section 5.3) whose code is included in the context pack so the model can
// reason about callers/callees of the change. Distance ranks how directly the
// file touches the changed files (1 = a file that references or is referenced
// by a changed file). Code is the file's current content.
type ContextFile struct {
	Path     string
	Distance int
	Code     string
}

// Request is the input an agent turns into findings.
type Request struct {
	RepoPath string
	// Changed carries the added lines of the working-tree diff.
	Changed []vcs.ChangedFile
	// Baseline carries the deterministic findings so the model does not
	// duplicate them and can reason about them.
	Baseline []model.Finding
	// Relevant carries the impact-slice files' code beyond the diff, ordered
	// by distance then path (see packContext).
	Relevant []ContextFile
	// Budget bounds context plus completion tokens for the pass. Zero means
	// DefaultBudget.
	Budget int
}

// Result is the agent output.
type Result struct {
	Findings []model.Finding
	Model    string
	Usage    provider.Usage
	// Invalid counts model entries dropped for missing location/message (I8).
	Invalid int
	// Demoted counts candidates the verification agent could not confirm.
	Demoted int
}

// Agent is the agent-layer role interface.
type Agent interface {
	Name() string
	// Run executes one agent pass over the request through the router.
	// localOnly excludes remote providers (offline mode, invariant I6).
	Run(ctx context.Context, router *provider.Router, localOnly bool, req Request) (*Result, error)
}

// splitBudget divides a review budget into context (input) and completion
// (output) token shares. Output is bounded so a runaway model cannot blow the
// whole budget; the context share holds the diff and baseline.
func splitBudget(budget int) (contextTokens, outputTokens int) {
	if budget <= 0 {
		budget = DefaultBudget
	}
	contextTokens = (budget * 3) / 4
	outputTokens = budget - contextTokens
	if outputTokens < 128 {
		outputTokens = 128
	}
	return contextTokens, outputTokens
}

// estimateTokens is a cheap code-token heuristic (~4 chars per token).
func estimateTokens(s string) int {
	return (len(s) + 3) / 4
}
