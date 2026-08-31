package rules

import (
	"strings"
	"testing"

	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
)

func TestParseRule(t *testing.T) {
	doc := `---
name: no-fmt-in-business-logic
paths: ["internal/**"]
severity: warning
pattern: "fmt\\.Println\\("
---
Use the project logger, not fmt.Println.
`
	r, err := Parse([]byte(doc), "review_rules.md")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if r.Name != "no-fmt-in-business-logic" {
		t.Errorf("name = %q", r.Name)
	}
	if r.Severity != model.SeverityWarning {
		t.Errorf("severity = %q", r.Severity)
	}
	if r.ModelAssisted() {
		t.Error("pattern rule should be deterministic")
	}
	if !r.MatchesPath("internal/engine/engine.go") {
		t.Error("internal/** should match internal/engine/engine.go")
	}
	if r.MatchesPath("cmd/main.go") {
		t.Error("internal/** should not match cmd/main.go")
	}
	if r.Message != "Use the project logger, not fmt.Println." {
		t.Errorf("message = %q", r.Message)
	}
}

func TestParseRuleDefaultsSeverity(t *testing.T) {
	doc := `---
name: no-pattern-rule
---
Some instruction that needs judgment.
`
	r, err := Parse([]byte(doc), "x")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if r.ModelAssisted() == false {
		t.Error("rule without pattern must be model-assisted")
	}
	if r.Severity != model.SeverityWarning {
		t.Errorf("default severity = %q, want warning", r.Severity)
	}
}

func TestParseRejectsMissingName(t *testing.T) {
	doc := "---\nseverity: error\n---\nbody"
	if _, err := Parse([]byte(doc), "x"); err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestParseRejectsBadSeverity(t *testing.T) {
	doc := "---\nname: x\nseverity: loud\n---\nbody"
	if _, err := Parse([]byte(doc), "x"); err == nil {
		t.Fatal("expected error for bad severity")
	}
}

func TestParseAll(t *testing.T) {
	doc := `---
name: rule-one
---
body one
---
name: rule-two
severity: error
---
body two
`
	rs, err := ParseAll([]byte(doc), "rules.md")
	if err != nil {
		t.Fatalf("ParseAll: %v", err)
	}
	if len(rs) != 2 {
		t.Fatalf("got %d rules, want 2", len(rs))
	}
	if rs[0].Name != "rule-one" || rs[1].Name != "rule-two" {
		t.Errorf("names = %q, %q", rs[0].Name, rs[1].Name)
	}
}

func TestDefaultsCompile(t *testing.T) {
	rs := Defaults()
	if len(rs) < 2 {
		t.Fatalf("too few default rules: %d", len(rs))
	}
	for _, r := range rs {
		if r.Name == "" {
			t.Error("default rule with empty name")
		}
	}
}

func TestEvaluateMatchesAddedLines(t *testing.T) {
	r, err := Parse([]byte(`---
name: no-console-log
pattern: "console\\.log\\("
---
No console.log.
`), "test")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	files := []ChangedFile{
		{
			Path: "src/app.ts",
			Added: []AddedLine{
				{Number: 10, Text: "console.log(\"hi\")"},
				{Number: 11, Text: "const x = 1"},
			},
		},
	}
	findings := Evaluate(files, []*Rule{r})
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1", len(findings))
	}
	f := findings[0]
	if f.Location.LineStart != 10 {
		t.Errorf("line = %d, want 10", f.Location.LineStart)
	}
	if f.RuleID != "no-console-log" {
		t.Errorf("rule_id = %q", f.RuleID)
	}
	if f.Source != model.SourceRule {
		t.Errorf("source = %q, want rule", f.Source)
	}
}

func TestEvaluateSkipsOutOfScopeAndModelAssisted(t *testing.T) {
	r, err := Parse([]byte(`---
name: scoped
paths: ["src/**"]
pattern: "TODO"
---
No TODOs.
`), "test")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	modelAssisted, _ := Parse([]byte("---\nname: semantic\n---\njudge this"), "test")

	files := []ChangedFile{
		{Path: "docs/guide.md", Added: []AddedLine{{Number: 1, Text: "TODO: later"}}},
		{Path: "src/main.go", Added: []AddedLine{{Number: 5, Text: "TODO: later"}}},
	}
	findings := Evaluate(files, []*Rule{r, modelAssisted})
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1", len(findings))
	}
	if findings[0].Location.File != "src/main.go" {
		t.Errorf("file = %q", findings[0].Location.File)
	}
}

func TestGlobDoubleStarMatchesRootFiles(t *testing.T) {
	r, err := Parse([]byte(`---
name: root-go
paths: ["**/*.go"]
---
body
`), "test")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !r.MatchesPath("auth.go") {
		t.Error("**/*.go should match a root-level .go file")
	}
	if !r.MatchesPath("internal/vcs/git.go") {
		t.Error("**/*.go should match a nested .go file")
	}
	if r.MatchesPath("auth.txt") {
		t.Error("**/*.go should not match a .txt file")
	}
}

func TestParseChangedFilesLineNumbers(t *testing.T) {
	diff := `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,3 +1,4 @@
 line1
+added2
 line2
 line3
+added5
`
	files := ParseChangedFiles(map[string]string{"a.go": diff})
	if len(files) != 1 {
		t.Fatalf("got %d files", len(files))
	}
	added := files[0].Added
	if len(added) != 2 {
		t.Fatalf("got %d added lines, want 2: %+v", len(added), added)
	}
	if added[0].Number != 2 || added[1].Number != 5 {
		t.Errorf("line numbers = %d, %d; want 2, 5", added[0].Number, added[1].Number)
	}
}

func TestDefaultsProduceZeroNoiseOnCleanCode(t *testing.T) {
	// Clean Go code with no console output, no conflicts, no trailing ws.
	diff := `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,2 +1,3 @@
 package main
 
 func main() {}
`
	files := ParseChangedFiles(map[string]string{"main.go": diff})
	findings := Evaluate(files, Defaults())
	if len(findings) != 0 {
		t.Fatalf("defaults flagged clean code: %+v", findings)
	}
}

func TestContainsHelper(t *testing.T) {
	if !strings.Contains("abc", "b") {
		t.Fatal("contains helper broken")
	}
}
