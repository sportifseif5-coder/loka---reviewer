package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/sportifseif5-coder/loka---reviewer/internal/config"
	"github.com/sportifseif5-coder/loka---reviewer/internal/vcs"
)

type stubFetcher struct {
	calls []string
	body  string
}

func (s *stubFetcher) Fetch(_ context.Context, url string) (string, error) {
	s.calls = append(s.calls, url)
	return s.body, nil
}

func TestWebAgentBlockedOffline(t *testing.T) {
	f := &stubFetcher{body: "CVE-2024-0001"}
	wa := NewWebAgent(f)
	out, err := wa.Run(context.Background(), WebRequest{
		Mode: config.ModeOffline,
		Changed: []vcs.ChangedFile{
			{Path: "app.py", Added: []vcs.AddedLine{{Number: 3, Text: "pickle.loads(data)"}}},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("offline mode opened egress: %v", f.calls)
	}
	if len(out.Findings) != 0 || len(out.Egress) != 0 {
		t.Fatalf("offline web agent must be a no-op: %+v", out)
	}
}

func TestWebAgentBlockedOnline(t *testing.T) {
	f := &stubFetcher{body: "CVE-2024-0001"}
	wa := NewWebAgent(f)
	out, err := wa.Run(context.Background(), WebRequest{
		Mode: config.ModeOnline,
		Changed: []vcs.ChangedFile{
			{Path: "app.py", Added: []vcs.AddedLine{{Number: 3, Text: "pickle.loads(data)"}}},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("online mode must not run web agents: %v", f.calls)
	}
	if len(out.Findings) != 0 {
		t.Fatalf("online web agent must be a no-op: %+v", out)
	}
}

func TestWebAgentCitesSourceInAgentMode(t *testing.T) {
	f := &stubFetcher{body: "pickle.loads can execute arbitrary code. See CVE-2024-0001."}
	wa := NewWebAgent(f)
	out, err := wa.Run(context.Background(), WebRequest{
		Mode: config.ModeAgent,
		Changed: []vcs.ChangedFile{
			{Path: "app.py", Added: []vcs.AddedLine{{Number: 3, Text: "obj = pickle.loads(data)"}}},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(f.calls) == 0 {
		t.Fatal("agent mode should fetch a citation")
	}
	if len(out.Egress) == 0 {
		t.Fatal("egress log is empty")
	}
	if len(out.Findings) == 0 {
		t.Fatal("expected a cited finding")
	}
	fnd := out.Findings[0]
	if fnd.Source != "agent" {
		t.Errorf("source = %q, want agent", fnd.Source)
	}
	if fnd.Location.File != "app.py" || fnd.Location.LineStart != 3 {
		t.Errorf("location = %+v", fnd.Location)
	}
	cited := false
	for _, e := range fnd.Evidence {
		if strings.Contains(e, "http") {
			cited = true
		}
	}
	if !cited {
		t.Errorf("finding must cite an external URL: %+v", fnd.Evidence)
	}
}
