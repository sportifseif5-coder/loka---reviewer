package agent

import (
	"context"

	"github.com/sportifseif5-coder/loka---reviewer/internal/provider"
)

// ReviewAgent is the primary agent role: it assembles the context pack,
// completes through the provider router, parses candidate findings, and runs
// them through the verification agent before returning. It opens no network
// path of its own; all transport happens inside the router (ADR-0003).
type ReviewAgent struct {
	verify *VerificationAgent
}

// NewReviewAgent builds a review agent with a verification stage.
func NewReviewAgent() *ReviewAgent {
	return &ReviewAgent{verify: NewVerificationAgent()}
}

// Name returns "review".
func (a *ReviewAgent) Name() string { return "review" }

// Run executes the review pass. localOnly keeps remote providers out of the
// routing set (offline mode, invariant I6). Errors are returned unwrapped so
// the engine can record an "LLM stage skipped" degradation and ship the
// deterministic baseline.
func (a *ReviewAgent) Run(ctx context.Context, router *provider.Router, localOnly bool, req Request) (*Result, error) {
	if router == nil || router.Len() == 0 {
		return nil, provider.ErrNoProvider
	}
	contextBudget, outputBudget := splitBudget(req.Budget)
	prompt := packContext(req, contextBudget)

	result, err := router.Complete(ctx, provider.Request{
		System:      SystemPrompt,
		Prompt:      prompt,
		MaxTokens:   outputBudget,
		Temperature: 0.2,
	}, localOnly)
	if err != nil {
		return nil, err
	}

	out := &Result{Model: result.Model, Usage: result.Usage}
	candidates, invalid, err := parseFindings(result.Content)
	if err != nil {
		return nil, err
	}
	out.Invalid = invalid
	out.Findings, out.Demoted = a.verify.Verify(candidates, req.Changed)
	return out, nil
}
