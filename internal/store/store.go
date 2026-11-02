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
	// Enforce foreign keys so ON DELETE CASCADE keeps index files/symbols
	// and review/finding rows consistent (SQLite disables this by default).
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
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
	// v6: rebuild the index tables repo-scoped. The v2 index tables keyed
	// files by bare path, which collides when one shared database holds
	// several repositories (the default user data dir does). They were never
	// written (the indexer ships after this migration), so the rebuild is
	// lossless. Files now carry repo_path as part of the primary key and all
	// cascade deletes are enforced by PRAGMA foreign_keys = ON.
	`
DROP TABLE IF EXISTS index_refs;
DROP TABLE IF EXISTS index_symbols;
DROP TABLE IF EXISTS index_files;

CREATE TABLE index_files (
    repo_path  TEXT NOT NULL,
    path       TEXT NOT NULL,
    language   TEXT NOT NULL DEFAULT '',
    hash       TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (repo_path, path)
);

CREATE TABLE index_symbols (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_path TEXT NOT NULL,
    file_path TEXT NOT NULL,
    name      TEXT NOT NULL,
    kind      TEXT NOT NULL,
    line      INTEGER NOT NULL,
    FOREIGN KEY (repo_path, file_path)
        REFERENCES index_files(repo_path, path) ON DELETE CASCADE
);

CREATE INDEX idx_symbols_name ON index_symbols(repo_path, name);
CREATE INDEX idx_symbols_file ON index_symbols(repo_path, file_path);

CREATE TABLE index_refs (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_path     TEXT NOT NULL,
    src_symbol_id INTEGER NOT NULL REFERENCES index_symbols(id) ON DELETE CASCADE,
    dst_symbol_id INTEGER REFERENCES index_symbols(id) ON DELETE CASCADE,
    kind          TEXT NOT NULL
);

CREATE INDEX idx_refs_src ON index_refs(repo_path, src_symbol_id);
CREATE INDEX idx_refs_dst ON index_refs(repo_path, dst_symbol_id);

CREATE TABLE index_imports (
    repo_path   TEXT NOT NULL,
    file_path   TEXT NOT NULL,
    import_path TEXT NOT NULL,
    FOREIGN KEY (repo_path, file_path)
        REFERENCES index_files(repo_path, path) ON DELETE CASCADE
);

CREATE INDEX idx_imports_file ON index_imports(repo_path, file_path);
`,
	// v7: history reads for the workbench. The reviews primary key is the id,
	// so per-repository listing otherwise scans the whole table. The index
	// serves both "most recent review per repository" (ListRepos) and "all
	// reviews for one repository" (ListReviews), newest first.
	`
CREATE INDEX idx_reviews_repo_started ON reviews(repo_path, started_at DESC, id DESC);
`,
	// v8: structured review summary (walkthrough + risk) on ReviewResult
	// (ADR-0006, architecture section 4.1). Empty strings are the pre-v8
	// default so existing rows stay readable.
	`
ALTER TABLE reviews ADD COLUMN walkthrough TEXT NOT NULL DEFAULT '';
ALTER TABLE reviews ADD COLUMN risk TEXT NOT NULL DEFAULT '';
`,
	// v9: Phase 2 learnings, failover banner, egress log, suggested fixes.
	`
ALTER TABLE reviews ADD COLUMN banner TEXT NOT NULL DEFAULT '';
ALTER TABLE reviews ADD COLUMN egress TEXT NOT NULL DEFAULT '[]';
ALTER TABLE findings ADD COLUMN suggested_fix TEXT NOT NULL DEFAULT '';
ALTER TABLE learnings ADD COLUMN repo_path TEXT NOT NULL DEFAULT '';
ALTER TABLE learnings ADD COLUMN category TEXT NOT NULL DEFAULT '';
CREATE TABLE feedback_events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_path  TEXT NOT NULL,
    finding_id TEXT NOT NULL DEFAULT '',
    category   TEXT NOT NULL,
    action     TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX idx_feedback_repo ON feedback_events(repo_path, category, action);
CREATE INDEX idx_learnings_repo ON learnings(repo_path, category);
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

	egress, err := json.Marshal(r.Egress)
	if err != nil {
		return fmt.Errorf("marshal egress: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO reviews (id, repo_path, mode, started_at, finished_at, findings_count, walkthrough, risk, banner, egress)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.RepoPath, r.Mode,
		r.StartedAt.UTC().Format(time.RFC3339),
		r.FinishedAt.UTC().Format(time.RFC3339),
		len(r.Findings), r.Summary.Walkthrough, r.Summary.Risk, r.Banner, string(egress)); err != nil {
		return fmt.Errorf("insert review: %w", err)
	}

	for _, f := range r.Findings {
		evidence, err := json.Marshal(f.Evidence)
		if err != nil {
			return fmt.Errorf("marshal evidence: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO findings
    (id, review_id, rule_id, category, severity, source, file_path, line_start, line_end, message, reasoning, confidence, evidence, demoted, suggested_fix)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			f.ID, r.ID, f.RuleID, f.Category, string(f.Severity), string(f.Source),
			f.Location.File, nullableInt(f.Location.LineStart), nullableInt(f.Location.LineEnd),
			f.Message, f.Reasoning, f.Confidence, string(evidence), boolInt(f.Demoted), f.SuggestedFix); err != nil {
			return fmt.Errorf("insert finding: %w", err)
		}
	}
	return tx.Commit()
}

// GetReview loads a review result and its findings by ID.
func (s *Store) GetReview(ctx context.Context, id string) (*model.ReviewResult, error) {
	var err error
	row := s.db.QueryRowContext(ctx, `
SELECT id, repo_path, mode, started_at, finished_at, walkthrough, risk, banner, egress
FROM reviews WHERE id = ?`, id)
	var r model.ReviewResult
	var started, finished, egress string
	if err := row.Scan(&r.ID, &r.RepoPath, &r.Mode, &started, &finished, &r.Summary.Walkthrough, &r.Summary.Risk, &r.Banner, &egress); err != nil {
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
	if egress != "" && egress != "[]" {
		if err := json.Unmarshal([]byte(egress), &r.Egress); err != nil {
			return nil, fmt.Errorf("unmarshal egress: %w", err)
		}
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT id, rule_id, category, severity, source, file_path, line_start, line_end, message, reasoning, confidence, evidence, demoted, suggested_fix
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
			&lineStart, &lineEnd, &f.Message, &f.Reasoning, &f.Confidence, &evidence, &demoted, &f.SuggestedFix); err != nil {
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

// RepoSummary is one row of the workbench "repositories" list: the most
// recent review stored for a repository plus the number of reviews kept for
// it.
type RepoSummary struct {
	RepoPath      string    `json:"repo_path"`
	Mode          string    `json:"mode"`
	LastStartedAt time.Time `json:"last_started_at"`
	LastFindings  int       `json:"last_findings"`
	ReviewCount   int       `json:"review_count"`
}

// ReviewSummary is one row of the per-repository review history list.
type ReviewSummary struct {
	ID            string    `json:"id"`
	Mode          string    `json:"mode"`
	StartedAt     time.Time `json:"started_at"`
	FinishedAt    time.Time `json:"finished_at"`
	FindingsCount int       `json:"findings_count"`
}

// ListRepos returns one summary per repository that has stored reviews, the
// newest review first. The "latest" tie-break on id keeps the order
// deterministic when two reviews share a started_at second.
func (s *Store) ListRepos(ctx context.Context) ([]RepoSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT r.repo_path, r.mode, r.started_at, r.findings_count,
       (SELECT COUNT(*) FROM reviews rc WHERE rc.repo_path = r.repo_path)
FROM reviews r
WHERE NOT EXISTS (
    SELECT 1 FROM reviews r2
    WHERE r2.repo_path = r.repo_path
      AND (r2.started_at > r.started_at OR (r2.started_at = r.started_at AND r2.id > r.id))
)
ORDER BY r.started_at DESC, r.repo_path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RepoSummary
	for rows.Next() {
		var rs RepoSummary
		var started string
		if err := rows.Scan(&rs.RepoPath, &rs.Mode, &started, &rs.LastFindings, &rs.ReviewCount); err != nil {
			return nil, err
		}
		if rs.LastStartedAt, err = time.Parse(time.RFC3339, started); err != nil {
			return nil, err
		}
		out = append(out, rs)
	}
	return out, rows.Err()
}

// ListReviews returns the reviews stored for one repository, newest first.
func (s *Store) ListReviews(ctx context.Context, repoPath string) ([]ReviewSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, mode, started_at, finished_at, findings_count
FROM reviews WHERE repo_path = ?
ORDER BY started_at DESC, id DESC`, repoPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ReviewSummary
	for rows.Next() {
		var rs ReviewSummary
		var started, finished string
		if err := rows.Scan(&rs.ID, &rs.Mode, &started, &finished, &rs.FindingsCount); err != nil {
			return nil, err
		}
		if rs.StartedAt, err = time.Parse(time.RFC3339, started); err != nil {
			return nil, err
		}
		if rs.FinishedAt, err = time.Parse(time.RFC3339, finished); err != nil {
			return nil, err
		}
		out = append(out, rs)
	}
	return out, rows.Err()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// RecordFeedback stores one user decision on a finding (accept/dismiss/apply).
func (s *Store) RecordFeedback(ctx context.Context, ev model.FeedbackEvent) error {
	if ev.RepoPath == "" || ev.Action == "" {
		return fmt.Errorf("feedback requires repo_path and action")
	}
	at := ev.At
	if at.IsZero() {
		at = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO feedback_events (repo_path, finding_id, category, action, created_at)
VALUES (?, ?, ?, ?, ?)`,
		ev.RepoPath, ev.FindingID, ev.Category, ev.Action, at.UTC().Format(time.RFC3339))
	return err
}

// ListFeedback returns recorded events for a repository, oldest first.
func (s *Store) ListFeedback(ctx context.Context, repoPath string) ([]model.FeedbackEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT repo_path, finding_id, category, action, created_at
FROM feedback_events WHERE repo_path = ? ORDER BY id`, repoPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.FeedbackEvent
	for rows.Next() {
		var ev model.FeedbackEvent
		var at string
		if err := rows.Scan(&ev.RepoPath, &ev.FindingID, &ev.Category, &ev.Action, &at); err != nil {
			return nil, err
		}
		if ev.At, err = time.Parse(time.RFC3339, at); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

const distillThreshold = 3

// Distill turns repeated dismissals into durable learnings for a repository.
// Returns the number of learnings upserted.
func (s *Store) Distill(ctx context.Context, repoPath string) (int, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT category, COUNT(*) FROM feedback_events
WHERE repo_path = ? AND action = ? AND category != ''
GROUP BY category`, repoPath, model.FeedbackDismiss)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	type agg struct {
		category string
		n        int
	}
	var aggs []agg
	for rows.Next() {
		var a agg
		if err := rows.Scan(&a.category, &a.n); err != nil {
			return 0, err
		}
		aggs = append(aggs, a)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	n := 0
	now := time.Now().UTC().Format(time.RFC3339)
	for _, a := range aggs {
		if a.n < distillThreshold {
			continue
		}
		body := fmt.Sprintf("user repeatedly dismisses category %s in this repository", a.category)
		res, err := s.db.ExecContext(ctx, `
UPDATE learnings SET body = ?, weight = ?, created_at = ?
WHERE repo_path = ? AND category = ?`,
			body, float64(a.n), now, repoPath, a.category)
		if err != nil {
			return n, err
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			if _, err := s.db.ExecContext(ctx, `
INSERT INTO learnings (body, source, weight, created_at, repo_path, category)
VALUES (?, 'feedback', ?, ?, ?, ?)`,
				body, float64(a.n), now, repoPath, a.category); err != nil {
				return n, err
			}
		}
		n++
	}
	return n, nil
}

// ListLearnings returns distilled preferences for a repository.
func (s *Store) ListLearnings(ctx context.Context, repoPath string) ([]model.Learning, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT repo_path, category, body, source, weight
FROM learnings WHERE repo_path = ? ORDER BY category`, repoPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Learning
	for rows.Next() {
		var l model.Learning
		if err := rows.Scan(&l.RepoPath, &l.Category, &l.Body, &l.Source, &l.Weight); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
