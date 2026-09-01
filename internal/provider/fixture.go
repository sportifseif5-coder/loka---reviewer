package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// Fixture is one recorded completion transcript (constitution section 10.6).
type Fixture struct {
	Model    string `json:"model"`
	System   string `json:"system,omitempty"`
	Prompt   string `json:"prompt"`
	Response string `json:"response"`
}

// FixtureProvider replays recorded transcripts. It never opens a socket and
// never reads keys, so the full pipeline can be exercised offline and in CI
// with deterministic output (invariant I3 applies to the baseline; fixtures
// make the LLM layer deterministic for testing).
type FixtureProvider struct {
	name     string
	model    string
	local    bool
	fixtures []Fixture
}

// NewFixture builds a recorded-transcript provider. local marks whether the
// fake backend is treated as a local provider for routing policy tests.
func NewFixture(name, model string, local bool, fixtures ...Fixture) *FixtureProvider {
	return &FixtureProvider{name: name, model: model, local: local, fixtures: fixtures}
}

// Name returns the fixture provider name.
func (f *FixtureProvider) Name() string { return f.name }

// Local reports the routing locality configured for this fixture.
func (f *FixtureProvider) Local() bool { return f.local }

// Available is always true: a recorded transcript needs no network or keys.
func (f *FixtureProvider) Available(ctx context.Context) bool { return true }

// Complete replays the recorded response whose system matches exactly and
// whose prompt matches exactly or is empty (empty acts as a wildcard, which
// lets tests drive responses without knowing the exact prompt). No match
// fails with ErrNoMatch.
func (f *FixtureProvider) Complete(_ context.Context, req Request) (Result, error) {
	for _, fx := range f.fixtures {
		if fx.System == req.System && (fx.Prompt == "" || fx.Prompt == req.Prompt) {
			return Result{Content: fx.Response, Model: f.model}, nil
		}
	}
	return Result{}, fmt.Errorf("%w: %s", ErrNoMatch, f.name)
}

// LoadFixtureFile reads a JSON array of Fixture transcripts.
func LoadFixtureFile(path string) ([]Fixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fixture file: %w", err)
	}
	var fixtures []Fixture
	if err := json.Unmarshal(data, &fixtures); err != nil {
		return nil, fmt.Errorf("parse fixture file: %w", err)
	}
	return fixtures, nil
}
