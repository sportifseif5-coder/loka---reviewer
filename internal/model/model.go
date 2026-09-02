// Package model defines the core domain types shared across the engine,
// storage, and UI layers. A finding is the atomic unit of review output and
// always carries a location and reasoning (invariant I8).
package model

import (
	"errors"
	"time"
)

// Severity ranks findings.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityError    Severity = "error"
	SeverityCritical Severity = "critical"
)

// Source identifies which layer produced a finding.
type Source string

const (
	SourceAnalyzer Source = "analyzer"
	SourceRule     Source = "rule"
	SourceLLM      Source = "llm"
	SourceAgent    Source = "agent"
)

// Location pins a finding to a place in the reviewed code.
type Location struct {
	File      string `json:"file"`
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end"`
}

// Finding is one issue with evidence attached.
type Finding struct {
	ID         string   `json:"id"`
	RuleID     string   `json:"rule_id,omitempty"`
	Category   string   `json:"category"`
	Severity   Severity `json:"severity"`
	Source     Source   `json:"source"`
	Location   Location `json:"location"`
	Message    string   `json:"message"`
	Reasoning  string   `json:"reasoning,omitempty"`
	Evidence   []string `json:"evidence,omitempty"`
	Confidence float64  `json:"confidence"`
	// Demoted marks an LLM/agent finding that the verification agent could
	// not confirm against the added lines. Demoted findings are kept (never
	// silently deleted) but capped at info severity and ranked last.
	Demoted bool `json:"demoted,omitempty"`
}

// ReviewRequest asks the engine to review a repository.
type ReviewRequest struct {
	RepoPath string `json:"repo_path"`
	Mode     string `json:"mode"`
}

// ReviewResult is the full output of one review run.
type ReviewResult struct {
	ID           string    `json:"id"`
	RepoPath     string    `json:"repo_path"`
	Mode         string    `json:"mode"`
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at"`
	Findings     []Finding `json:"findings"`
	AnalyzersRun []string  `json:"analyzers_run"`
	Warnings     []string  `json:"warnings,omitempty"`
	Degradations []string  `json:"degradations,omitempty"`
}

// ErrEmptyRepoPath is returned when a review is requested without a repo path.
var ErrEmptyRepoPath = errors.New("repo path must not be empty")
