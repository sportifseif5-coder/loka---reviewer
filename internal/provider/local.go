package provider

import "context"

// DefaultOllamaURL is Ollama's default local endpoint.
const DefaultOllamaURL = "http://localhost:11434"

// DefaultLlamaCppURL is llama.cpp server's default local endpoint.
const DefaultLlamaCppURL = "http://localhost:8080"

// Ollama is a local provider backed by an Ollama instance. Content never
// leaves the machine (invariant I1).
type Ollama struct {
	name  string
	model string
	c     *client
}

// NewOllama builds a local Ollama provider for the given model. baseURL
// defaults to the Ollama local endpoint when empty.
func NewOllama(model, baseURL string) *Ollama {
	if baseURL == "" {
		baseURL = DefaultOllamaURL
	}
	return &Ollama{name: "ollama", model: model, c: newClient(baseURL, model, "")}
}

// Name returns "ollama".
func (o *Ollama) Name() string { return o.name }

// Local always reports true: Ollama serves on the local machine.
func (o *Ollama) Local() bool { return true }

// Available reports whether the local Ollama endpoint is reachable.
func (o *Ollama) Available(ctx context.Context) bool { return o.c.ping(ctx) }

// Complete proxies to the local Ollama chat completions endpoint.
func (o *Ollama) Complete(ctx context.Context, req Request) (Result, error) {
	return o.c.Complete(ctx, req)
}

// LlamaCpp is a local provider backed by a llama.cpp server (OpenAI-compatible
// mode). Content never leaves the machine (invariant I1).
type LlamaCpp struct {
	name  string
	model string
	c     *client
}

// NewLlamaCpp builds a local llama.cpp provider. baseURL defaults to the
// llama.cpp server default endpoint when empty.
func NewLlamaCpp(model, baseURL string) *LlamaCpp {
	if baseURL == "" {
		baseURL = DefaultLlamaCppURL
	}
	return &LlamaCpp{name: "llamacpp", model: model, c: newClient(baseURL, model, "")}
}

// Name returns "llamacpp".
func (l *LlamaCpp) Name() string { return l.name }

// Local always reports true: llama.cpp serves on the local machine.
func (l *LlamaCpp) Local() bool { return true }

// Available reports whether the local llama.cpp endpoint is reachable.
func (l *LlamaCpp) Available(ctx context.Context) bool { return l.c.ping(ctx) }

// Complete proxies to the local llama.cpp chat completions endpoint.
func (l *LlamaCpp) Complete(ctx context.Context, req Request) (Result, error) {
	return l.c.Complete(ctx, req)
}
