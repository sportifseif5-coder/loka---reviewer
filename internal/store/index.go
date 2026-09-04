package store

import (
	"context"
	"database/sql"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"
)

// IndexSymbol is one declared symbol inside an indexed file.
type IndexSymbol struct {
	Name string
	Kind string
	Line int
}

// IndexFile is an indexed file together with its declarations and imports.
type IndexFile struct {
	Path     string
	Language string
	Hash     string
	Symbols  []IndexSymbol
	Imports  []string
}

// IndexRef is a reference edge from a source symbol to a destination symbol.
// Both symbols live in the same package directory; cross-file edges within a
// package are supported, cross-package symbol edges are not (v1).
type IndexRef struct {
	SrcFile string
	SrcName string
	DstFile string
	DstName string
	Kind    string
}

// DirIndex is the full index of one package directory: its files, their
// symbols, and every intra-package reference.
type DirIndex struct {
	Dir   string
	Files []IndexFile
	Refs  []IndexRef
}

// dirFiles selects the index_files rows whose relative path belongs to dir.
func (s *Store) dirFilePaths(ctx context.Context, repoPath, dir string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT path FROM index_files WHERE repo_path = ?`, repoPath)
	if err != nil {
		return nil, fmt.Errorf("query dir files: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		if dirOf(p) == dir {
			out = append(out, p)
		}
	}
	return out, rows.Err()
}

// dirOf returns the package directory of a relative path; root-level files
// belong to the empty-string directory.
func dirOf(rel string) string {
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		return rel[:i]
	}
	return ""
}

// DirOf is the exported form of dirOf, used by the indexer to group files.
func DirOf(rel string) string { return dirOf(rel) }

// ReplaceDirIndex atomically replaces the stored index of one package
// directory (repoPath, dir) with files and refs. Files removed from the
// directory since the last write are dropped because the directory's rows
// are deleted first; ON DELETE CASCADE removes their symbols and refs.
// All refs must reference symbols within the directory being written.
func (s *Store) ReplaceDirIndex(ctx context.Context, repoPath, dir string, files []IndexFile, refs []IndexRef) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	oldPaths, err := s.dirFilePathsTx(ctx, tx, repoPath, dir)
	if err != nil {
		return err
	}
	for _, p := range oldPaths {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM index_files WHERE repo_path = ? AND path = ?`, repoPath, p); err != nil {
			return fmt.Errorf("delete stale file row %s: %w", p, err)
		}
	}

	// idByKey maps "file\x00name" to the symbol id inserted below, so refs
	// can resolve their endpoints without touching the database again.
	idByKey := make(map[string]int64, 4096)
	now := time.Now().UTC().Format(time.RFC3339)
	for _, f := range files {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO index_files (repo_path, path, language, hash, updated_at)
VALUES (?, ?, ?, ?, ?)`,
			repoPath, f.Path, f.Language, f.Hash, now); err != nil {
			return fmt.Errorf("insert file row %s: %w", f.Path, err)
		}
		for _, sym := range f.Symbols {
			res, err := tx.ExecContext(ctx, `
INSERT INTO index_symbols (repo_path, file_path, name, kind, line)
VALUES (?, ?, ?, ?, ?)`,
				repoPath, f.Path, sym.Name, sym.Kind, sym.Line)
			if err != nil {
				return fmt.Errorf("insert symbol %s.%s: %w", f.Path, sym.Name, err)
			}
			id, err := res.LastInsertId()
			if err != nil {
				return fmt.Errorf("symbol id %s.%s: %w", f.Path, sym.Name, err)
			}
			idByKey[symKey(f.Path, sym.Name)] = id
		}
		for _, imp := range f.Imports {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO index_imports (repo_path, file_path, import_path)
VALUES (?, ?, ?)`, repoPath, f.Path, imp); err != nil {
				return fmt.Errorf("insert import %s %s: %w", f.Path, imp, err)
			}
		}
	}

	for _, r := range refs {
		srcID, ok := idByKey[symKey(r.SrcFile, r.SrcName)]
		if !ok {
			return fmt.Errorf("ref src symbol %s.%s not in package %s", r.SrcFile, r.SrcName, dir)
		}
		dstID, ok := idByKey[symKey(r.DstFile, r.DstName)]
		if !ok {
			return fmt.Errorf("ref dst symbol %s.%s not in package %s", r.DstFile, r.DstName, dir)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO index_refs (repo_path, src_symbol_id, dst_symbol_id, kind)
VALUES (?, ?, ?, ?)`, repoPath, srcID, dstID, r.Kind); err != nil {
			return fmt.Errorf("insert ref %s.%s -> %s.%s: %w",
				r.SrcFile, r.SrcName, r.DstFile, r.DstName, err)
		}
	}
	return tx.Commit()
}

// dirFilePathsTx is the transactional variant of dirFilePaths.
func (s *Store) dirFilePathsTx(ctx context.Context, tx *sql.Tx, repoPath, dir string) ([]string, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT path FROM index_files WHERE repo_path = ?`, repoPath)
	if err != nil {
		return nil, fmt.Errorf("query dir files: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		if dirOf(p) == dir {
			out = append(out, p)
		}
	}
	return out, rows.Err()
}

// DeleteDirIndex removes every stored row for one package directory.
func (s *Store) DeleteDirIndex(ctx context.Context, repoPath, dir string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	paths, err := s.dirFilePathsTx(ctx, tx, repoPath, dir)
	if err != nil {
		return err
	}
	for _, p := range paths {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM index_files WHERE repo_path = ? AND path = ?`, repoPath, p); err != nil {
			return fmt.Errorf("delete file row %s: %w", p, err)
		}
	}
	return tx.Commit()
}

// IndexHashes returns the current content hash of every indexed file of a
// repository, keyed by relative path. It drives incremental indexing.
func (s *Store) IndexHashes(ctx context.Context, repoPath string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT path, hash FROM index_files WHERE repo_path = ?`, repoPath)
	if err != nil {
		return nil, fmt.Errorf("query index hashes: %w", err)
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var p, h string
		if err := rows.Scan(&p, &h); err != nil {
			return nil, err
		}
		out[p] = h
	}
	return out, rows.Err()
}

// DirIndex loads the full index of one package directory.
func (s *Store) DirIndex(ctx context.Context, repoPath, dir string) (*DirIndex, error) {
	// Files and their symbols.
	fileRows, err := s.db.QueryContext(ctx, `
SELECT f.path, f.language, f.hash, sy.name, sy.kind, sy.line
FROM index_files f
LEFT JOIN index_symbols sy
  ON sy.repo_path = f.repo_path AND sy.file_path = f.path
WHERE f.repo_path = ?`, repoPath)
	if err != nil {
		return nil, fmt.Errorf("query dir symbols: %w", err)
	}
	defer fileRows.Close()

	filesByPath := map[string]*IndexFile{}
	var order []string
	for fileRows.Next() {
		var p, lang, hash string
		var name, kind sql.NullString
		var line sql.NullInt64
		if err := fileRows.Scan(&p, &lang, &hash, &name, &kind, &line); err != nil {
			return nil, err
		}
		if dirOf(p) != dir {
			continue
		}
		f, ok := filesByPath[p]
		if !ok {
			f = &IndexFile{Path: p, Language: lang, Hash: hash}
			filesByPath[p] = f
			order = append(order, p)
		}
		if name.Valid {
			f.Symbols = append(f.Symbols, IndexSymbol{Name: name.String, Kind: kind.String, Line: int(line.Int64)})
		}
	}
	if err := fileRows.Err(); err != nil {
		return nil, err
	}

	// Imports per file.
	impRows, err := s.db.QueryContext(ctx, `
SELECT file_path, import_path FROM index_imports WHERE repo_path = ?`, repoPath)
	if err != nil {
		return nil, fmt.Errorf("query dir imports: %w", err)
	}
	defer impRows.Close()
	for impRows.Next() {
		var p, imp string
		if err := impRows.Scan(&p, &imp); err != nil {
			return nil, err
		}
		if f, ok := filesByPath[p]; ok {
			f.Imports = append(f.Imports, imp)
		}
	}
	if err := impRows.Err(); err != nil {
		return nil, err
	}

	// Intra-package references, denormalized to symbol names.
	refRows, err := s.db.QueryContext(ctx, `
SELECT src.path, srcs.name, dst.path, dsts.name, r.kind
FROM index_refs r
JOIN index_symbols srcs  ON r.src_symbol_id = srcs.id
JOIN index_files    src  ON srcs.repo_path = src.repo_path AND srcs.file_path = src.path
JOIN index_symbols dsts ON r.dst_symbol_id = dsts.id
JOIN index_files    dst  ON dsts.repo_path = dst.repo_path AND dsts.file_path = dst.path
WHERE r.repo_path = ?`, repoPath)
	if err != nil {
		return nil, fmt.Errorf("query dir refs: %w", err)
	}
	defer refRows.Close()

	di := &DirIndex{Dir: dir}
	refSeen := map[string]bool{}
	for refRows.Next() {
		var sf, sn, df, dn, kind string
		if err := refRows.Scan(&sf, &sn, &df, &dn, &kind); err != nil {
			return nil, err
		}
		if dirOf(sf) != dir || dirOf(df) != dir {
			continue
		}
		key := strings.Join([]string{sf, sn, df, dn, kind}, "\x00")
		if refSeen[key] {
			continue
		}
		refSeen[key] = true
		di.Refs = append(di.Refs, IndexRef{SrcFile: sf, SrcName: sn, DstFile: df, DstName: dn, Kind: kind})
	}
	if err := refRows.Err(); err != nil {
		return nil, err
	}

	sort.Strings(order)
	for _, p := range order {
		f := filesByPath[p]
		sort.Slice(f.Symbols, func(i, j int) bool {
			if f.Symbols[i].Line != f.Symbols[j].Line {
				return f.Symbols[i].Line < f.Symbols[j].Line
			}
			return f.Symbols[i].Name < f.Symbols[j].Name
		})
		sort.Strings(f.Imports)
		di.Files = append(di.Files, *f)
	}
	sort.Slice(di.Refs, func(i, j int) bool {
		a, b := di.Refs[i], di.Refs[j]
		if a.SrcFile != b.SrcFile {
			return a.SrcFile < b.SrcFile
		}
		if a.SrcName != b.SrcName {
			return a.SrcName < b.SrcName
		}
		if a.DstFile != b.DstFile {
			return a.DstFile < b.DstFile
		}
		if a.DstName != b.DstName {
			return a.DstName < b.DstName
		}
		return a.Kind < b.Kind
	})
	return di, nil
}

// symKey builds the id-map key for a symbol.
func symKey(file, name string) string {
	return file + "\x00" + name
}

// CleanPath normalizes a repository-relative path to forward slashes.
func CleanPath(p string) string {
	return path.Clean(strings.ReplaceAll(p, "\\", "/"))
}
