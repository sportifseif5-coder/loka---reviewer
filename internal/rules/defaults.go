package rules

// defaultSpecs are the built-in conservative, low-noise rules. Each is a
// complete rule document with front-matter and body.
var defaultSpecs = []string{
	`---
name: merge-conflict-markers
severity: error
pattern: "^(<<<<<<<|>>>>>>>|=======)$"
---
Merge conflict markers left in the working tree must be resolved before the
change is ready. Remove the conflict markers and keep the correct side of the
conflict.
`,
	`---
name: trailing-whitespace
severity: info
pattern: "[ \\t]+$"
---
Trailing whitespace on added lines. Most editors strip this automatically;
keeping it makes diffs noisier.
`,
	`---
name: debug-print-in-go
paths: ["**/*.go"]
severity: warning
pattern: "fmt\\.(Println|Printf|Print)\\(|log\\.Println\\("
---
Avoid leaving debug print statements in committed code. Use the project's
logger instead, or remove the statement.
`,
	`---
name: debug-print-in-js
paths: ["**/*.js", "**/*.ts", "**/*.tsx", "**/*.jsx", "**/*.vue"]
severity: warning
pattern: "console\\.(log|debug)\\("
---
Avoid leaving console.log/console.debug calls in committed code. Use the
project's logger, or remove the statement.
`,
}
