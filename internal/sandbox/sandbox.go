// Package sandbox enforces capability-scoped agent tool execution
// (architecture 8.3, constitution section 11). Default deny: no shell, no
// dotfile access, no network unless the mode grants a capability.
package sandbox

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

// Kind is one tool operation.
type Kind string

const (
	KindRead           Kind = "read"
	KindFetch          Kind = "fetch"
	KindShell          Kind = "shell"
	KindCompleteRemote Kind = "complete-remote"
	KindWrite          Kind = "write"
)

// Capability is a named grant.
type Capability string

const (
	CapFetch  Capability = "cap.fetch"
	CapRemote Capability = "cap.remote"
)

// Op is one attempted tool call.
type Op struct {
	Kind Kind
	Path string
	URL  string
}

// Policy is a capability set plus deny rules.
type Policy struct {
	caps map[Capability]bool
}

// Default is offline-safe: no network, no shell, no dotfiles.
func Default() Policy {
	return Policy{caps: map[Capability]bool{}}
}

// With returns a copy that grants extra capabilities.
func (p Policy) With(cs ...Capability) Policy {
	caps := map[Capability]bool{}
	for c, ok := range p.caps {
		if ok {
			caps[c] = true
		}
	}
	for _, c := range cs {
		caps[c] = true
	}
	return Policy{caps: caps}
}

// ForMode maps a repository mode onto a policy (I6).
func ForMode(mode string) Policy {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "agent":
		return Default().With(CapFetch, CapRemote)
	case "online":
		return Default().With(CapRemote)
	default:
		return Default()
	}
}

// Allow reports whether op is permitted.
func (p Policy) Allow(op Op) error {
	switch op.Kind {
	case KindShell:
		return fmt.Errorf("sandbox deny: shell is not a granted capability")
	case KindWrite:
		return fmt.Errorf("sandbox deny: write is not a granted capability")
	case KindFetch:
		if !p.caps[CapFetch] {
			return fmt.Errorf("sandbox deny: fetch requires cap.fetch")
		}
		return nil
	case KindCompleteRemote:
		if !p.caps[CapRemote] {
			return fmt.Errorf("sandbox deny: remote complete requires cap.remote")
		}
		return nil
	case KindRead:
		if isDotfile(op.Path) {
			return fmt.Errorf("sandbox deny: dotfile access %s", op.Path)
		}
		return nil
	}
	return fmt.Errorf("sandbox deny: unknown op %s", op.Kind)
}

func isDotfile(p string) bool {
	if p == "" {
		return false
	}
	cleaned := filepath.ToSlash(p)
	for _, part := range strings.Split(cleaned, "/") {
		if strings.HasPrefix(part, ".") && part != "." && part != ".." {
			return true
		}
	}
	return false
}

// Guard records denied operations for the review audit log.
type Guard struct {
	policy Policy
	mu     sync.Mutex
	denied []string
}

// NewGuard wraps a policy.
func NewGuard(p Policy) *Guard {
	return &Guard{policy: p}
}

// Check applies the policy.
func (g *Guard) Check(ctx context.Context, op Op) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := g.policy.Allow(op); err != nil {
		g.mu.Lock()
		g.denied = append(g.denied, err.Error())
		g.mu.Unlock()
		return err
	}
	return nil
}

// Denied returns a copy of the deny log.
func (g *Guard) Denied() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]string, len(g.denied))
	copy(out, g.denied)
	return out
}
