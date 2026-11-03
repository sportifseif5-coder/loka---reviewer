package sandbox

import (
	"context"
	"strings"
	"testing"
)

func TestDefaultPolicyDeniesShellAndDotfiles(t *testing.T) {
	p := Default()
	if err := p.Allow(Op{Kind: KindShell, Path: "bash"}); err == nil {
		t.Fatal("shell must be denied")
	}
	if err := p.Allow(Op{Kind: KindRead, Path: "/root/.bashrc"}); err == nil {
		t.Fatal("dotfile read must be denied")
	}
	if err := p.Allow(Op{Kind: KindRead, Path: "/home/user/.ssh/id_rsa"}); err == nil {
		t.Fatal("ssh key read must be denied")
	}
}

func TestFetchAllowedOnlyWithCapability(t *testing.T) {
	p := Default()
	if err := p.Allow(Op{Kind: KindFetch, URL: "https://example.com"}); err == nil {
		t.Fatal("fetch without cap.fetch must be denied")
	}
	p = p.With(CapFetch)
	if err := p.Allow(Op{Kind: KindFetch, URL: "https://example.com/cve"}); err != nil {
		t.Fatalf("fetch with cap: %v", err)
	}
}

func TestOfflinePolicyForbidsEveryNetworkOp(t *testing.T) {
	p := ForMode("offline")
	if err := p.Allow(Op{Kind: KindFetch, URL: "https://example.com"}); err == nil {
		t.Fatal("offline fetch must be denied")
	}
	if err := p.Allow(Op{Kind: KindCompleteRemote, URL: "https://api.openai.com"}); err == nil {
		t.Fatal("offline remote complete must be denied")
	}
}

func TestAgentModeGrantsFetchNotShell(t *testing.T) {
	p := ForMode("agent")
	if err := p.Allow(Op{Kind: KindFetch, URL: "https://en.wikipedia.org/w/api.php"}); err != nil {
		t.Fatalf("agent fetch: %v", err)
	}
	if err := p.Allow(Op{Kind: KindShell, Path: "/bin/sh"}); err == nil {
		t.Fatal("agent mode still denies shell")
	}
}

func TestGuardRecordsDeniedOps(t *testing.T) {
	g := NewGuard(Default())
	if err := g.Check(context.Background(), Op{Kind: KindShell, Path: "rm"}); err == nil {
		t.Fatal("expected deny")
	}
	if len(g.Denied()) == 0 {
		t.Fatal("denied log empty")
	}
	if !strings.Contains(g.Denied()[0], "shell") {
		t.Errorf("denied = %v", g.Denied())
	}
}
