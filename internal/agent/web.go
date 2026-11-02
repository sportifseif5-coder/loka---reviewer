package agent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sportifseif5-coder/loka---reviewer/internal/config"
	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/vcs"
)

// Fetcher retrieves a URL body. Tests inject a stub so CI never opens a socket.
type Fetcher interface {
	Fetch(ctx context.Context, url string) (string, error)
}

// HTTPFetcher is the production fetcher. It is only constructed when agent
// mode is enabled; the web agent still refuses to call it in other modes.
type HTTPFetcher struct {
	Client *http.Client
}

// Fetch GETs url and returns a truncated body.
func (h HTTPFetcher) Fetch(ctx context.Context, raw string) (string, error) {
	c := h.Client
	if c == nil {
		c = &http.Client{Timeout: 8 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// WebRequest is the input for a web-agent pass.
type WebRequest struct {
	Mode    config.Mode
	Changed []vcs.ChangedFile
}

// WebResult is findings plus the egress log for the audit trail (I6).
type WebResult struct {
	Findings []model.Finding
	Egress   []string
}

// sink is one capability-scoped pattern the web agent may look up.
type sink struct {
	needle string
	rule   string
	msg    string
	query  string
}

var webSinks = []sink{
	{needle: "pickle.loads", rule: "web.pickle-loads", msg: "pickle.loads can execute arbitrary code", query: "pickle.loads CVE arbitrary code"},
	{needle: "eval(", rule: "web.eval", msg: "eval of untrusted input is a code-execution sink", query: "javascript eval CVE"},
	{needle: "innerHTML", rule: "web.inner-html", msg: "assigning innerHTML can introduce XSS", query: "innerHTML XSS"},
}

const wikiSearch = "https://en.wikipedia.org/w/api.php?action=opensearch&limit=1&namespace=0&format=json&search="

// WebAgent looks up known sinks against an external source. It never runs
// unless the repository is in agent mode (I6) and it logs every URL.
type WebAgent struct {
	fetch Fetcher
}

// NewWebAgent builds a web agent. A nil fetcher uses HTTPFetcher.
func NewWebAgent(f Fetcher) *WebAgent {
	if f == nil {
		f = HTTPFetcher{}
	}
	return &WebAgent{fetch: f}
}

// Name returns "web".
func (a *WebAgent) Name() string { return "web" }

// Run is a no-op unless mode is agent. Findings cite the fetched URL.
func (a *WebAgent) Run(ctx context.Context, req WebRequest) (WebResult, error) {
	if req.Mode != config.ModeAgent {
		return WebResult{}, nil
	}
	var out WebResult
	seen := map[string]bool{}
	for _, cf := range req.Changed {
		for _, ln := range cf.Added {
			for _, sk := range webSinks {
				if !strings.Contains(ln.Text, sk.needle) {
					continue
				}
				target := wikiSearch + url.QueryEscape(sk.query)
				if !seen[target] {
					if _, err := a.fetch.Fetch(ctx, target); err != nil {
						out.Egress = append(out.Egress, target)
						continue
					}
					out.Egress = append(out.Egress, target)
					seen[target] = true
				}
				out.Findings = append(out.Findings, model.Finding{
					RuleID:     sk.rule,
					Category:   "security",
					Severity:   model.SeverityWarning,
					Source:     model.SourceAgent,
					Location:   model.Location{File: cf.Path, LineStart: ln.Number, LineEnd: ln.Number},
					Message:    sk.msg,
					Reasoning:  "cited from an external source in agent mode",
					Evidence:   []string{fmt.Sprintf("source: %s", target)},
					Confidence: 0.5,
				})
			}
		}
	}
	return out, nil
}
