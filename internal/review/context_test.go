package review

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
)

// writeLines writes rel under dir with n lines named line1..lineN and returns
// its absolute path.
func writeLines(t *testing.T, dir, rel string, n int) string {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	var b []byte
	for i := 1; i <= n; i++ {
		b = append(b, []byte("line"+strconv.Itoa(i)+"\n")...)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFileWindowAroundFinding(t *testing.T) {
	repo := t.TempDir()
	writeLines(t, repo, filepath.Join("pkg", "a.go"), 20)

	cases := []struct {
		name      string
		loc       model.Location
		radius    int
		wantStart int
		wantEnd   int
	}{
		{"middle", model.Location{File: "pkg/a.go", LineStart: 10, LineEnd: 10}, 2, 8, 12},
		{"multi-line range", model.Location{File: "pkg/a.go", LineStart: 5, LineEnd: 7}, 1, 4, 8},
		{"clamped at start", model.Location{File: "pkg/a.go", LineStart: 1, LineEnd: 1}, 5, 1, 6},
		{"clamped at end", model.Location{File: "pkg/a.go", LineStart: 20, LineEnd: 20}, 5, 15, 20},
		{"zero radius uses default", model.Location{File: "pkg/a.go", LineStart: 10, LineEnd: 10}, 0, 7, 13},
		{"missing line uses top", model.Location{File: "pkg/a.go"}, 1, 1, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FileWindow(repo, "pkg/a.go", tc.loc, tc.radius)
			if err != nil {
				t.Fatalf("FileWindow: %v", err)
			}
			if len(got) == 0 {
				t.Fatal("empty window")
			}
			if got[0].Number != tc.wantStart || got[len(got)-1].Number != tc.wantEnd {
				t.Fatalf("window = [%d..%d], want [%d..%d]",
					got[0].Number, got[len(got)-1].Number, tc.wantStart, tc.wantEnd)
			}
			// Numbers are contiguous and Text matches the source line.
			for i, ln := range got {
				want := tc.wantStart + i
				if ln.Number != want {
					t.Errorf("line %d number = %d, want %d", i, ln.Number, want)
				}
				if ln.Text != "line"+strconv.Itoa(want) {
					t.Errorf("line %d text = %q, want %q", i, ln.Text, "line"+strconv.Itoa(want))
				}
			}
		})
	}
}

func TestFileWindowRejectsBadPaths(t *testing.T) {
	repo := t.TempDir()
	writeLines(t, repo, "a.go", 3)

	if _, err := FileWindow(repo, "missing.go", model.Location{LineStart: 1}, 2); err == nil {
		t.Error("missing file: want error")
	}
	if _, err := FileWindow(repo, "/etc/passwd", model.Location{LineStart: 1}, 2); err == nil {
		t.Error("absolute path: want error")
	}
	if _, err := FileWindow(repo, "../outside.go", model.Location{LineStart: 1}, 2); err == nil {
		t.Error("escaping path: want error")
	}
	if _, err := FileWindow("", "a.go", model.Location{LineStart: 1}, 2); err == nil {
		t.Error("empty repo: want error")
	}
	if _, err := FileWindow(repo, "", model.Location{LineStart: 1}, 2); err == nil {
		t.Error("empty file: want error")
	}
}
