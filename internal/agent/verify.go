package agent

import (
	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/vcs"
)

// VerificationAgent checks candidate findings against the added lines of the
// working-tree diff (architecture section 8.1). Verification is deterministic:
// a candidate whose file and line land on an added line is confirmed; anything
// else is demoted, never deleted — demoted findings stay visible at info
// severity with low confidence so nothing the model surfaced is silently lost.
type VerificationAgent struct{}

// NewVerificationAgent builds a verifier.
func NewVerificationAgent() *VerificationAgent { return &VerificationAgent{} }

// ConfirmConfidence is assigned to findings verified against the added lines.
const ConfirmConfidence = 0.6

// DemoteConfidence is assigned to findings the verifier could not confirm.
const DemoteConfidence = 0.2

// Verify classifies candidates. It returns the classified findings and the
// number that were demoted.
func (v *VerificationAgent) Verify(candidates []model.Finding, changed []vcs.ChangedFile) ([]model.Finding, int) {
	added := addedLineSet(changed)
	out := make([]model.Finding, 0, len(candidates))
	demoted := 0
	for _, f := range candidates {
		if nums, ok := added[f.Location.File]; ok && nums[f.Location.LineStart] {
			f.Confidence = ConfirmConfidence
			out = append(out, f)
			continue
		}
		demoted++
		f.Demoted = true
		f.Severity = model.SeverityInfo
		f.Confidence = DemoteConfidence
		out = append(out, f)
	}
	return out, demoted
}

// addedLineSet maps each changed file to the set of its added line numbers.
func addedLineSet(changed []vcs.ChangedFile) map[string]map[int]bool {
	out := make(map[string]map[int]bool, len(changed))
	for _, cf := range changed {
		nums := make(map[int]bool, len(cf.Added))
		for _, ln := range cf.Added {
			nums[ln.Number] = true
		}
		out[cf.Path] = nums
	}
	return out
}
