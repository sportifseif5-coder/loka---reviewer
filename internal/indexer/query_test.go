package indexer

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/sportifseif5-coder/loka---reviewer/internal/store"
)

// syncedDemo indexes demoRepo into an in-memory store and returns the repo
// root plus the indexer.
func syncedDemo(t *testing.T) (string, *Indexer) {
	t.Helper()
	root := demoRepo(t)
	s, err := store.Open("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	ix := New(s)
	if _, err := ix.Sync(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	return root, ix
}

func refSlice(refs []Ref) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.File+"\x00"+r.Name+"\x00"+r.Kind)
	}
	return out
}

func TestQueryCallersCalleesTypeUses(t *testing.T) {
	root, ix := syncedDemo(t)
	ctx := context.Background()

	// Callers of New in a.go: seed (a.go) and describe (b.go), both calls.
	callers, err := ix.Callers(ctx, root, Symbol{File: "a.go", Name: "New"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"a.go\x00seed\x00call",
		"b.go\x00describe\x00call",
	}
	if got := refSlice(callers); !reflect.DeepEqual(got, want) {
		t.Errorf("Callers(New) = %v, want %v", got, want)
	}

	// Callees of describe in b.go: New (call) and count (use).
	callees, err := ix.Callees(ctx, root, Symbol{File: "b.go", Name: "describe"})
	if err != nil {
		t.Fatal(err)
	}
	// The store orders by (src file, src name, dst file, dst name, kind), so
	// within a.go "New" sorts before "count".
	want = []string{
		"a.go\x00New\x00call",
		"a.go\x00count\x00use",
	}
	if got := refSlice(callees); !reflect.DeepEqual(got, want) {
		t.Errorf("Callees(describe) = %v, want %v", got, want)
	}

	// TypeUses of the Store struct: New's return type (use), Cache embedding
	// (use), and the method receiver Store.Save (use).
	uses, err := ix.TypeUses(ctx, root, Symbol{File: "a.go", Name: "Store"})
	if err != nil {
		t.Fatal(err)
	}
	want = []string{
		"a.go\x00New\x00use",
		"a.go\x00Store.Save\x00use",
		"b.go\x00Cache\x00use",
	}
	if got := refSlice(uses); !reflect.DeepEqual(got, want) {
		t.Errorf("TypeUses(Store) = %v, want %v", got, want)
	}

	// A method symbol is callable: call() in b.go calls resolve in a.go.
	callers, err = ix.Callers(ctx, root, Symbol{File: "a.go", Name: "resolve"})
	if err != nil {
		t.Fatal(err)
	}
	want = []string{"b.go\x00call\x00call"}
	if got := refSlice(callers); !reflect.DeepEqual(got, want) {
		t.Errorf("Callers(resolve) = %v, want %v", got, want)
	}
}

func TestQueryTypeUsesRejectsNonType(t *testing.T) {
	root, ix := syncedDemo(t)
	_, err := ix.TypeUses(context.Background(), root, Symbol{File: "a.go", Name: "New"})
	if err == nil {
		t.Error("TypeUses of a func must fail")
	}
}

func TestQueryImports(t *testing.T) {
	root, ix := syncedDemo(t)
	ctx := context.Background()
	got, err := ix.Imports(ctx, root, "b.go")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"fmt"}) {
		t.Errorf("Imports(b.go) = %v, want [fmt]", got)
	}
	empty, err := ix.Imports(ctx, root, "a.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Errorf("Imports(a.go) = %v, want []", empty)
	}
}

func TestQueryFilesTouching(t *testing.T) {
	root, ix := syncedDemo(t)
	got, err := ix.FilesTouching(context.Background(), root, Symbol{File: "a.go", Name: "New"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.go", "b.go"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FilesTouching(New) = %v, want %v", got, want)
	}
	// A symbol referenced only from its own file still returns the declarer.
	got, err = ix.FilesTouching(context.Background(), root, Symbol{File: "sub/x.go", Name: "B"})
	if err != nil {
		t.Fatal(err)
	}
	want = []string{"sub/x.go"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FilesTouching(B) = %v, want %v", got, want)
	}
}

func TestQueryImpactSet(t *testing.T) {
	root, ix := syncedDemo(t)
	ctx := context.Background()

	// Changing a.go touches describe/seed etc. -> b.go at distance 1.
	got, err := ix.ImpactSet(ctx, root, []string{"a.go"})
	if err != nil {
		t.Fatal(err)
	}
	want := []ImpactFile{{Path: "a.go", Distance: 0}, {Path: "b.go", Distance: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ImpactSet(a.go) = %+v, want %+v", got, want)
	}

	// Changing only sub/x.go stays within that package (no cross-package refs
	// at v1), so the file itself is distance 0 and nothing else appears.
	got, err = ix.ImpactSet(ctx, root, []string{"sub/x.go"})
	if err != nil {
		t.Fatal(err)
	}
	want = []ImpactFile{{Path: "sub/x.go", Distance: 0}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ImpactSet(sub/x.go) = %+v, want %+v", got, want)
	}

	// Multi-file diff: ordering is by distance then path.
	got, err = ix.ImpactSet(ctx, root, []string{"sub/x.go", "b.go"})
	if err != nil {
		t.Fatal(err)
	}
	want = []ImpactFile{
		{Path: "b.go", Distance: 0},
		{Path: "sub/x.go", Distance: 0},
		{Path: "a.go", Distance: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ImpactSet(b.go, sub/x.go) = %+v, want %+v", got, want)
	}
}

func TestQueryDeterminism(t *testing.T) {
	root, ix := syncedDemo(t)
	ctx := context.Background()
	a, err := ix.ImpactSet(ctx, root, []string{"a.go", "b.go"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := ix.ImpactSet(ctx, root, []string{"b.go", "a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Errorf("impact set is not deterministic on input order:\n%+v\n%+v", a, b)
	}
}

func TestQueryNilStoreErrors(t *testing.T) {
	ix := New(nil)
	ctx := context.Background()
	if _, err := ix.Callers(ctx, "/x", Symbol{}); err == nil {
		t.Error("Callers on nil store must error")
	}
	if _, err := ix.ImpactSet(ctx, "/x", []string{"a.go"}); err == nil {
		t.Error("ImpactSet on nil store must error")
	}
}

func TestQueryRefsDeterministicOrder(t *testing.T) {
	root, ix := syncedDemo(t)
	ctx := context.Background()
	// All callers of count are stored in deterministic (file, name, kind)
	// order regardless of insertion order in the refs.
	callers, err := ix.Callers(ctx, root, Symbol{File: "a.go", Name: "count"})
	if err != nil {
		t.Fatal(err)
	}
	if !sort.StringsAreSorted(refSlice(callers)) {
		t.Errorf("callers not sorted: %v", refSlice(callers))
	}
}
