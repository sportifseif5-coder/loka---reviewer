// Package store persists index, findings, reviews, and learnings in a local
// SQLite database (ADR-0005). All data stays on the user's machine.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
)

// Store wraps a SQLite database and its schema migrations.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the database at path. An empty path uses
// an in-memory database, used by tests.
func Open(path string) (*Store, error) {
	if path == "" {
		path = ":memory:"
	} else if path != ":memory:" {
		if dir := filepath.Dir(path); dir != "." {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return nil, fmt.Errorf("create data dir: %w", err)
			}
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the underlying database handle.
func (s *Store) Close() error {
	return s.db.Close()
}

// migrations is an append-only list of schema migrations. Never edit an
// applied migration; append a new one.
var migrations = []string{
	// v1: reviews and findings (ADR-0006).
	`
CREATE TABLE reviews (
    id             TEXT PRIMARY KEY,
    repo_path      TEXT NOT NULL,
    mode           TEXT NOT NULL,
    started_at     TEXT NOT NULL,
    finished_at    TEXT NOT NULL,
    findings_count INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE findings (
    id         TEXT PRIMARY KEY,
    review_id  TEXT NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    rule_id    TEXT NOT NULL DEFAULT '',
    category   TEXT NOT NULL,
    severity   TEXT NOT NULL,
    source     TEXT NOT NULL,
    file_path  TEXT NOT NULL,
    line_start INTEGER,
    line_end   INTEGER,
    message    TEXT NOT NULL,
    reasoning  TEXT NOT NULL DEFAULT '',
    confidence REAL NOT NULL DEFAULT 1.0
);

CREATE INDEX idx_findings_review ON findings(review_id);
`,
	// v2: lightweight graph index tables (ADR-0004).
	`
CREATE TABLE index_files (
    path       TEXT PRIMARY KEY,
    language   TEXT NOT NULL DEFAULT '',
    hash       TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE index_symbols (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    file_path TEXT NOT NULL REFERENCES index_files(path) ON DELETE CASCADE,
    name      TEXT NOT NULL,
    kind      TEXT NOT NULL,
    line      INTEGER NOT NULL
);

CREATE INDEX idx_symbols_name ON index_symbols(name);

CREATE TABLE index_refs (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    src_symbol_id  INTEGER NOT NULL REFERENCES index_symbols(id) ON DELETE CASCADE,
    dst_symbol_id  INTEGER REFERENCES index_symbols(id) ON DELETE CASCADE,
    kind           TEXT NOT NULL
);
`,
	// v3: learnings table.
	`
CREATE TABLE learnings (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    body       TEXT NOT NULL,
    source     TEXT NOT NULL DEFAULT 'feedback',
    weight     REAL NOT NULL DEFAULT 1.0,
    created_at TEXT NOT NULL
);
`,
	// v4: findings carry evidence (ADR-0006), stored as a JSON array.
	`
ALTER TABLE findings ADD COLUMN evidence TEXT NOT NULL DEFAULT '[]';
`,
	// v5: findings carry a demotion flag set by the verification agent.
	`
ALTER TABLE findings ADD COLUMN demoted INTEGER NOT NULL DEFAULT 0;
`,
}

// migrate applies pending migrations in order, each inside a transaction.
func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
)`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}
	for i, migration := range migrations {
		version := i + 1
		var count int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&count); err != nil {
			return fmt.Errorf("check migration %d: %w", version, err)
		}
		if count > 0 {
			continue
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx, migration); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %d: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
			version, time.Now().UTC().Format(time.RFC3339)); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", version, err)
		}
	}
	return nil
}

// SaveReview persists a review result and its findings.
func (s *Store) SaveReview(ctx context.Context, r *model.ReviewResult) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
INSERT INTO reviews (id, repo_path, mode, started_at, finished_at, findings_count)
VALUES (?, ?, ?, ?, ?, ?)`,
		r.ID, r.RepoPath, r.Mode,
		r.StartedAt.UTC().Format(time.RFC3339),
		r.FinishedAt.UTC().Format(time.RFC3339),
		len(r.Findings)); err != nil {
		return fmt.Errorf("insert review: %w", err)
	}

	for _, f := range r.Findings {
		evidence, err := json.Marshal(f.Evidence)
		if err != nil {
			return fmt.Errorf("marshal evidence: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO findings
    (id, review_id, rule_id, category, severity, source, file_path, line_start, line_end, message, reasoning, confidence, evidence, demoted)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			f.ID, r.ID, f.RuleID, f.Category, string(f.Severity), string(f.Source),
			f.Location.File, nullableInt(f.Location.LineStart), nullableInt(f.Location.LineEnd),
			f.Message, f.Reasoning, f.Confidence, string(evidence), boolInt(f.Demoted)); err != nil {
			return fmt.Errorf("insert finding: %w", err)
		}
	}
	return tx.Commit()
}

// GetReview loads a review result and its findings by ID.
func (s *Store) GetReview(ctx context.Context, id string) (*model.ReviewResult, error) {
	var err error
	row := s.db.QueryRowContext(ctx, `
SELECT id, repo_path, mode, started_at, finished_at
FROM reviews WHERE id = ?`, id)
	var r model.ReviewResult
	var started, finished string
	if err := row.Scan(&r.ID, &r.RepoPath, &r.Mode, &started, &finished); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("review %s not found", id)
		}
		return nil, err
	}
	if r.StartedAt, err = time.Parse(time.RFC3339, started); err != nil {
		return nil, err
	}
	if r.FinishedAt, err = time.Parse(time.RFC3339, finished); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT id, rule_id, category, severity, source, file_path, line_start, line_end, message, reasoning, confidence, evidence, demoted
FROM findings WHERE review_id = ? ORDER BY file_path, line_start`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var f model.Finding
		var sev, src string
		var lineStart, lineEnd sql.NullInt64
		var evidence string
		var demoted int
		if err := rows.Scan(&f.ID, &f.RuleID, &f.Category, &sev, &src, &f.Location.File,
			&lineStart, &lineEnd, &f.Message, &f.Reasoning, &f.Confidence, &evidence, &demoted); err != nil {
			return nil, err
		}
		f.Severity = model.Severity(sev)
		f.Source = model.Source(src)
		f.Demoted = demoted != 0
		if lineStart.Valid {
			f.Location.LineStart = int(lineStart.Int64)
		}
		if lineEnd.Valid {
			f.Location.LineEnd = int(lineEnd.Int64)
		}
		if evidence != "" {
			if err := json.Unmarshal([]byte(evidence), &f.Evidence); err != nil {
				return nil, fmt.Errorf("unmarshal evidence: %w", err)
			}
		}
		r.Findings = append(r.Findings, f)
	}
	return &r, rows.Err()
}

func nullableInt(v int) any {
	if v <= 0 {
		return nil
	}
	return v
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
