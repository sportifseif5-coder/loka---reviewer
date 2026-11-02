package analyzer

import (
	"context"
	"testing"

	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
)

func TestPythonLintFindsPickleLoads(t *testing.T) {
	unit := unitFor("", map[string]map[int]string{
		"app.py": {4: "obj = pickle.loads(blob)"},
	})
	findings, err := PythonLint{}.Analyze(context.Background(), unit)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(findings), findings)
	}
	if findings[0].RuleID != "python.pickle-loads" {
		t.Errorf("rule = %q", findings[0].RuleID)
	}
	if findings[0].Location.LineStart != 4 {
		t.Errorf("line = %d", findings[0].Location.LineStart)
	}
	if findings[0].Source != model.SourceAnalyzer {
		t.Errorf("source = %q", findings[0].Source)
	}
}

func TestPythonLintIgnoresNonPython(t *testing.T) {
	unit := unitFor("", map[string]map[int]string{
		"app.go": {4: "pickle.loads(blob)"},
	})
	findings, err := PythonLint{}.Analyze(context.Background(), unit)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("non-python file produced findings: %+v", findings)
	}
}

func TestTypeScriptLintFindsInnerHTML(t *testing.T) {
	unit := unitFor("", map[string]map[int]string{
		"ui.ts": {9: "el.innerHTML = userInput"},
	})
	findings, err := TypeScriptLint{}.Analyze(context.Background(), unit)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(findings), findings)
	}
	if findings[0].RuleID != "ts.inner-html" {
		t.Errorf("rule = %q", findings[0].RuleID)
	}
}

func TestTypeScriptLintCoversJSX(t *testing.T) {
	unit := unitFor("", map[string]map[int]string{
		"ui.tsx": {2: "eval(code)"},
	})
	findings, err := TypeScriptLint{}.Analyze(context.Background(), unit)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("got %d findings: %+v", len(findings), findings)
	}
}
