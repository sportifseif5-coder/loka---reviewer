package analyzer

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
)

// secretPattern pairs a detector regex with the metadata used to build a
// finding. The regex must capture the secret value as submatch 1.
type secretPattern struct {
	category string
	severity model.Severity
	message  string
	re       *regexp.Regexp
}

// secretPatterns is the bundled local secret-detection set. Values are
// redacted before being stored as evidence.
var secretPatterns = []secretPattern{
	{
		category: "secret.aws-access-key",
		severity: model.SeverityCritical,
		message:  "AWS access key ID committed to source",
		re:       regexp.MustCompile(`(AKIA[0-9A-Z]{16})`),
	},
	{
		category: "secret.aws-secret-key",
		severity: model.SeverityCritical,
		message:  "AWS secret access key committed to source",
		re:       regexp.MustCompile(`(?i)aws_secret_access_key\s*[=:]\s*["']?([A-Za-z0-9/+=]{20,})`),
	},
	{
		category: "secret.github-token",
		severity: model.SeverityCritical,
		message:  "GitHub personal access token committed to source",
		re:       regexp.MustCompile(`(ghp_[0-9A-Za-z]{36})`),
	},
	{
		category: "secret.private-key",
		severity: model.SeverityCritical,
		message:  "Private key material committed to source",
		re:       regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----`),
	},
	{
		category: "secret.slack-token",
		severity: model.SeverityError,
		message:  "Slack token committed to source",
		re:       regexp.MustCompile(`(xox[baprs]-[0-9A-Za-z-]{10,})`),
	},
	{
		category: "secret.stripe-key",
		severity: model.SeverityError,
		message:  "Stripe live key committed to source",
		re:       regexp.MustCompile(`(sk_live_[0-9A-Za-z]{24})`),
	},
	{
		category: "secret.generic-key",
		severity: model.SeverityWarning,
		message:  "Possible credential assigned in source",
		re:       regexp.MustCompile(`(?i)(?:api[_-]?key|apikey|secret[_-]?key|access[_-]?token|auth[_-]?token|password)\s*[=:]\s*["']?([A-Za-z0-9_\-\.]{16,})`),
	},
}

// ignoredPaths are file names that routinely contain high-entropy strings
// but are not secrets (lock files, sum files, vendored data).
var ignoredPaths = map[string]bool{
	"go.sum": true, "package-lock.json": true, "yarn.lock": true,
	"Cargo.lock": true, "pnpm-lock.yaml": true, "poetry.lock": true,
}

// SecretDetector scans added lines for committed secrets using locally
// bundled patterns. Values are redacted in findings.
type SecretDetector struct{}

// Name returns the analyzer name.
func (SecretDetector) Name() string { return "secret-detection" }

// Analyze scans added lines of changed files for secret patterns.
func (SecretDetector) Analyze(ctx context.Context, unit AnalysisUnit) ([]model.Finding, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var findings []model.Finding
	for _, cf := range unit.Changed {
		if ignoredPaths[baseName(cf.Path)] || strings.HasPrefix(cf.Path, "vendor/") {
			continue
		}
		for _, al := range cf.Added {
			findings = append(findings, scanLine(cf.Path, al.Number, al.Text)...)
		}
	}
	return findings, nil
}

func scanLine(path string, line int, text string) []model.Finding {
	var findings []model.Finding
	for _, p := range secretPatterns {
		m := p.re.FindStringSubmatch(text)
		if m == nil {
			continue
		}
		value := ""
		if len(m) > 1 {
			value = m[1]
		}
		findings = append(findings, model.Finding{
			RuleID:     p.category,
			Category:   p.category,
			Severity:   p.severity,
			Source:     model.SourceAnalyzer,
			Location:   model.Location{File: path, LineStart: line, LineEnd: line},
			Message:    p.message,
			Reasoning:  fmt.Sprintf("matched bundled pattern %q", p.category),
			Confidence: 1.0,
			Evidence:   []string{redact(text, value)},
		})
	}
	return findings
}

// redact replaces the secret value in a line so the finding does not
// persist the credential itself.
func redact(line, value string) string {
	if value == "" {
		return line
	}
	return strings.ReplaceAll(line, value, "***")
}

func baseName(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}
