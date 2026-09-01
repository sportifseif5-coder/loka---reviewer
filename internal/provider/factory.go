package provider

import (
	"fmt"
	"strings"

	"github.com/sportifseif5-coder/loka---reviewer/internal/config"
)

// FromConfig builds a Provider from one config.Provider entry. Unknown
// provider names are errors so a misconfiguration surfaces at startup rather
// than silently skipping the LLM layer.
func FromConfig(p config.Provider) (Provider, error) {
	switch strings.ToLower(strings.TrimSpace(p.Name)) {
	case "ollama":
		return NewOllama(p.Model, p.BaseURL), nil
	case "llamacpp", "llama.cpp", "llama_cpp":
		return NewLlamaCpp(p.Model, p.BaseURL), nil
	case "openai", "openai-compatible", "remote":
		return NewOpenAICompatible(p), nil
	case "fixture":
		// A fixture provider is only meaningful in tests; it is still exposed
		// so config can drive deterministic demo/CI runs.
		return &FixtureProvider{name: "fixture", model: p.Model, local: p.IsLocal()}, nil
	}
	return nil, fmt.Errorf("unknown provider %q", p.Name)
}

// Resolve builds the routing set from the effective configuration. The router
// is empty (not an error) when no providers are configured; the engine then
// skips the LLM stage and delivers the deterministic baseline.
func Resolve(cfg *config.Config) (*Router, error) {
	r := NewRouter()
	for _, p := range cfg.Providers {
		pv, err := FromConfig(p)
		if err != nil {
			return nil, err
		}
		r.Add(pv)
	}
	return r, nil
}
