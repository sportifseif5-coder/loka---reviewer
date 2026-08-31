package vcs

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// buildRepo creates a throwaway git repo with a committed baseline and the
// given working-tree modifications, then returns its path.
func buildRepo(t *testing.T, mutate func(dir string)) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init", "-q", "-b", "master")
	run("config", "user.name", "test")
	run("config", "user.email", "test@example.com")

	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("README.md", "# baseline\n")
	write("main.go", "package main\n\nfunc main() {}\n")
	run("add", ".")
	run("commit", "-q", "-m", "baseline")

	if mutate != nil {
		mutate(dir)
	}
	return dir
}

func TestIsRepo(t *testing.T) {
	ctx := context.Background()
	g := NewGit()
	dir := t.TempDir()
	t.Logf("isrepo temp dir = %s", dir)
	out, err := g.run(ctx, dir, "rev-parse", "--is-inside-work-tree")
	t.Logf("rev-parse out=%q err=%v", string(out), err)
	ok := g.IsRepo(ctx, dir)
	t.Logf("IsRepo=%v", ok)
	if ok {
		t.Fatal("temp dir must not be a repo")
	}
	repo := buildRepo(t, nil)
	if !g.IsRepo(ctx, repo) {
		t.Fatal("built repo should be recognized")
	}
}

func TestWorkingDiffModifiedAndUntracked(t *testing.T) {
	ctx := context.Background()
	repo := buildRepo(t, func(dir string) {
		p := filepath.Join(dir, "README.md")
		if err := os.WriteFile(p, []byte("# baseline\n\nextra line\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		untracked := filepath.Join(dir, "new.txt")
		if err := os.WriteFile(untracked, []byte("token = sk-test123456789\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	})

	g := NewGit()
	d, err := g.WorkingDiff(ctx, repo)
	if err != nil {
		t.Fatalf("WorkingDiff: %v", err)
	}

	if len(d.Changes) != 2 {
		t.Fatalf("got %d changes, want 2: %+v", len(d.Changes), d.Changes)
	}

	var readme, newTxt *Change
	for i := range d.Changes {
		switch d.Changes[i].Path {
		case "README.md":
			readme = &d.Changes[i]
		case "new.txt":
			newTxt = &d.Changes[i]
		}
	}
	if readme == nil {
		t.Fatal("README.md not in changes")
	}
	if readme.Status != StatusModified {
		t.Errorf("README.md status = %s, want modified", readme.Status)
	}
	if readme.Additions != 2 {
		t.Errorf("README.md additions = %d, want 2", readme.Additions)
	}

	if newTxt == nil {
		t.Fatal("new.txt not in changes")
	}
	if newTxt.Status != StatusAdded {
		t.Errorf("new.txt status = %s, want added", newTxt.Status)
	}
	diff, ok := d.Files["new.txt"]
	if !ok || diff == "" {
		t.Fatal("new.txt has no diff text")
	}
	if !contains(diff, "+token = sk-test123456789") {
		t.Errorf("untracked diff missing content:\n%s", diff)
	}
}

func TestWorkingDiffDeleted(t *testing.T) {
	ctx := context.Background()
	repo := buildRepo(t, func(dir string) {
		if err := os.Remove(filepath.Join(dir, "main.go")); err != nil {
			t.Fatal(err)
		}
	})

	g := NewGit()
	d, err := g.WorkingDiff(ctx, repo)
	if err != nil {
		t.Fatalf("WorkingDiff: %v", err)
	}
	if len(d.Changes) != 1 || d.Changes[0].Status != StatusDeleted {
		t.Fatalf("changes = %+v, want one deleted file", d.Changes)
	}
	if d.Changes[0].Deletions == 0 {
		t.Error("deleted file should report deletions")
	}
}

func TestWorkingDiffNotARepo(t *testing.T) {
	ctx := context.Background()
	g := NewGit()
	_, err := g.WorkingDiff(ctx, t.TempDir())
	if err != ErrNotRepository {
		t.Fatalf("err = %v, want ErrNotRepository", err)
	}
}

func TestBlame(t *testing.T) {
	ctx := context.Background()
	repo := buildRepo(t, func(dir string) {
		p := filepath.Join(dir, "main.go")
		_ = os.WriteFile(p, []byte("package main\n\nfunc main() { println(\"hi\") }\n"), 0o644)
	})
	g := NewGit()
	commit, author, err := g.Blame(ctx, repo, "main.go", 3)
	if err != nil {
		t.Fatalf("Blame: %v", err)
	}
	if commit == "" {
		t.Error("blame returned empty commit")
	}
	if author == "" {
		t.Error("blame returned empty author")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
