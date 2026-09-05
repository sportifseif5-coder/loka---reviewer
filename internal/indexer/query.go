package indexer

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/sportifseif5-coder/loka---reviewer/internal/store"
)

// Symbol names one declaration in the index. Because refs are stored per
// package directory and a name may repeat across packages, a symbol is always
// addressed by the file that declares it plus its name (methods are qualified
// by receiver base type, e.g. "Store.Save").
type Symbol struct {
	File string
	Name string
}

// Ref is one stored reference involving a queried symbol: the other endpoint
// of the edge (the referencing symbol for Callers/TypeUses, the referenced
// symbol for Callees) together with the edge kind (RefCall or RefUse).
type Ref struct {
	File string
	Name string
	Kind string
}

// ImpactFile is one file of an impact set: a file whose symbols are reachable
// from a changed file along reference edges. Distance is the minimum number of
// symbol-edge hops from any changed file (0 = the changed file itself).
type ImpactFile struct {
	Path     string
	Distance int
}

// impactMaxDepth bounds how far ImpactSet walks from the changed files so a
// fully connected package never floods a context pack.
const impactMaxDepth = 2

// requireStore guards the query methods against a dry-run Indexer (nil store).
func (ix *Indexer) requireStore() (*store.Store, error) {
	if ix.store == nil {
		return nil, fmt.Errorf("index query requires a store")
	}
	return ix.store, nil
}

// Callers returns the symbols that reference sym (edges whose destination is
// the declaration). Results are deduplicated and ordered by file, name, kind.
func (ix *Indexer) Callers(ctx context.Context, repoPath string, sym Symbol) ([]Ref, error) {
	s, err := ix.requireStore()
	if err != nil {
		return nil, err
	}
	edges, err := s.RefsTo(ctx, repoPath, sym.File, sym.Name)
	if err != nil {
		return nil, err
	}
	out := make([]Ref, 0, len(edges))
	for _, e := range edges {
		out = append(out, Ref{File: e.SrcFile, Name: e.SrcName, Kind: e.Kind})
	}
	return dedupeRefs(out), nil
}

// Callees returns the symbols sym references (edges whose source is the
// declaration). Results are deduplicated and ordered by file, name, kind.
func (ix *Indexer) Callees(ctx context.Context, repoPath string, sym Symbol) ([]Ref, error) {
	s, err := ix.requireStore()
	if err != nil {
		return nil, err
	}
	edges, err := s.RefsFrom(ctx, repoPath, sym.File, sym.Name)
	if err != nil {
		return nil, err
	}
	out := make([]Ref, 0, len(edges))
	for _, e := range edges {
		out = append(out, Ref{File: e.DstFile, Name: e.DstName, Kind: e.Kind})
	}
	return dedupeRefs(out), nil
}

// TypeUses returns every reference to a type declaration typ. A type is only
// referenced in use positions (never called), so this is Callers for a symbol
// whose kind is type/struct/interface; it is a distinct name so the intent is
// explicit at the query surface.
func (ix *Indexer) TypeUses(ctx context.Context, repoPath string, typ Symbol) ([]Ref, error) {
	s, err := ix.requireStore()
	if err != nil {
		return nil, err
	}
	kind, ok, err := s.SymbolKind(ctx, repoPath, typ.File, typ.Name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("type %s.%s not in index", typ.File, typ.Name)
	}
	switch kind {
	case KindType, KindStruct, KindInterface:
	default:
		return nil, fmt.Errorf("%s.%s is a %s, not a type", typ.File, typ.Name, kind)
	}
	return ix.Callers(ctx, repoPath, typ)
}

// Imports returns the imports of one indexed file, sorted.
func (ix *Indexer) Imports(ctx context.Context, repoPath, file string) ([]string, error) {
	s, err := ix.requireStore()
	if err != nil {
		return nil, err
	}
	return s.FileImports(ctx, repoPath, file)
}

// FilesTouching returns the files that declare or reference sym, sorted.
// The declaring file is always included so callers can open the definition
// even when nothing references the symbol yet.
func (ix *Indexer) FilesTouching(ctx context.Context, repoPath string, sym Symbol) ([]string, error) {
	refs, err := ix.Callers(ctx, repoPath, sym)
	if err != nil {
		return nil, err
	}
	set := map[string]bool{sym.File: true}
	for _, r := range refs {
		set[r.File] = true
	}
	out := make([]string, 0, len(set))
	for f := range set {
		out = append(out, f)
	}
	sort.Strings(out)
	return out, nil
}

// ImpactSet computes the files reachable from the changed files along
// reference edges (architecture section 5.3): the changed files at distance 0,
// then files whose symbols reference changed symbols and files changed symbols
// reference, walking up to impactMaxDepth hops. Results are ordered by
// distance then path. Callers can drop the distance-0 files (the diff itself)
// when only impact beyond the diff is wanted.
func (ix *Indexer) ImpactSet(ctx context.Context, repoPath string, changed []string) ([]ImpactFile, error) {
	s, err := ix.requireStore()
	if err != nil {
		return nil, err
	}
	snap, err := s.RepoIndex(ctx, repoPath)
	if err != nil {
		return nil, err
	}

	// File-level adjacency derived from symbol edges; refs never leave their
	// package directory, so no file connects to another package's files.
	adj := map[string]map[string]bool{}
	for _, r := range snap.Refs {
		if r.SrcFile == r.DstFile {
			continue
		}
		if adj[r.SrcFile] == nil {
			adj[r.SrcFile] = map[string]bool{}
		}
		if adj[r.DstFile] == nil {
			adj[r.DstFile] = map[string]bool{}
		}
		adj[r.SrcFile][r.DstFile] = true
		adj[r.DstFile][r.SrcFile] = true
	}

	dist := map[string]int{}
	changedSet := map[string]bool{}
	for _, c := range changed {
		p := store.CleanPath(c)
		if p == "" || p == "." || p == "/" {
			continue
		}
		changedSet[p] = true
		if _, ok := adj[p]; ok {
			dist[p] = 0
		}
	}
	// New or removed files carry no edges but are still part of the diff.
	for p := range changedSet {
		if _, ok := dist[p]; !ok {
			dist[p] = 0
		}
	}

	// Breadth-first expansion over the adjacency graph.
	frontier := make([]string, 0, len(dist))
	for p := range dist {
		frontier = append(frontier, p)
	}
	sort.Strings(frontier)
	for depth := 1; depth <= impactMaxDepth; depth++ {
		var next []string
		for _, p := range frontier {
			if dist[p] != depth-1 {
				continue
			}
			for n := range adj[p] {
				if _, seen := dist[n]; seen {
					continue
				}
				dist[n] = depth
				next = append(next, n)
			}
		}
		sort.Strings(next)
		frontier = append(frontier, next...)
	}

	out := make([]ImpactFile, 0, len(dist))
	for p, d := range dist {
		out = append(out, ImpactFile{Path: p, Distance: d})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Distance != out[j].Distance {
			return out[i].Distance < out[j].Distance
		}
		return out[i].Path < out[j].Path
	})
	return out, nil
}

// dedupeRefs removes duplicate (file, name, kind) entries while preserving the
// deterministic order already established by the store query.
func dedupeRefs(refs []Ref) []Ref {
	seen := map[string]bool{}
	out := make([]Ref, 0, len(refs))
	for _, r := range refs {
		key := strings.Join([]string{r.File, r.Name, r.Kind}, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	return out
}
