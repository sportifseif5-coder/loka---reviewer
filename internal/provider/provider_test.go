package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/sportifseif5-coder/loka---reviewer/internal/config"
)

// stubProvider is a controllable fake for routing tests. It never opens a
// socket, keeping the suite runnable with networking disabled.
type stubProvider struct {
	name     string
	local    bool
	avail    bool
	response string
}

func (s stubProvider) Name() string                   { return s.name }
func (s stubProvider) Local() bool                    { return s.local }
func (s stubProvider) Available(context.Context) bool { return s.avail }
func (s stubProvider) Complete(_ context.Context, _ Request) (Result, error) {
	return Result{Content: s.response, Model: s.name}, nil
}

func TestRouterLocalPreferred(t *testing.T) {
	remote := stubProvider{name: "remote", local: false, avail: true, response: "from-remote"}
	local := stubProvider{name: "local", local: true, avail: true, response: "from-local"}
	r := NewRouter()
	// Remote registered first: local must still win (ADR-0003 policy).
	r.Add(remote)
	r.Add(local)

	res, err := r.Complete(context.Background(), Request{Prompt: "p"}, false)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if res.Content != "from-local" {
		t.Errorf("content = %q, want local preferred", res.Content)
	}
}

func TestRouterFailover(t *testing.T) {
	down := stubProvider{name: "down", local: true, avail: false}
	up := stubProvider{name: "up", local: true, avail: true, response: "ok"}
	r := NewRouter()
	r.Add(down)
	r.Add(up)

	res, err := r.Complete(context.Background(), Request{Prompt: "p"}, true)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if res.Content != "ok" {
		t.Errorf("content = %q, want failover to next provider", res.Content)
	}
}

func TestRouterOfflineExcludesRemote(t *testing.T) {
	remote := stubProvider{name: "remote", local: false, avail: true, response: "from-remote"}
	r := NewRouter()
	r.Add(remote)

	if _, err := r.Complete(context.Background(), Request{Prompt: "p"}, true); !errors.Is(err, ErrNoProvider) {
		t.Fatalf("offline Complete err = %v, want ErrNoProvider", err)
	}
	if res, err := r.Complete(context.Background(), Request{Prompt: "p"}, false); err != nil || res.Content != "from-remote" {
		t.Fatalf("online Complete = (%v, %v), want remote response", res, err)
	}
}

func TestRouterEmpty(t *testing.T) {
	r := NewRouter()
	if _, err := r.Complete(context.Background(), Request{Prompt: "p"}, false); !errors.Is(err, ErrNoProvider) {
		t.Fatalf("empty router err = %v, want ErrNoProvider", err)
	}
}

func TestFixtureReplaysExactMatch(t *testing.T) {
	f := NewFixture("fx", "qwen", true, Fixture{System: "s", Prompt: "p", Response: "resp"})
	res, err := f.Complete(context.Background(), Request{System: "s", Prompt: "p"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if res.Content != "resp" {
		t.Errorf("content = %q, want resp", res.Content)
	}
	if !f.Available(context.Background()) {
		t.Errorf("fixture provider must always be available")
	}

	if _, err := f.Complete(context.Background(), Request{Prompt: "nope"}); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("mismatch err = %v, want ErrNoMatch", err)
	}
}

func TestFixturePromptWildcard(t *testing.T) {
	f := NewFixture("fx", "qwen", true, Fixture{System: "s", Response: "any-prompt"})
	res, err := f.Complete(context.Background(), Request{System: "s", Prompt: "some dynamic prompt"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if res.Content != "any-prompt" {
		t.Errorf("content = %q, want any-prompt", res.Content)
	}
}

// roundTripFunc adapts a function to http.RoundTripper without sockets.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestClientRequestShape(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(`{
				"choices":[{"message":{"content":"reviewed"}}],
				"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}
			}`)),
		}, nil
	})

	c := newClient("http://localhost:11434", "qwen", "secret")
	c.hc = &http.Client{Transport: rt}

	res, err := c.Complete(context.Background(), Request{System: "be strict", Prompt: "review this", MaxTokens: 512, Temperature: 0.2})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if gotPath != "/v1/chat/completions" {
		t.Errorf("path = %q, want /v1/chat/completions", gotPath)
	}
	if gotAuth != "Bearer secret" {
		t.Errorf("auth = %q, want Bearer secret", gotAuth)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(gotBody), &sent); err != nil {
		t.Fatalf("sent body not json: %v", err)
	}
	if sent["model"] != "qwen" {
		t.Errorf("model = %v, want qwen", sent["model"])
	}
	if sent["max_tokens"] != float64(512) {
		t.Errorf("max_tokens = %v, want 512", sent["max_tokens"])
	}
	msgs, ok := sent["messages"].([]any)
	if !ok || len(msgs) != 2 {
		t.Fatalf("messages = %v, want 2 entries", sent["messages"])
	}
	sys := msgs[0].(map[string]any)
	user := msgs[1].(map[string]any)
	if sys["role"] != "system" || sys["content"] != "be strict" {
		t.Errorf("system message = %v", sys)
	}
	if user["role"] != "user" || user["content"] != "review this" {
		t.Errorf("user message = %v", user)
	}

	if res.Content != "reviewed" {
		t.Errorf("content = %q, want reviewed", res.Content)
	}
	if res.Usage.TotalTokens != 12 {
		t.Errorf("usage = %+v, want 12 total", res.Usage)
	}
}

func TestClientPing(t *testing.T) {
	for status, want := range map[int]bool{200: true, 401: true, 503: false} {
		status := status
		rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
		})
		c := newClient("http://localhost:11434", "m", "")
		c.hc = &http.Client{Transport: rt}
		if got := c.ping(context.Background()); got != want {
			t.Errorf("ping status %d = %v, want %v", status, got, want)
		}
	}

	failRT := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})
	c := newClient("http://localhost:11434", "m", "")
	c.hc = &http.Client{Transport: failRT}
	if c.ping(context.Background()) {
		t.Errorf("ping with refused connection = true, want false")
	}
}

func TestFromConfig(t *testing.T) {
	local := true
	ollama, err := FromConfig(config.Provider{Name: "ollama", Model: "qwen2.5-coder:7b", Local: &local})
	if err != nil {
		t.Fatalf("ollama: %v", err)
	}
	if !ollama.Local() {
		t.Errorf("ollama should be local")
	}

	remote := false
	openai, err := FromConfig(config.Provider{Name: "openai", Model: "gpt-4o", Local: &remote, BaseURL: "https://api.example.com/v1"})
	if err != nil {
		t.Fatalf("openai: %v", err)
	}
	if openai.Local() {
		t.Errorf("openai-compatible remote should not be local")
	}

	if _, err := FromConfig(config.Provider{Name: "nope", Model: "m"}); err == nil {
		t.Errorf("unknown provider should error")
	}
}
