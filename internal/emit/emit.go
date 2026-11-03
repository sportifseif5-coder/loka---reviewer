// Package emit posts review findings as Git-platform comments. It is a pure
// emitter: there is no merge, push, or approve method (invariant I7,
// architecture section 16).
package emit

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
)

// Host identifies a Git platform.
type Host string

const (
	HostGitHub Host = "github"
	HostGitLab Host = "gitlab"
)

// Target is one pull/merge request. Number is the PR/MR id.
type Target struct {
	Host   Host
	Owner  string
	Repo   string
	Number int
}

// PostedComment is one inline comment the poster would send.
type PostedComment struct {
	Host   Host
	Owner  string
	Repo   string
	Number int
	Path   string
	Line   int
	Body   string
	Merge  bool
	Push   bool
}

// Poster delivers a comment. Tests inject a stub so CI never opens a socket.
type Poster interface {
	Post(ctx context.Context, comment PostedComment) error
}

// Emitter formats findings as comments and hands them to a Poster.
type Emitter struct {
	Poster Poster
	DryRun bool
}

// Format renders one finding as a platform comment body.
func Format(f model.Finding, host string) string {
	loc := f.Location.File
	if f.Location.LineStart > 0 {
		loc = fmt.Sprintf("%s:%d", f.Location.File, f.Location.LineStart)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "**Loka** `%s` (%s)\n\n", f.RuleID, f.Severity)
	fmt.Fprintf(&b, "%s\n\n", f.Message)
	fmt.Fprintf(&b, "_Location: %s_", loc)
	if f.Reasoning != "" {
		fmt.Fprintf(&b, "\n\n%s", f.Reasoning)
	}
	_ = host
	return b.String()
}

// Emit posts one comment per finding with a location. It never merges.
func (e Emitter) Emit(ctx context.Context, t Target, res *model.ReviewResult) (int, error) {
	if res == nil {
		return 0, fmt.Errorf("nil review")
	}
	n := 0
	for _, f := range res.Findings {
		if f.Location.File == "" {
			continue
		}
		c := PostedComment{
			Host:   t.Host,
			Owner:  t.Owner,
			Repo:   t.Repo,
			Number: t.Number,
			Path:   f.Location.File,
			Line:   f.Location.LineStart,
			Body:   Format(f, string(t.Host)),
		}
		n++
		if e.DryRun || e.Poster == nil {
			continue
		}
		if err := e.Poster.Post(ctx, c); err != nil {
			return n - 1, err
		}
	}
	return n, nil
}

// ParseTarget accepts a GitHub PR or GitLab MR URL.
func ParseTarget(raw string) (Target, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Target{}, err
	}
	host := strings.ToLower(u.Hostname())
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	switch {
	case strings.Contains(host, "github"):
		if len(parts) < 4 || parts[2] != "pull" {
			return Target{}, fmt.Errorf("not a github pull request url")
		}
		n, err := strconv.Atoi(parts[3])
		if err != nil {
			return Target{}, err
		}
		return Target{Host: HostGitHub, Owner: parts[0], Repo: parts[1], Number: n}, nil
	case strings.Contains(host, "gitlab"):
		n, owner, repo, ok := gitlabMR(parts)
		if !ok {
			return Target{}, fmt.Errorf("not a gitlab merge request url")
		}
		return Target{Host: HostGitLab, Owner: owner, Repo: repo, Number: n}, nil
	}
	return Target{}, fmt.Errorf("unsupported git host %q", host)
}

func gitlabMR(parts []string) (number int, owner, repo string, ok bool) {
	for i := 0; i+2 < len(parts); i++ {
		if parts[i] == "-" && parts[i+1] == "merge_requests" {
			n, err := strconv.Atoi(parts[i+2])
			if err != nil || i < 2 {
				return 0, "", "", false
			}
			return n, parts[0], parts[1], true
		}
	}
	return 0, "", "", false
}
