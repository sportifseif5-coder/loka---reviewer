// Package advisory matches changed dependency lines against a locally
// bundled vulnerability database (architecture 6.2). Updates are
// user-initiated from a local file; the package never opens a socket (I1, I6).
package advisory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/sportifseif5-coder/loka---reviewer/internal/analyzer"
	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
)

// Advisory is one locally stored vulnerability record.
type Advisory struct {
	ID        string `json:"id"`
	Package   string `json:"package"`
	Ecosystem string `json:"ecosystem"`
	Below     string `json:"below"`
	Severity  string `json:"severity"`
	Summary   string `json:"summary"`
}

// DB is a versioned advisory snapshot.
type DB struct {
	UpdatedAt  string     `json:"updated_at"`
	Advisories []Advisory `json:"advisories"`
}

// Bundled returns the in-binary advisory set shipped with Loka.
func Bundled() DB {
	return bundled
}

var bundled = DB{
	UpdatedAt: "2026-11-02T00:00:00Z",
	Advisories: []Advisory{{
		ID:        "GO-2026-0001",
		Package:   "github.com/loka-fixture/vulnerable",
		Ecosystem: "go",
		Below:     "1.0.0",
		Severity:  "high",
		Summary:   "fixture advisory used by the precision corpus",
	}},
}

// Load reads a DB from a local JSON file.
func Load(path string) (DB, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return DB{}, err
	}
	var db DB
	if err := json.Unmarshal(data, &db); err != nil {
		return DB{}, fmt.Errorf("parse advisory db: %w", err)
	}
	return db, nil
}

// Update copies a local advisory file into dest. Remote URLs are rejected
// so the update flow cannot exfiltrate code or pull untrusted network data.
func Update(src, dest string) error {
	if looksRemote(src) {
		return fmt.Errorf("advisory update is local-file only (got %s)", src)
	}
	if looksRemote(dest) {
		return fmt.Errorf("advisory dest must be a local path")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	var db DB
	if err := json.Unmarshal(data, &db); err != nil {
		return fmt.Errorf("parse advisory db: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o600)
}

func looksRemote(p string) bool {
	s := strings.ToLower(strings.TrimSpace(p))
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "ftp://")
}

// Scanner is the deterministic advisory analyzer.
type Scanner struct {
	DB DB
}

// Name returns the analyzer name.
func (Scanner) Name() string { return "advisory" }

var (
	goModRequire = regexp.MustCompile(`^\s*([A-Za-z0-9./_\-]+)\s+v?([0-9]+\.[0-9]+\.[0-9]+)`)
	pipPin       = regexp.MustCompile(`^\s*([A-Za-z0-9._\-]+)==([0-9]+\.[0-9]+\.[0-9]+)`)
)

// Analyze matches added dependency lines against the local DB.
func (s Scanner) Analyze(ctx context.Context, unit analyzer.AnalysisUnit) ([]model.Finding, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	db := s.DB
	if len(db.Advisories) == 0 {
		db = Bundled()
	}
	var out []model.Finding
	for _, cf := range unit.Changed {
		base := filepath.Base(cf.Path)
		for _, ln := range cf.Added {
			pkg, ver, ok := parseDep(base, ln.Text)
			if !ok {
				continue
			}
			for _, a := range db.Advisories {
				if !pkgMatch(a.Package, pkg) {
					continue
				}
				if !below(ver, a.Below) {
					continue
				}
				out = append(out, model.Finding{
					RuleID:     "advisory." + a.ID,
					Category:   "advisory",
					Severity:   sev(a.Severity),
					Source:     model.SourceAnalyzer,
					Location:   model.Location{File: cf.Path, LineStart: ln.Number, LineEnd: ln.Number},
					Message:    fmt.Sprintf("%s %s is vulnerable (%s): %s", pkg, ver, a.ID, a.Summary),
					Reasoning:  "matched locally bundled advisory database",
					Evidence:   []string{strings.TrimSpace(ln.Text), "fixed in " + a.Below + "+"},
					Confidence: 1.0,
				})
			}
		}
	}
	return out, nil
}

func parseDep(base, line string) (pkg, ver string, ok bool) {
	line = strings.TrimSpace(line)
	switch base {
	case "go.mod":
		if strings.HasPrefix(line, "module ") || strings.HasPrefix(line, "go ") {
			return "", "", false
		}
		line = strings.TrimPrefix(line, "require ")
		m := goModRequire.FindStringSubmatch(line)
		if m == nil {
			return "", "", false
		}
		return m[1], m[2], true
	case "requirements.txt":
		m := pipPin.FindStringSubmatch(line)
		if m == nil {
			return "", "", false
		}
		return m[1], m[2], true
	}
	return "", "", false
}

func pkgMatch(want, got string) bool {
	return strings.EqualFold(want, got)
}

func below(have, cutoff string) bool {
	h, ok1 := parseVer(have)
	c, ok2 := parseVer(cutoff)
	if !ok1 || !ok2 {
		return false
	}
	for i := 0; i < 3; i++ {
		if h[i] < c[i] {
			return true
		}
		if h[i] > c[i] {
			return false
		}
	}
	return false
}

func parseVer(s string) ([3]int, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	parts := strings.Split(s, ".")
	if len(parts) < 3 {
		return [3]int{}, false
	}
	var out [3]int
	for i := 0; i < 3; i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}

func sev(s string) model.Severity {
	switch strings.ToLower(s) {
	case "critical":
		return model.SeverityCritical
	case "high", "error":
		return model.SeverityError
	case "medium", "warning":
		return model.SeverityWarning
	default:
		return model.SeverityInfo
	}
}
