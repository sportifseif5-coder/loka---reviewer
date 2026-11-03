// Package portable exports and imports Loka data as plain files (invariant
// I5). The user can delete the app and keep reviews, findings, and feedback.
package portable

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/store"
)

// Snapshot is the on-disk export format.
type Snapshot struct {
	Reviews   []*model.ReviewResult `json:"reviews"`
	Feedback  []model.FeedbackEvent `json:"feedback"`
	Learnings []model.Learning      `json:"learnings"`
}

// Export writes reviews.json, feedback.json, and learnings.json under dest.
func Export(ctx context.Context, s *store.Store, dest string) error {
	if err := rejectSystemPath(dest); err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return err
	}
	snap, err := collect(ctx, s)
	if err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dest, "reviews.json"), snap.Reviews); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dest, "feedback.json"), snap.Feedback); err != nil {
		return err
	}
	return writeJSON(filepath.Join(dest, "learnings.json"), snap.Learnings)
}

// Import reads an export directory into dst. Existing ids are overwritten
// by insert; collisions on review id fail the import.
func Import(ctx context.Context, dst *store.Store, src string) error {
	if err := rejectSystemPath(src); err != nil {
		return err
	}
	var reviews []*model.ReviewResult
	if err := readJSON(filepath.Join(src, "reviews.json"), &reviews); err != nil {
		return err
	}
	var feedback []model.FeedbackEvent
	if err := readJSON(filepath.Join(src, "feedback.json"), &feedback); err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, r := range reviews {
		if r == nil {
			continue
		}
		if err := dst.SaveReview(ctx, r); err != nil {
			return err
		}
	}
	for _, ev := range feedback {
		if err := dst.RecordFeedback(ctx, ev); err != nil {
			return err
		}
	}
	return nil
}

func collect(ctx context.Context, s *store.Store) (Snapshot, error) {
	repos, err := s.ListRepos(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	var snap Snapshot
	seenFB := map[string]bool{}
	seenLearn := map[string]bool{}
	for _, repo := range repos {
		hist, err := s.ListReviews(ctx, repo.RepoPath)
		if err != nil {
			return Snapshot{}, err
		}
		for _, h := range hist {
			r, err := s.GetReview(ctx, h.ID)
			if err != nil {
				return Snapshot{}, err
			}
			snap.Reviews = append(snap.Reviews, r)
		}
		if !seenFB[repo.RepoPath] {
			fb, err := s.ListFeedback(ctx, repo.RepoPath)
			if err != nil {
				return Snapshot{}, err
			}
			snap.Feedback = append(snap.Feedback, fb...)
			seenFB[repo.RepoPath] = true
		}
		if !seenLearn[repo.RepoPath] {
			ls, err := s.ListLearnings(ctx, repo.RepoPath)
			if err != nil {
				return Snapshot{}, err
			}
			snap.Learnings = append(snap.Learnings, ls...)
			seenLearn[repo.RepoPath] = true
		}
	}
	if snap.Reviews == nil {
		snap.Reviews = []*model.ReviewResult{}
	}
	if snap.Feedback == nil {
		snap.Feedback = []model.FeedbackEvent{}
	}
	if snap.Learnings == nil {
		snap.Learnings = []model.Learning{}
	}
	return snap, nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func rejectSystemPath(p string) error {
	abs, err := filepath.Abs(p)
	if err != nil {
		return err
	}
	abs = filepath.Clean(abs)
	for _, root := range []string{"/etc", "/usr", "/bin", "/sbin", "/boot", "/sys", "/proc"} {
		if abs == root || strings.HasPrefix(abs, root+"/") {
			return fmt.Errorf("refusing to write portable data under %s", root)
		}
	}
	return nil
}
