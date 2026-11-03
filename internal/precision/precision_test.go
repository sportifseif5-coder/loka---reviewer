package precision

import (
	"path/filepath"
	"testing"
)

func TestCorpusPrecisionMeetsBudget(t *testing.T) {
	dir := filepath.Join("testdata", "corpus")
	rep, err := Evaluate(dir)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if rep.Cases == 0 {
		t.Fatal("corpus produced no cases")
	}
	if !rep.Pass {
		t.Fatalf("precision gate failed: %+v", rep)
	}
	if rep.FalsePositives != 0 {
		t.Fatalf("false positives = %d, want 0 on the v1 corpus", rep.FalsePositives)
	}
}

func TestEvaluateRejectsMissingExpect(t *testing.T) {
	_, err := Evaluate(t.TempDir())
	if err == nil {
		t.Fatal("empty corpus must error")
	}
}

func TestReportJSONRoundTrip(t *testing.T) {
	r := Report{
		Cases:          2,
		TruePositives:  1,
		FalsePositives: 0,
		FalseNegatives: 1,
		Precision:      1,
		Pass:           true,
	}
	b, err := r.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnmarshalReport(b)
	if err != nil {
		t.Fatal(err)
	}
	if got.Cases != 2 || !got.Pass {
		t.Fatalf("round trip: %+v", got)
	}
}
