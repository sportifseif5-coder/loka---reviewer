package provider

import (
	"context"

	"github.com/sportifseif5-coder/loka---reviewer/internal/config"
	"github.com/sportifseif5-coder/loka---reviewer/internal/secrets"
)

// OpenAICompatible is a provider for any OpenAI-compatible endpoint. It may
// be local (explicitly marked) or remote; remote use requires per-repository
// enablement, which the engine enforces by excluding non-local providers in
// offline mode (invariant I6).
type OpenAICompatible struct {
	name    string
	model   string
	local   bool
	baseURL string
	c       *client
}

// NewOpenAICompatible builds a provider from a config entry. The API key is
// resolved from the OS/file keyring when api_key_keyring is set, otherwise
// from the configured environment variable. An empty key means the endpoint
// needs no authentication. Keys are never logged (constitution 10.5).
func NewOpenAICompatible(p config.Provider) *OpenAICompatible {
	return NewOpenAICompatibleWithStore(p, secrets.DefaultStore())
}

// NewOpenAICompatibleWithStore is the testable constructor: the secret store
// is injected so CI never needs a real keyring.
func NewOpenAICompatibleWithStore(p config.Provider, store secrets.Store) *OpenAICompatible {
	lookup := secrets.Lookup{EnvVar: p.APIKeyEnv}
	if p.APIKeyKeyring != "" {
		if svc, acct, err := secrets.ParseKeyringRef(p.APIKeyKeyring); err == nil {
			lookup.Service = svc
			lookup.Account = acct
		}
	}
	key, _, _ := secrets.Resolve(context.Background(), store, lookup)
	return &OpenAICompatible{
		name:    p.Name,
		model:   p.Model,
		local:   p.IsLocal(),
		baseURL: p.BaseURL,
		c:       newClient(p.BaseURL, p.Model, key),
	}
}

// Name returns the configured provider name.
func (o *OpenAICompatible) Name() string { return o.name }

// Local reports whether the endpoint is treated as local.
func (o *OpenAICompatible) Local() bool { return o.local }

// Available reports whether the endpoint is reachable.
func (o *OpenAICompatible) Available(ctx context.Context) bool { return o.c.ping(ctx) }

// Complete proxies to the configured OpenAI-compatible endpoint.
func (o *OpenAICompatible) Complete(ctx context.Context, req Request) (Result, error) {
	return o.c.Complete(ctx, req)
}

// BaseURL exposes the configured endpoint (used in diagnostics).
func (o *OpenAICompatible) BaseURL() string { return o.baseURL }
