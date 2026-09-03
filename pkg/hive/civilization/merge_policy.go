// Package civilization contains production orchestration primitives. Its
// lifecycle names operational state only; TLC remains an external workflow.
package civilization

import (
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/transpara-ai/hive/pkg/hive/tlcbridge"
)

var gitHeadPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// AutoMergePolicy is inert unless Enabled is true and the enabling Human
// authority is named. Repositories and protected paths are explicit inputs,
// not inferred from the TLC brief.
type AutoMergePolicy struct {
	Enabled        bool
	AuthorityRef   string
	Repositories   map[string]struct{}
	ProtectedPaths []string
}

// MergeCandidate is a current provider observation. Callers must refresh it
// immediately before executing the merge effect.
type MergeCandidate struct {
	BoundRequest          tlcbridge.BoundRequest
	Repository            string
	PullRequestNumber     int
	CreatedByCivilization bool
	Open                  bool
	Draft                 bool
	HeadSHA               string
	ReviewedHeadSHA       string
	ValidatedHeadSHA      string
	RequiredChecksPassing bool
	OrdinaryReviewPassing bool
	UnresolvedBlockers    int
	OpenInterventions     int
	ChangedFiles          []string
}

type MergeDecision struct {
	Eligible bool     `json:"eligible"`
	Reasons  []string `json:"reasons"`
}

// EvaluateAutoMerge applies a fail-closed eligibility matrix. It has no side
// effects and grants no authority.
func EvaluateAutoMerge(policy AutoMergePolicy, candidate MergeCandidate) MergeDecision {
	reasons := make([]string, 0, 16)
	deny := func(reason string) { reasons = append(reasons, reason) }

	if !policy.Enabled {
		deny("auto-merge is disabled")
	}
	if strings.TrimSpace(policy.AuthorityRef) == "" {
		deny("auto-merge activation has no Human authority reference")
	}
	if candidate.BoundRequest.Envelope.Route != "Routine" {
		deny("TLC route is not Routine")
	}
	if !candidate.CreatedByCivilization {
		deny("pull request was not created by Civilization")
	}
	if _, allowed := policy.Repositories[candidate.Repository]; !allowed {
		deny("repository is not in the auto-merge allowlist")
	}
	if candidate.Repository == "" ||
		candidate.Repository != candidate.BoundRequest.Source.Repository ||
		candidate.Repository != candidate.BoundRequest.Effects.PullRequestRepository {
		deny("repository does not match bound source effects")
	}
	if candidate.PullRequestNumber <= 0 || !candidate.Open || candidate.Draft {
		deny("pull request is not open and ready")
	}
	if !gitHeadPattern.MatchString(candidate.HeadSHA) ||
		candidate.HeadSHA != candidate.ReviewedHeadSHA ||
		candidate.HeadSHA != candidate.ValidatedHeadSHA {
		deny("current, reviewed, and validated heads do not match")
	}
	if !candidate.RequiredChecksPassing {
		deny("required checks are not passing")
	}
	if !candidate.OrdinaryReviewPassing {
		deny("ordinary review is not passing")
	}
	if candidate.UnresolvedBlockers != 0 {
		deny("unresolved blockers remain")
	}
	if candidate.OpenInterventions != 0 {
		deny("Human intervention remains open")
	}
	for _, changed := range candidate.ChangedFiles {
		if protectedPath(changed, policy.ProtectedPaths) {
			deny("changed file is protected: " + changed)
		}
	}

	sort.Strings(reasons)
	return MergeDecision{Eligible: len(reasons) == 0, Reasons: reasons}
}

func protectedPath(changed string, patterns []string) bool {
	raw := strings.ReplaceAll(strings.TrimSpace(changed), "\\", "/")
	if raw == "" || path.IsAbs(raw) {
		return true
	}
	for _, segment := range strings.Split(raw, "/") {
		if segment == ".." || segment == "." || segment == "" {
			return true
		}
	}
	changed = path.Clean(raw)
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if recursivePathMatch(pattern, changed) {
			return true
		}
		if strings.HasSuffix(pattern, "/") && strings.HasPrefix(changed, pattern) {
			return true
		}
	}
	return false
}

func recursivePathMatch(pattern, changed string) bool {
	patternParts := strings.Split(strings.TrimSuffix(pattern, "/"), "/")
	changedParts := strings.Split(changed, "/")
	var match func(int, int) bool
	match = func(patternIndex, changedIndex int) bool {
		if patternIndex == len(patternParts) {
			return changedIndex == len(changedParts)
		}
		if patternParts[patternIndex] == "**" {
			for next := changedIndex; next <= len(changedParts); next++ {
				if match(patternIndex+1, next) {
					return true
				}
			}
			return false
		}
		if changedIndex == len(changedParts) {
			return false
		}
		matched, err := path.Match(patternParts[patternIndex], changedParts[changedIndex])
		return err == nil && matched && match(patternIndex+1, changedIndex+1)
	}
	return match(0, 0)
}

// DefaultProtectedPaths are conservative repository areas that require Human
// review even when TLC classified the surrounding work as Routine.
func DefaultProtectedPaths() []string {
	return []string{
		".github/",
		".agents/",
		".codex/",
		"AGENTS.md",
		"POLICY.md",
		"**/auth/**",
		"**/migrations/**",
		"**/*secret*",
		"**/*credential*",
		"**/*crypto*",
		"**/*deploy*",
		"**/Dockerfile",
		"**/docker-compose*.yml",
		"**/docker-compose*.yaml",
	}
}
