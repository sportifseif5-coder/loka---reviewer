package indexer

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sportifseif5-coder/loka---reviewer/internal/store"
)

const demoA = `package demo

const limit = 10

var count = 0

type Store struct {
	Name string
}

type Reader interface {
	Read([]byte) (int, error)
}

func New() *Store {
	return &Store{}
}

var seed = New()

func (s *Store) Save() {
	count++
}

func resolve() int {
	return limit
}

func recurse() {
	recurse()
}
`

const demoB = `package demo

import "fmt"

type Cache struct {
	Store
	tag string
}

func describe() string {
	s := New()
	if s != nil {
		s.Save()
	}
	total := count
	fmt.Println(total)
	return fmt.Sprintf("n=%d", total)
}

func pick() int {
	resolve := 1
	_ = resolve
	return resolve
}

func call() int {
	return resolve()
}

func helper(err error) int {
	if err != nil {
		return limit
	}
	return 0
}
`

const subX = `package sub

func A() {}

func B() {
	A()
}
`

const brokenDir = `package demo

func Oops( {
`

// writeRepo materializes a fixture tree and returns its root path.
func writeRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func demoRepo(t *testing.T) string {
	return writeRepo(t, map[string]string{
		"a.go":     demoA,
		"b.go":     demoB,
		"sub/x.go": subX,
	})
}

func refKey(r store.IndexRef) string {
	return strings.Join([]string{r.SrcFile, r.SrcName, r.DstFile, r.DstName, r.Kind}, "\x00")
}

func TestIndexDirExtraction(t *testing.T) {
	root := demoRepo(t)
	files := []indexedFile{
		{rel: "a.go", abs: filepath.Join(root, "a.go"), lang: "go", hash: "h1"},
		{rel: "b.go", abs: filepath.Join(root, "b.go"), lang: "go", hash: "h2"},
	}
	di, skipped := indexGoDir("", files)
	if len(skipped) != 0 {
		t.Fatalf("unexpected skipped files: %v", skipped)
	}
	if len(di.Files) != 2 {
		t.Fatalf("files: got %d want 2", len(di.Files))
	}

	symbols := map[string]bool{}
	kinds := map[string]string{}
	for _, f := range di.Files {
		for _, s := range f.Symbols {
			symbols[f.Path+":"+s.Name] = true
			kinds[s.Name] = s.Kind
		}
	}
	for _, want := range []string{"a.go:limit", "a.go:count", "a.go:Store",
		"a.go:Reader", "a.go:New", "a.go:seed", "a.go:Store.Save",
		"a.go:resolve", "a.go:recurse", "b.go:Cache", "b.go:describe",
		"b.go:pick", "b.go:call", "b.go:helper"} {
		if !symbols[want] {
			t.Errorf("missing symbol %q", want)
		}
	}
	if kinds["Store"] != KindStruct || kinds["Reader"] != KindInterface {
		t.Errorf("type kinds wrong: Store=%q Reader=%q", kinds["Store"], kinds["Reader"])
	}
	if kinds["Store.Save"] != KindMethod {
		t.Errorf("method kind wrong: %q", kinds["Store.Save"])
	}

	if len(di.Refs) != 10 {
		t.Errorf("refs: got %d want 10", len(di.Refs))
		for _, r := range di.Refs {
			t.Logf("  ref %s.%s -> %s.%s (%s)", r.SrcFile, r.SrcName, r.DstFile, r.DstName, r.Kind)
		}
	}
	wantRefs := map[string]bool{
		refKey(store.IndexRef{SrcFile: "a.go", SrcName: "New", DstFile: "a.go", DstName: "Store", Kind: RefUse}):        true,
		refKey(store.IndexRef{SrcFile: "a.go", SrcName: "seed", DstFile: "a.go", DstName: "New", Kind: RefCall}):        true,
		refKey(store.IndexRef{SrcFile: "a.go", SrcName: "Store.Save", DstFile: "a.go", DstName: "Store", Kind: RefUse}): true,
		refKey(store.IndexRef{SrcFile: "a.go", SrcName: "Store.Save", DstFile: "a.go", DstName: "count", Kind: RefUse}): true,
		refKey(store.IndexRef{SrcFile: "a.go", SrcName: "resolve", DstFile: "a.go", DstName: "limit", Kind: RefUse}):    true,
		refKey(store.IndexRef{SrcFile: "b.go", SrcName: "Cache", DstFile: "a.go", DstName: "Store", Kind: RefUse}):      true,
		refKey(store.IndexRef{SrcFile: "b.go", SrcName: "describe", DstFile: "a.go", DstName: "New", Kind: RefCall}):    true,
		refKey(store.IndexRef{SrcFile: "b.go", SrcName: "describe", DstFile: "a.go", DstName: "count", Kind: RefUse}):   true,
		refKey(store.IndexRef{SrcFile: "b.go", SrcName: "call", DstFile: "a.go", DstName: "resolve", Kind: RefCall}):    true,
		refKey(store.IndexRef{SrcFile: "b.go", SrcName: "helper", DstFile: "a.go", DstName: "limit", Kind: RefUse}):     true,
	}
	got := map[string]bool{}
	for _, r := range di.Refs {
		got[refKey(r)] = true
	}
	if !reflect.DeepEqual(got, wantRefs) {
		t.Errorf("ref set mismatch")
		for _, r := range di.Refs {
			t.Logf("  got %s.%s -> %s.%s (%s)", r.SrcFile, r.SrcName, r.DstFile, r.DstName, r.Kind)
		}
	}
}

// TestIndexDirDeterminism ensures identical input yields identical rows.
func TestIndexDirDeterminism(t *testing.T) {
	root := demoRepo(t)
	rel, _ := filepath.Rel(root, filepath.Join(root, "a.go"))
	files := func() []indexedFile {
		return []indexedFile{
			{rel: rel, abs: filepath.Join(root, rel), lang: "go", hash: "h1"},
		}
	}
	a, _ := indexGoDir("", files())
	b, _ := indexGoDir("", files())
	if !reflect.DeepEqual(a, b) {
		t.Error("two runs of indexGoDir differ")
	}
}

func TestIndexGoDirSkipsBrokenFile(t *testing.T) {
	root := writeRepo(t, map[string]string{"broken.go": brokenDir})
	di, skipped := indexGoDir("", []indexedFile{
		{rel: "broken.go", abs: filepath.Join(root, "broken.go"), lang: "go", hash: "h"},
	})
	if len(skipped) != 1 || skipped[0] != "broken.go" {
		t.Fatalf("skipped = %v, want [broken.go]", skipped)
	}
	if len(di.Files) != 0 || len(di.Refs) != 0 {
		t.Fatalf("broken file must not be indexed: %+v", di)
	}
}

func TestSyncIncremental(t *testing.T) {
	root := demoRepo(t)
	s, err := store.Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ix := New(s)
	ctx := context.Background()

	first, err := ix.Sync(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if first.DirsChanged != 2 || first.Files != 3 || first.Symbols != 16 || first.Refs != 11 {
		t.Fatalf("first sync summary = %+v", first)
	}

	second, err := ix.Sync(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if second.DirsChanged != 0 || second.Files != 0 {
		t.Fatalf("unchanged resync must be a no-op, got %+v", second)
	}

	// Touch one file in the demo directory.
	p := filepath.Join(root, "a.go")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, append(data, []byte("\n// touched\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	third, err := ix.Sync(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if third.DirsChanged != 1 || third.Files != 2 {
		t.Fatalf("touched-dir sync summary = %+v", third)
	}

	// Delete the whole sub package directory file.
	if err := os.Remove(filepath.Join(root, "sub", "x.go")); err != nil {
		t.Fatal(err)
	}
	fourth, err := ix.Sync(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if fourth.DirsChanged != 1 || fourth.Files != 0 {
		t.Fatalf("emptied-dir sync summary = %+v", fourth)
	}
	sub, err := s.DirIndex(ctx, root, "sub")
	if err != nil {
		t.Fatal(err)
	}
	if len(sub.Files) != 0 || len(sub.Refs) != 0 {
		t.Fatalf("emptied dir must be deleted, got %+v", sub)
	}
}

func TestSyncPersistsDirIndex(t *testing.T) {
	root := demoRepo(t)
	s, err := store.Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := New(s).Sync(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	di, err := s.DirIndex(context.Background(), root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(di.Files) != 2 {
		t.Fatalf("dir files = %d, want 2", len(di.Files))
	}
	var bImports []string
	for _, f := range di.Files {
		if f.Path == "b.go" {
			bImports = append(bImports, f.Imports...)
		}
	}
	if len(bImports) != 1 || bImports[0] != "fmt" {
		t.Errorf("b.go imports = %v, want [fmt]", bImports)
	}
	if len(di.Refs) != 10 {
		t.Errorf("dir refs = %d, want 10", len(di.Refs))
	}
}

// TestSyncDryRun ensures a nil store still produces a deterministic summary.
func TestSyncDryRun(t *testing.T) {
	root := demoRepo(t)
	ix := New(nil)
	sum, err := ix.Sync(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Files != 3 || sum.Symbols != 16 || sum.Refs != 11 {
		t.Fatalf("dry-run summary = %+v", sum)
	}
}
