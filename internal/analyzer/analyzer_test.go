package analyzer

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/vcs"
)

// unitFor builds an AnalysisUnit from path -> (line number -> text).
func unitFor(path string, lines map[string]map[int]string) AnalysisUnit {
	var changed []vcs.ChangedFile
	for p, lns := range lines {
		var added []vcs.AddedLine
		for n, text := range lns {
			added = append(added, vcs.AddedLine{Number: n, Text: text})
		}
		changed = append(changed, vcs.ChangedFile{Path: p, Added: added})
	}
	return AnalysisUnit{RepoPath: path, Changed: changed}
}

func TestSecretDetectorFindsAndRedacts(t *testing.T) {
	unit := unitFor("", map[string]map[int]string{
		"cmd/main.go": {
			12: `aws_secret_access_key = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"`,
			13: `api_key: "dGhpc2lzYTE2Y2hhciB0b2tlbnN0cmluZw=="`,
			14: `const greeting = "hello"`,
		},
	})
	d := SecretDetector{}
	findings, err := d.Analyze(context.Background(), unit)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("got %d findings, want 2: %+v", len(findings), findings)
	}

	var aws *model.Finding
	for i := range findings {
		if findings[i].RuleID == "secret.aws-secret-key" {
			aws = &findings[i]
		}
	}
	if aws == nil {
		t.Fatal("aws-secret-key finding missing")
	}
	if aws.Severity != model.SeverityCritical {
		t.Errorf("aws severity = %q, want critical", aws.Severity)
	}
	if aws.Location.File != "cmd/main.go" || aws.Location.LineStart != 12 {
		t.Errorf("location = %+v", aws.Location)
	}
	if strings.Contains(aws.Evidence[0], "wJalrXUtnFEMI") {
		t.Errorf("evidence must redact the secret: %q", aws.Evidence[0])
	}
}

func TestSecretDetectorIgnoresLockFiles(t *testing.T) {
	unit := unitFor("", map[string]map[int]string{
		"go.sum": {1: `AKIAIOSFODNN7EXAMPLE github.com/x v1.0`},
	})
	d := SecretDetector{}
	findings, err := d.Analyze(context.Background(), unit)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("lock file produced findings: %+v", findings)
	}
}

func TestGoVetBridgeReportsDiagnostics(t *testing.T) {
	fixture := filepath.Join("testdata", "vetbroken")
	unit := AnalysisUnit{
		RepoPath: fixture,
		Changed: []vcs.ChangedFile{
			{Path: "main.go", Added: []vcs.AddedLine{{Number: 7, Text: `	fmt.Printf("%d", name)`}}},
		},
	}
	b := GoVetBridge{}
	findings, err := b.Analyze(context.Background(), unit)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(findings) == 0 {
		t.Fatal("go vet produced no findings on broken fixture")
	}
	f := findings[0]
	if f.Source != model.SourceAnalyzer {
		t.Errorf("source = %q, want analyzer", f.Source)
	}
	if f.Location.File != "main.go" {
		t.Errorf("file = %q, want main.go", f.Location.File)
	}
	if !strings.Contains(strings.ToLower(f.Message), "fmt") && !strings.Contains(strings.ToLower(f.Message), "format") {
		t.Errorf("unexpected message: %q", f.Message)
	}
}

func TestGoVetBridgeNoGoFiles(t *testing.T) {
	unit := unitFor("", map[string]map[int]string{"README.md": {1: "hello"}})
	b := GoVetBridge{}
	findings, err := b.Analyze(context.Background(), unit)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("got %d findings, want 0", len(findings))
	}
}

func TestSplitDiagnostic(t *testing.T) {
	file, line, msg, ok := splitDiagnostic("internal/vcs/git.go:12:5: unused parameter x")
	if !ok || file != "internal/vcs/git.go" || line != 12 || msg != "unused parameter x" {
		t.Errorf("parsed %q %d %q ok=%v", file, line, msg, ok)
	}
	_, _, _, ok = splitDiagnostic("garbage")
	if ok {
		t.Error("garbage should not parse")
	}
}
