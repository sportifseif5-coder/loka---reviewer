package secrets

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMemoryStoreRoundTrip(t *testing.T) {
	s := NewMemoryStore()
	if err := s.Set(context.Background(), "loka", "openai", "sk-test"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get(context.Background(), "loka", "openai")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "sk-test" {
		t.Fatalf("Get = %q, want sk-test", got)
	}
}

func TestMemoryStoreMissing(t *testing.T) {
	s := NewMemoryStore()
	_, err := s.Get(context.Background(), "loka", "missing")
	if err == nil {
		t.Fatal("expected error for missing secret")
	}
}

func TestFileStoreRoundTrip0600(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenFileStore(dir)
	if err != nil {
		t.Fatalf("OpenFileStore: %v", err)
	}
	if err := s.Set(context.Background(), "loka", "openai", "sk-file"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get(context.Background(), "loka", "openai")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "sk-file" {
		t.Fatalf("Get = %q, want sk-file", got)
	}
	info, err := os.Stat(filepath.Join(dir, "loka", "openai"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %o, want 0600", info.Mode().Perm())
	}
}

func TestResolvePrefersKeyringOverEnv(t *testing.T) {
	mem := NewMemoryStore()
	if err := mem.Set(context.Background(), "loka", "openai", "from-keyring"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("USER_LLM_API_KEY", "from-env")
	got, src, err := Resolve(context.Background(), mem, Lookup{
		Service: "loka",
		Account: "openai",
		EnvVar:  "USER_LLM_API_KEY",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "from-keyring" {
		t.Fatalf("secret = %q, want from-keyring", got)
	}
	if src != SourceKeyring {
		t.Fatalf("source = %q, want keyring", src)
	}
}

func TestResolveFallsBackToEnv(t *testing.T) {
	t.Setenv("USER_LLM_API_KEY", "from-env")
	got, src, err := Resolve(context.Background(), NewMemoryStore(), Lookup{
		EnvVar: "USER_LLM_API_KEY",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "from-env" {
		t.Fatalf("secret = %q, want from-env", got)
	}
	if src != SourceEnv {
		t.Fatalf("source = %q, want env", src)
	}
}

func TestResolveEmptyWhenUnset(t *testing.T) {
	got, src, err := Resolve(context.Background(), NewMemoryStore(), Lookup{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "" || src != SourceNone {
		t.Fatalf("got (%q, %q), want empty/none", got, src)
	}
}

func TestParseKeyringRef(t *testing.T) {
	svc, acct, err := ParseKeyringRef("loka/openai")
	if err != nil {
		t.Fatalf("ParseKeyringRef: %v", err)
	}
	if svc != "loka" || acct != "openai" {
		t.Fatalf("got %q/%q, want loka/openai", svc, acct)
	}
	if _, _, err := ParseKeyringRef("nslash"); err == nil {
		t.Fatal("expected error for missing slash")
	}
	if _, _, err := ParseKeyringRef(""); err == nil {
		t.Fatal("expected error for empty ref")
	}
}
