// Package provider implements the provider-plugin LLM layer (ADR-0003). The
// core engine never imports a vendor SDK; all LLM interaction goes through
// the Provider interface (invariant I4). Local providers (Ollama, llama.cpp)
// never leave the machine; remote providers are excluded in offline mode
// (invariant I6).
package provider

import (
	"context"
	"errors"
)

// Request is one completion request to a model backend.
type Request struct {
	// Model overrides the provider's configured model when non-empty.
	Model       string
	System      string
	Prompt      string
	MaxTokens   int
	Temperature float64
}

// Result is one completion result from a model backend.
type Result struct {
	Content string
	Model   string
	Usage   Usage
}

// Usage reports token accounting as reported by the backend.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// Provider is an LLM backend. Implementations must be safe to use with no
// keys and no network (offline mode), degrading through availability rather
// than erroring at construction time.
type Provider interface {
	// Name identifies the provider kind and configured entry.
	Name() string
	// Local reports whether the provider is a local-only backend. Local
	// providers never transmit content off the machine.
	Local() bool
	// Available reports whether the provider can serve a request right now
	// without requiring network egress (for local providers) and without
	// failing on configuration alone.
	Available(ctx context.Context) bool
	// Complete sends a completion request and returns the model response.
	Complete(ctx context.Context, req Request) (Result, error)
}

// ErrNoProvider is returned when no eligible provider could serve a request.
// Callers treat it as "LLM skipped" and deliver the deterministic baseline.
var ErrNoProvider = errors.New("no provider available")

// ErrNoMatch is returned by recorded-fixture providers when the prompt has no
// recorded transcript entry.
var ErrNoMatch = errors.New("no recorded fixture for prompt")

// Router holds providers in preference order and implements the ADR-0003
// routing policy: local providers are always tried first, then remote
// providers (unless localOnly); a failure or unavailability moves to the next
// provider, and total failure surfaces ErrNoProvider.
type Router struct {
	providers []Provider
}

// NewRouter builds an empty router.
func NewRouter() *Router { return &Router{} }

// Add appends a provider to the routing set.
func (r *Router) Add(p Provider) { r.providers = append(r.providers, p) }

// Len returns the number of registered providers.
func (r *Router) Len() int { return len(r.providers) }

// Complete routes a request. With localOnly (offline mode) remote providers
// are never consulted. Local providers are always tried before remote ones.
func (r *Router) Complete(ctx context.Context, req Request, localOnly bool) (Result, error) {
	for _, p := range r.providers {
		if p.Local() {
			if res, ok := tryOne(ctx, p, req); ok {
				return res, nil
			}
		}
	}
	if !localOnly {
		for _, p := range r.providers {
			if !p.Local() {
				if res, ok := tryOne(ctx, p, req); ok {
					return res, nil
				}
			}
		}
	}
	return Result{}, ErrNoProvider
}

func tryOne(ctx context.Context, p Provider, req Request) (Result, bool) {
	if !p.Available(ctx) {
		return Result{}, false
	}
	res, err := p.Complete(ctx, req)
	if err != nil {
		return Result{}, false
	}
	return res, true
}
