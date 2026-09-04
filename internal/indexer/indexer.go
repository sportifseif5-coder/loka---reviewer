// Package indexer builds and queries the lightweight per-repository graph
// index (ADR-0004): files, symbols, and intra-package reference edges,
// stored in SQLite and updated incrementally. The v1 backend parses Go with
// the standard library (go/parser + go/ast) so indexing is fully offline and
// deterministic; other languages are added behind the same per-file symbol
// extraction seam without changing the storage or query surface.
package indexer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sportifseif5-coder/loka---reviewer/internal/store"
)

// Indexer walks a repository, extracts per-language symbols and references,
// and persists them per package directory. Sync is incremental: a directory
// is re-parsed only when one of its files was added, removed, or changed.
type Indexer struct {
	store *store.Store
	// skipDirs contains directory base names that are never indexed.
	skipDirs map[string]bool
}

// New returns an Indexer persisting into s. The caller owns the store's
// lifecycle. A nil store makes Sync compute a summary without persisting,
// which is useful for dry-run/dogfooding assertions.
func New(s *store.Store) *Indexer {
	return &Indexer{
		store: s,
		skipDirs: map[string]bool{
			".git":         true,
			".codegraph":   true,
			"node_modules": true,
			"vendor":       true,
			"dist":         true,
			"build":        true,
		},
	}
}

// SkipDirs replaces the default set of directory base names to skip.
func (ix *Indexer) SkipDirs(names ...string) {
	ix.skipDirs = make(map[string]bool, len(names))
	for _, n := range names {
		ix.skipDirs[n] = true
	}
}

// Summary reports what a Sync pass did.
type Summary struct {
	// Files is the number of file rows written for changed directories.
	Files int
	// Symbols and Refs count what was extracted across indexed directories.
	Symbols int
	Refs    int
	// DirsChanged is the number of package directories re-parsed.
	DirsChanged int
	// Skipped lists files that failed to parse and were left unindexed.
	Skipped []string
}

// indexedFile is a discovered candidate file.
type indexedFile struct {
	rel  string
	abs  string
	lang string
	hash string
}

// Sync re-indexes repoPath. Directories whose file set or any file hash
// changed are re-parsed wholesale (a Go package is one directory, so its
// symbol/ref rows must stay internally consistent); everything else is left
// untouched. Deterministic: the same snapshot produces the same rows.
func (ix *Indexer) Sync(ctx context.Context, repoPath string) (Summary, error) {
	var sum Summary
	candidates, err := ix.discover(repoPath)
	if err != nil {
		return sum, err
	}

	// Stored hashes tell us what changed since the last pass.
	stored := map[string]string{}
	if ix.store != nil {
		if stored, err = ix.store.IndexHashes(ctx, repoPath); err != nil {
			return sum, fmt.Errorf("load stored hashes: %w", err)
		}
	}

	now := map[string]string{}
	for _, c := range candidates {
		now[c.rel] = c.hash
	}

	// A directory is stale if its file->hash mapping differs from storage in
	// any way (added, removed, or changed file).
	stale := map[string]bool{}
	for rel, h := range now {
		if stored[rel] != h {
			stale[store.DirOf(rel)] = true
		}
	}
	for rel := range stored {
		if _, ok := now[rel]; !ok {
			stale[store.DirOf(rel)] = true
		}
	}

	dirs := make([]string, 0, len(stale))
	for d := range stale {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)

	byDir := map[string][]indexedFile{}
	for _, c := range candidates {
		byDir[store.DirOf(c.rel)] = append(byDir[store.DirOf(c.rel)], c)
	}

	for _, dir := range dirs {
		files := byDir[dir]
		if len(files) == 0 {
			if ix.store != nil {
				if err := ix.store.DeleteDirIndex(ctx, repoPath, dir); err != nil {
					return sum, fmt.Errorf("delete empty dir %q: %w", dir, err)
				}
			}
			sum.DirsChanged++
			continue
		}
		di, skipped := ix.indexDir(dir, files)
		sum.Skipped = append(sum.Skipped, skipped...)
		if ix.store != nil {
			if err := ix.store.ReplaceDirIndex(ctx, repoPath, dir, di.Files, di.Refs); err != nil {
				return sum, fmt.Errorf("persist dir %q: %w", dir, err)
			}
		}
		sum.DirsChanged++
		sum.Files += len(di.Files)
		for _, f := range di.Files {
			sum.Symbols += len(f.Symbols)
		}
		sum.Refs += len(di.Refs)
	}
	sort.Strings(sum.Skipped)
	return sum, nil
}

// discover walks repoPath and returns every parseable source file.
func (ix *Indexer) discover(repoPath string) ([]indexedFile, error) {
	var out []indexedFile
	err := filepath.WalkDir(repoPath, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != repoPath && ix.skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		lang, ok := languageFor(d.Name())
		if !ok {
			return nil
		}
		rel, err := filepath.Rel(repoPath, p)
		if err != nil {
			return err
		}
		rel = store.CleanPath(rel)
		h, err := fileHash(p)
		if err != nil {
			return err
		}
		out = append(out, indexedFile{rel: rel, abs: p, lang: lang, hash: h})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", repoPath, err)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].rel < out[j].rel })
	return out, nil
}

func fileHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// languageFor maps a file name to an index language by extension. v1 ships
// Go only; new languages are added here and given an extractor in
// indexDir.
func languageFor(name string) (string, bool) {
	switch {
	case strings.HasSuffix(name, ".go"):
		return "go", true
	}
	return "", false
}

// indexDir parses every file of a package directory and produces its index
// rows. Parse failures are skipped (their stale rows are removed by the
// caller's replace) and reported, keeping indexing deterministic and cheap.
func (ix *Indexer) indexDir(dir string, files []indexedFile) (*store.DirIndex, []string) {
	di, skipped := indexGoDir(dir, files)
	return di, skipped
}
